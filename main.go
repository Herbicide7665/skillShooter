package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"path/filepath"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"skill-hub/internal/daemon"
	"skill-hub/internal/llm"
	"skill-hub/internal/pusher"
	"skill-hub/internal/server"
	"skill-hub/internal/store"
)

// Config config.yaml
type Config struct {
	LLM llm.Config `yaml:",inline"`
}

func exeDir() string {
	ex, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(ex)
}

func loadConfig() Config {
	var cfg Config
	cfg.LLM.BaseURL = os.Getenv("SKILLHUB_GLM_BASE_URL")
	cfg.LLM.Model = os.Getenv("SKILLHUB_GLM_MODEL")
	// env 优先
	if k := os.Getenv("GLM_API_KEY"); k != "" {
		cfg.LLM.APIKey = k
	} else if k := os.Getenv("ZHIPUAI_API_KEY"); k != "" {
		cfg.LLM.APIKey = k
	}
	// config.yaml 与二进制同目录
	path := filepath.Join(exeDir(), "config.yaml")
	if data, err := os.ReadFile(path); err == nil {
		var fc Config
		if err := yaml.Unmarshal(data, &fc); err == nil {
			if cfg.LLM.APIKey == "" {
				cfg.LLM.APIKey = fc.LLM.APIKey
			}
			if cfg.LLM.BaseURL == "" {
				cfg.LLM.BaseURL = fc.LLM.BaseURL
			}
			if cfg.LLM.Model == "" {
				cfg.LLM.Model = fc.LLM.Model
			}
		}
	}
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
	}
	if cfg.LLM.Model == "" {
		cfg.LLM.Model = "glm-5.3-flash"
	}
	return cfg
}

func main() {
	if len(os.Args) < 2 {
		// 无参数 = 客户端模式（双击已配置的二进制）
		runClientDefault()
		return
	}
	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	case "push":
		cmdPush(os.Args[2:])
	case "pull":
		cmdPull(os.Args[2:])
	case "daemon":
		cmdDaemon(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`skill-hub — 跨机 Agent Skill 管理平台

用法:
  skill-hub serve  [--addr 0.0.0.0:8790] [--db skillhub.db]
      启动 web 服务（数据与 config.yaml 就在二进制同目录；启动日志含接入 token）
  skill-hub push --server URL --token TOKEN --path <目录> [--hostname 名字]
      递归扫描目录下所有含 SKILL.md 的子目录并上传
  skill-hub pull --server URL --token TOKEN --dest <目录> [--machine 名字] [--overwrite]
      把 hub 上全部（或某机器的）skill 最新版本拉回本地（合并成果分发）
  skill-hub daemon --server URL --token TOKEN [--hostname 名字] [选项]
      常驻客户端：增量监控本机 agent 目录推送 + 应用云端合并结果 + 接收远程 skill 调用
      选项: --interval 30  --apply-interval 300  --job-poll 5  --roots 路径1,路径2
            --no-watch / --no-apply / --no-jobs
`)
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "0.0.0.0:8790", "监听地址")
	dbPath := fs.String("db", "", "SQLite 路径（默认二进制同目录 skillhub.db）")
	fs.Parse(args)

	dir := exeDir()
	if *dbPath == "" {
		*dbPath = filepath.Join(dir, "skillhub.db")
	}
	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()
	if tok, err := st.EnsureToken(); err != nil {
		fmt.Fprintf(os.Stderr, "生成 token 失败: %v\n", err)
		os.Exit(1)
	} else {
		fmt.Printf("[skill-hub] 接入 token: %s\n", tok)
	}

	cfg := loadConfig()
	lc := llm.New(cfg.LLM)
	srv := server.New(st, lc)

	mux := http.NewServeMux()
	srv.Serve(mux)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "监听失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[skill-hub] serving http://%s  db=%s  glm=%s(model=%s)\n",
		ln.Addr(), *dbPath, map[bool]string{true: "已配置", false: "未配置"}[cfg.LLM.APIKey != ""], cfg.LLM.Model)

	httpSrv := &http.Server{Handler: mux}
	go func() {
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "服务异常退出: %v\n", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
}

