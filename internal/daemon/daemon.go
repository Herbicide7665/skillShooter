// Package daemon 常驻客户端：增量监控推送 + 应用云端合并结果 + 远程任务执行
package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"skillrelay/internal/pusher"
	"skillrelay/internal/store"
)

type Options struct {
	Server        string
	Token         string
	Hostname      string
	Interval      int // 监控轮询秒
	ApplyInterval int // 应用云端结果间隔秒
	JobPollSec    int // 任务轮询秒
	HeartbeatSec  int // 心跳探活秒（默认 1800）
	Roots         []string
	NoWatch       bool
	NoApply       bool
	NoJobs        bool
	NoHeartbeat   bool
	PushOpt       pusher.Options
	Stop          chan struct{} // 托盘退出信号
	SyncNow       chan struct{} // 托盘"立即同步"信号（推送变更+应用合并+部署分配）
	UploadAll     chan struct{} // 托盘"上传全部"信号（强制全量推送）
	PullAssigned  chan struct{} // 托盘"拉取分配"信号
}

type hubJob struct {
	ID      int64  `json:"id"`
	Machine string `json:"machine"`
	Skill   string `json:"skill"`
	Prompt  string `json:"prompt"`
}

type hubDispatch struct {
	ID     int64  `json:"id"`
	Machine string `json:"machine"`
	Action string `json:"action"`
}

type canonEntry struct {
	Name         string   `json:"name"`
	VersionID    int64    `json:"version_id"`
	TreeHash     string   `json:"tree_hash"`
	SkillID      int64    `json:"skill_id"`
	Machine      string   `json:"machine"`
	SourceHashes []string `json:"source_hashes"`
}

var stateDir string

func statePath() string { return filepath.Join(stateDir, "state.json") }

type watchState struct {
	Sigs map[string]string `json:"sigs"`
}

func loadState() watchState {
	st := watchState{Sigs: map[string]string{}}
	if b, err := os.ReadFile(statePath()); err == nil {
		json.Unmarshal(b, &st)
		if st.Sigs == nil {
			st.Sigs = map[string]string{}
		}
	}
	return st
}

func saveState(st watchState) {
	b, _ := json.Marshal(st)
	os.WriteFile(statePath(), b, 0o600)
}

// DefaultRoots 自动发现本机常见 agent skill 目录（存在才返回）
func DefaultRoots(extra []string) []string {
	home, _ := os.UserHomeDir()
	cand := []string{
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".claude", "plugins"),
		filepath.Join(home, ".zcode", "cli", "plugins", "cache"),
	}
	var out []string
	for _, d := range append(cand, extra...) {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

var logFile *os.File

func log(f string, a ...interface{}) {
	line := fmt.Sprintf("[daemon %s] %s", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(f, a...))
	fmt.Println(line)
	if logFile != nil {
		logFile.WriteString(line + "\n")
	}
}

func openLogFile() {
	home, _ := os.UserHomeDir()
	os.MkdirAll(filepath.Join(home, ".skillrelay"), 0o755)
	if f, err := os.OpenFile(filepath.Join(home, ".skillrelay", "daemon.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		logFile = f
	}
}

// Run 阻塞运行常驻客户端
func Run(opt Options) error {
	home, _ := os.UserHomeDir()
	stateDir = filepath.Join(home, ".skillrelay")
	os.MkdirAll(stateDir, 0o755)
	openLogFile()
	if len(opt.Roots) == 0 {
		opt.Roots = DefaultRoots(nil) // 未指定则自动发现；指定了则只监控指定目录
	}
	if opt.Interval <= 0 {
		opt.Interval = 30
	}
	if opt.ApplyInterval <= 0 {
		opt.ApplyInterval = 300
	}
	if opt.JobPollSec <= 0 {
		opt.JobPollSec = 5
	}
	if opt.HeartbeatSec <= 0 {
		opt.HeartbeatSec = 1800 // 30min
	}
	log("启动 server=%s hostname=%s roots=%v watch=%vs apply=%vs jobs=%vs hb=%vs",
		opt.Server, opt.Hostname, opt.Roots, opt.Interval, opt.ApplyInterval, opt.JobPollSec, opt.HeartbeatSec)

	// 启动时做一次全量推送（注册机器 + 基线）
	pushAllRoots(opt)
	debug.FreeOSMemory() // 全量收集峰值内存立即归还 OS

	// 启动即心跳：注册在线状态并立刻认领后台积压的下发指令（不等首个 30min ticker）
	if !opt.NoHeartbeat {
		heartbeatOnce(opt)
	}

	watchTick := time.NewTicker(time.Duration(opt.Interval) * time.Second)
	applyTick := time.NewTicker(time.Duration(opt.ApplyInterval) * time.Second)
	jobTick := time.NewTicker(time.Duration(opt.JobPollSec) * time.Second)
	hbTick := time.NewTicker(time.Duration(opt.HeartbeatSec) * time.Second)
	for {
		select {
		case <-watchTick.C:
			if !opt.NoWatch {
				watchOnce(opt)
			}
		case <-applyTick.C:
			if !opt.NoApply {
				applyOnce(opt)
			}
		case <-jobTick.C:
			if !opt.NoJobs {
				jobOnce(opt)
			}
		case <-hbTick.C:
			if !opt.NoHeartbeat {
				heartbeatOnce(opt)
			}
		case <-opt.SyncNow:
			log("手动触发同步")
			doSync(opt)
		case <-opt.UploadAll:
			log("手动上传全部本机 skill")
			pushAllRoots(opt)
			debug.FreeOSMemory()
		case <-opt.PullAssigned:
			log("手动拉取云端分配 skill")
			pullAssignedOnce(opt)
		case <-opt.Stop:
			log("收到退出信号，客户端停止")
			return nil
		}
	}
}

// pushAllRoots 全量推送所有监控根（启动基线/手动上传）
func pushAllRoots(opt Options) {
	for _, root := range opt.Roots {
		po := opt.PushOpt
		po.Server, po.Token, po.Path, po.Hostname = opt.Server, opt.Token, root, opt.Hostname
		if out, err := pusher.Push(po); err != nil {
			log("全量推送 %s 失败: %v", root, err)
		} else {
			log("全量推送 %s: %s", root, strings.SplitN(out, "\"count\"", 2)[0]+trimCount(out))
		}
	}
}

// doSync 完整同步：推送变更 → 应用云端合并 → 部署云端分配
func doSync(opt Options) {
	if !opt.NoWatch {
		watchOnce(opt)
	}
	if !opt.NoApply {
		applyOnce(opt)
	}
	pullAssignedOnce(opt)
}

// heartbeatOnce 心跳探活：上报存活并捎带认领后台下发的指令
func heartbeatOnce(opt Options) {
	var resp struct {
		OK         bool          `json:"ok"`
		ServerTime string        `json:"server_time"`
		Dispatches []hubDispatch `json:"dispatches"`
	}
	if err := apiPost(opt, "/api/heartbeat", map[string]string{"hostname": opt.Hostname}, &resp); err != nil {
		log("心跳失败: %v", err)
		return
	}
	for _, d := range resp.Dispatches {
		log("认领下发指令 #%d action=%s", d.ID, d.Action)
		status, detail := runDispatch(opt, d.Action)
		if err := apiPost(opt, fmt.Sprintf("/api/dispatches/%d/result", d.ID),
			map[string]string{"status": status, "result": detail}, nil); err != nil {
			log("回执指令 #%d 失败: %v", d.ID, err)
		}
		log("指令 #%d 完成 status=%s", d.ID, status)
	}
}

// runDispatch 执行一条下发指令
func runDispatch(opt Options, action string) (status, detail string) {
	switch action {
	case "pull":
		return pullAssignedStatus(opt)
	default: // sync
		doSync(opt)
		return "done", "同步完成（推送变更+应用合并+部署分配）"
	}
}

// assignedDest 分配 skill 的落盘目录：第一个用户级 skill 根（非插件缓存）
func assignedDest(opt Options) string {
	for _, root := range opt.Roots {
		norm := strings.ToLower(filepath.ToSlash(root))
		if strings.Contains(norm, "/plugins/") || strings.Contains(norm, "plugins/cache") {
			continue
		}
		return root
	}
	if len(opt.Roots) > 0 {
		return opt.Roots[0]
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agents", "skills")
}

// pullAssignedOnce 拉取云端分配给本机的 skill 并落盘
func pullAssignedOnce(opt Options) {
	if _, detail := pullAssignedStatus(opt); detail != "" {
		log("%s", detail)
	}
}

func pullAssignedStatus(opt Options) (status, detail string) {
	out, err := pusher.PullAssigned(pusher.PullOptions{
		Server: opt.Server, Token: opt.Token,
		Dest: assignedDest(opt), Machine: opt.Hostname,
	})
	if err != nil {
		return "failed", fmt.Sprintf("拉取分配失败: %v", err)
	}
	return "done", out
}

func trimCount(out string) string {
	i := strings.Index(out, "\"count\"")
	if i < 0 {
		return ""
	}
	rest := out[i:]
	if len(rest) > 22 {
		rest = rest[:22]
	}
	return rest
}

// watchOnce 增量：只推签名变化的 skill
func watchOnce(opt Options) {
	st := loadState()
	for _, root := range opt.Roots {
		skills, _, err := pusher.ScanSkills(root, pusher.Options{})
		if err != nil {
			continue
		}
		changed := map[string]map[string][]byte{}
		for dirRel, files := range skills {
			full := filepath.Join(root, filepath.FromSlash(dirRel))
			sig := pusher.SignatureSkillMD(full) // 只看 SKILL.md（size+mtime），不读内容
			key := root + "::" + dirRel
			if st.Sigs[key] == sig {
				continue
			}
			st.Sigs[key] = sig
			changed[dirRel] = files
		}
		if len(changed) > 0 {
			out, err := pusher.UploadBundle(opt.Server, opt.Token, opt.Hostname, root, changed)
			if err != nil {
				log("增量推送 %d 个失败: %v", len(changed), err)
				continue
			}
			log("增量推送 %d 个变更 skill: %s", len(changed), trimCount(out))
			saveState(st)
		}
	}
	debug.FreeOSMemory() // 每轮结束归还峰值内存
}

// applyOnce 把云端 merged 版本落到本地同名 skill 目录（只覆盖本地已存在的）
func applyOnce(opt Options) {
	var canon []canonEntry
	if err := apiGet(opt, "/api/canonical", &canon); err != nil {
		log("拉取 canonical 失败: %v", err)
		return
	}
	for _, e := range canon {
		target := ""
		for _, root := range opt.Roots {
			p := filepath.Join(root, e.Name)
			if fi, err := os.Stat(filepath.Join(p, "SKILL.md")); err == nil && !fi.IsDir() {
				target = p
				break
			}
		}
		if target == "" {
			continue // 本地不存在同名 skill，不注入
		}
		files, ok := pusher.CollectSkillMD(target)
		var localHash string
		if ok {
			localHash = store.TreeHash(files)
		}
		if localHash == e.TreeHash {
			continue // 已是云端合并版
		}
		// 只有本地内容仍是合并前的某个源变体才应用，避免覆盖用户后续手工修改
		isSource := false
		for _, sh := range e.SourceHashes {
			if localHash == sh {
				isSource = true
				break
			}
		}
		if localHash != "" && !isSource {
			continue
		}
		tarGz, err := httpGetBytes(opt, fmt.Sprintf("/api/skills/%d/download?version=%d", e.SkillID, e.VersionID))
		if err != nil {
			log("下载 %s v%d 失败: %v", e.Name, e.VersionID, err)
			continue
		}
		// 只替换 SKILL.md 核心文档，不动本地目录内其他文件（脚本/产物归本机所有）
		tmp := target + ".incoming"
		os.RemoveAll(tmp)
		if err := pusher.ExtractTarGz(tarGz, tmp); err != nil {
			log("应用 %s v%d 失败（本地未改动）: %v", e.Name, e.VersionID, err)
			os.RemoveAll(tmp)
			continue
		}
		md, ok := pusher.CollectSkillMD(tmp)
		os.RemoveAll(tmp)
		if !ok {
			log("应用 %s v%d 失败: 包内无 SKILL.md", e.Name, e.VersionID)
			continue
		}
		if err := os.WriteFile(filepath.Join(target, "SKILL.md"), md["SKILL.md"], 0o644); err != nil {
			log("应用 %s v%d 写入失败: %v", e.Name, e.VersionID, err)
			continue
		}
		log("已应用云端合并结果: %s -> v%d（来自 %s）", e.Name, e.VersionID, e.Machine)
	}
	debug.FreeOSMemory()
}

// jobOnce 认领并执行一个远程任务
func jobOnce(opt Options) {
	var job hubJob
	if err := apiGet(opt, "/api/jobs/next?machine="+urlEscape(opt.Hostname), &job); err != nil || job.ID == 0 {
		return
	}
	log("认领任务 #%d skill=%q", job.ID, job.Skill)
	status, cli, report := executeJob(opt, &job)
	if err := apiPost(opt, fmt.Sprintf("/api/jobs/%d/result", job.ID), map[string]interface{}{
		"status": status, "agent_cli": cli, "report": report,
	}, nil); err != nil {
		log("回传任务 #%d 结果失败: %v", job.ID, err)
	}
	log("任务 #%d 完成 status=%s cli=%s", job.ID, status, cli)
	debug.FreeOSMemory()
}

// pickAgentCLI 按本机实际条件选 agent CLI（claude 优先，zcode 次之）
func pickAgentCLI() string {
	for _, c := range []string{"claude", "zcode"} {
		if _, err := exec.LookPath(c); err == nil {
			return c
		}
	}
	return ""
}

func executeJob(opt Options, job *hubJob) (status, cli string, report map[string]interface{}) {
	home, _ := os.UserHomeDir()
	workdir := filepath.Join(home, ".skillrelay", "jobs", fmt.Sprint(job.ID))
	os.MkdirAll(workdir, 0o755)

	started := time.Now().UTC().Format(time.RFC3339)
	report = map[string]interface{}{
		"format":    "asdste100",
		"version":   "1.0",
		"job_id":    job.ID,
		"skill":     job.Skill,
		"machine":   opt.Hostname,
		"started_at": started,
	}
	cli = pickAgentCLI()
	if cli == "" {
		report["status"] = "failed"
		report["error"] = "本机未找到 claude / zcode CLI"
		report["finished_at"] = time.Now().UTC().Format(time.RFC3339)
		return "failed", "", report
	}
	report["agent_cli"] = cli

	prompt := job.Prompt
	if job.Skill != "" {
		prompt = fmt.Sprintf("使用 skill「%s」完成以下任务（若该 skill 存在于本机 skill 目录则严格按其 SKILL.md 执行）。\n\n%s", job.Skill, job.Prompt)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, "-p")
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = workdir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	dur := time.Since(time.Now())
	_ = dur
	fin := time.Now().UTC().Format(time.RFC3339)
	report["finished_at"] = fin
	report["exit_code"] = cmd.ProcessState.ExitCode()
	out := stdout.String()
	if len(out) > 65536 {
		out = out[:65536] + "\n…(截断)"
	}
	report["output"] = out
	e := stderr.String()
	if len(e) > 8192 {
		e = e[:8192]
	}
	report["stderr"] = e
	// 工件：workdir 里生成的文件
	var arts []string
	_ = filepath.WalkDir(workdir, func(p string, fd os.DirEntry, ferr error) error {
		if ferr == nil && !fd.IsDir() {
			rel, _ := filepath.Rel(workdir, p)
			if rel != "" {
				arts = append(arts, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	report["artifacts"] = arts

	if ctx.Err() == context.DeadlineExceeded {
		report["status"] = "timeout"
		return "timeout", cli, report
	}
	if err != nil {
		report["status"] = "error"
		report["error"] = err.Error()
		return "error", cli, report
	}
	report["status"] = "ok"
	return "ok", cli, report
}

// ---- hub API 小封装（全部走 HMAC 签名，token 不上 wire）----

func (o Options) headers(req *http.Request) {
	pusher.AddAuth(req, o.Token, nil)
}

func apiGet(opt Options, path string, dst interface{}) error {
	req, _ := http.NewRequest("GET", strings.TrimRight(opt.Server, "/")+path, nil)
	opt.headers(req)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil // 无任务等
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b[:min(len(b), 200)]))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func apiPost(opt Options, path string, body interface{}, dst interface{}) error {
	bj, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", strings.TrimRight(opt.Server, "/")+path, bytes.NewReader(bj))
	req.Header.Set("Content-Type", "application/json")
	pusher.AddAuth(req, opt.Token, bj) // JSON body 必须参与签名
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b[:min(len(b), 200)]))
	}
	if dst != nil {
		return json.NewDecoder(resp.Body).Decode(dst)
	}
	return nil
}

func httpGetBytes(opt Options, path string) ([]byte, error) {
	req, _ := http.NewRequest("GET", strings.TrimRight(opt.Server, "/")+path, nil)
	opt.headers(req)
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func urlEscape(s string) string {
	// 仅用于 machine 名，简单替换足够
	r := strings.NewReplacer(" ", "%20", "&", "%26", "?", "%3F", "#", "%23")
	return r.Replace(s)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