func cmdPush(args []string) {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	serverURL := fs.String("server", os.Getenv("SKILLHUB_SERVER"), "服务地址，如 http://1.2.3.4:8790")
	token := fs.String("token", os.Getenv("SKILLHUB_TOKEN"), "接入 token（服务端设置页可见）")
	path := fs.String("path", "", "要扫描的 skill 根目录")
	hostname := fs.String("hostname", "", "来源机器标记（默认取本机主机名）")
	maxFileMB := fs.Int("max-file-mb", 10, "单文件上限(MB)，超限跳过（0=不限制）")
	maxSkillMB := fs.Int("max-skill-mb", 50, "单skill总量预算(MB)，超出丢最大文件（0=不限制）")
	includeBin := fs.Bool("include-binaries", false, "不按扩展名剔除媒体/档案文件（mp4/zip/exe等）")
	fs.Parse(args)

	opt := pusher.Options{Server: *serverURL, Token: *token, Path: *path, Hostname: *hostname,
		MaxFileBytes: int64(*maxFileMB) * 1024 * 1024, MaxSkillBytes: int64(*maxSkillMB) * 1024 * 1024,
		IncludeBinaries: *includeBin}
	start := time.Now()
	out, err := pusher.Push(opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "push 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("push 完成 (%.1fs): %s\n", time.Since(start).Seconds(), out)
}

func cmdPull(args []string) {
	fs := flag.NewFlagSet("pull", flag.ExitOnError)
	serverURL := fs.String("server", os.Getenv("SKILLHUB_SERVER"), "服务地址，如 http://1.2.3.4:8790")
	token := fs.String("token", os.Getenv("SKILLHUB_TOKEN"), "接入 token")
	dest := fs.String("dest", "", "拉取落盘目录（如 ~/.agents/skills）")
	machine := fs.String("machine", "", "只拉某台机器的版本（空=全部，同名多机自动加 @machine 后缀）")
	overwrite := fs.Bool("overwrite", false, "覆盖已存在的 skill 目录")
	fs.Parse(args)

	opt := pusher.PullOptions{Server: *serverURL, Token: *token, Dest: *dest,
		Machine: *machine, Overwrite: *overwrite}
	start := time.Now()
	out, err := pusher.Pull(opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pull 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("pull 完成 (%.1fs): %s\n", time.Since(start).Seconds(), out)
}

func cmdDaemon(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	serverURL := fs.String("server", os.Getenv("SKILLHUB_SERVER"), "hub 地址")
	token := fs.String("token", os.Getenv("SKILLHUB_TOKEN"), "接入 token")
	hostname := fs.String("hostname", "", "本机标识（默认取主机名）")
	interval := fs.Int("interval", 30, "监控轮询间隔秒")
	applyInterval := fs.Int("apply-interval", 300, "应用云端合并结果间隔秒")
	jobPoll := fs.Int("job-poll", 5, "远程任务轮询间隔秒")
	roots := fs.String("roots", "", "监控目录（逗号分隔；留空=自动发现 .agents/.claude/.zcode，指定则只监控这些）")
	noWatch := fs.Bool("no-watch", false, "关闭监控推送")
	noApply := fs.Bool("no-apply", false, "关闭应用云端结果")
	noJobs := fs.Bool("no-jobs", false, "关闭远程任务执行")
	tray := fs.Bool("tray", false, "Windows 托盘模式（隐藏窗口）")
	maxFileMB := fs.Int("max-file-mb", 10, "单文件上限(MB)")
	maxSkillMB := fs.Int("max-skill-mb", 50, "单skill总量预算(MB)")
	fs.Parse(args)

	hn := *hostname
	if hn == "" {
		hn, _ = os.Hostname()
	}
	var extraRoots []string
	if *roots != "" {
		extraRoots = strings.Split(*roots, ",")
	}
	opt := daemon.Options{
		Server: *serverURL, Token: *token, Hostname: hn,
		Interval: *interval, ApplyInterval: *applyInterval, JobPollSec: *jobPoll,
		Roots: extraRoots, NoWatch: *noWatch, NoApply: *noApply, NoJobs: *noJobs,
		PushOpt: pusher.Options{
			MaxFileBytes:  int64(*maxFileMB) * 1024 * 1024,
			MaxSkillBytes: int64(*maxSkillMB) * 1024 * 1024,
		},
	}
	if *tray {
		if err := daemon.RunTray(opt); err != nil {
			fmt.Fprintf(os.Stderr, "daemon 退出: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := daemon.Run(opt); err != nil {
		fmt.Fprintf(os.Stderr, "daemon 退出: %v\n", err)
		os.Exit(1)
	}
}

// runClientDefault 无参数双击模式：读自身尾部内嵌配置（回落 config.json），
// 有配置则直接以托盘（Windows）/CLI（其他）常驻；无配置弹窗提示。
func runClientDefault() {
	server, token, ok := daemon.SelfConfig()
	if !ok {
		if p := daemon.ConfigFilePath(); func() bool { _, err := os.Stat(p); return err == nil }() {
			if b, err := os.ReadFile(p); err == nil {
				var cfg struct{ Server, Token string }
				if json.Unmarshal(b, &cfg) == nil && cfg.Server != "" && cfg.Token != "" {
					server, token, ok = cfg.Server, cfg.Token, true
				}
			}
		}
	}
	hn, _ := os.Hostname()
	opt := daemon.Options{Server: server, Token: token, Hostname: hn}
	if !ok {
		daemon.AlertNoConfig()
		os.Exit(1)
	}
	daemon.SaveConfigFile(server, token)
	if err := daemon.RunTray(opt); err != nil {
		fmt.Fprintf(os.Stderr, "daemon 退出: %v\n", err)
		os.Exit(1)
	}
}
