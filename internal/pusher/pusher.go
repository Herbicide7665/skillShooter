package pusher

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Options push/daemon 共用参数
type Options struct {
	Server         string
	Token          string
	Path           string
	Hostname       string
	MaxFileBytes   int64  // 单文件上限，超限跳过（0=不限制）
	MaxSkillBytes  int64  // 单 skill 目录总预算，超出按"大文件先丢"裁剪（0=不限制）
	IncludeBinaries bool  // 不按扩展名剔除媒体/档案文件
}

// mediaExts 默认剔除的媒体/档案扩展名（skill 正文与脚本不含这些）
var mediaExts = map[string]bool{
	".mp3": true, ".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
	".webm": true, ".flv": true, ".wav": true, ".ogg": true, ".m4a": true,
	".wmv": true, ".m4v": true, ".aac": true, ".flac": true, ".wma": true,
	".ts": true, ".mts": true, ".m2ts": true,
	".apk": true, ".zip": true, ".tar": true, ".gz": true, ".tgz": true,
	".7z": true, ".rar": true, ".iso": true, ".exe": true, ".dll": true,
	".so": true, ".bin": true, ".parquet": true,
}

type fileInfo struct {
	rel  string
	size int64
	data []byte
}

func skipDirs(name string) bool {
	return name == ".git" || name == "node_modules" || name == "__pycache__"
}

// ScanSkills 递归扫描 root 下所有含 SKILL.md 的 skill 目录（支持多层嵌套）。
// 返回 skillDir(相对root)->files、skill 数、被跳过的文件数与字节数。
func ScanSkills(root string, opt Options) (map[string]map[string][]byte, int, int, int64, error) {
	out := map[string]map[string][]byte{}
	n := 0
	skippedN := 0
	var skippedBytes int64
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 跳过不可读
		}
		if d.IsDir() {
			if skipDirs(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}
		files, skn, skb := CollectSkillDir(filepath.Dir(path), opt)
		skippedN += skn
		skippedBytes += skb
		if files == nil {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			rel = filepath.Base(path)
		}
		out[filepath.ToSlash(rel)] = files
		n++
		return nil
	})
	return out, n, skippedN, skippedBytes, err
}

// CollectSkillDir 按 Options 策略收集单个 skill 目录（返回 nil 表示无有效 SKILL.md）
func CollectSkillDir(skillDir string, opt Options) (map[string][]byte, int, int64) {
	var candidates []fileInfo
	skillMD := fileInfo{}
	skippedN := 0
	var skippedBytes int64
	_ = filepath.WalkDir(skillDir, func(p string, fd os.DirEntry, ferr error) error {
		if ferr != nil || fd.IsDir() {
			if ferr == nil && fd.IsDir() && skipDirs(fd.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(skillDir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		info, err := fd.Info()
		if err != nil {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(rel))
		if !opt.IncludeBinaries && mediaExts[ext] {
			skippedN++
			skippedBytes += info.Size()
			return nil
		}
		if opt.MaxFileBytes > 0 && info.Size() > opt.MaxFileBytes {
			skippedN++
			skippedBytes += info.Size()
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		fi := fileInfo{rel: rel, size: info.Size(), data: data}
		if fi.rel == "SKILL.md" {
			skillMD = fi
		} else {
			candidates = append(candidates, fi)
		}
		return nil
	})
	if skillMD.rel == "" {
		return nil, skippedN, skippedBytes
	}
	// 单 skill 总量预算：小文件优先保住，超出预算丢最大的
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].size < candidates[j].size })
	budget := opt.MaxSkillBytes - skillMD.size
	files := map[string][]byte{"SKILL.md": skillMD.data}
	for _, c := range candidates {
		if opt.MaxSkillBytes > 0 && budget-c.size < 0 {
			skippedN++
			skippedBytes += c.size
			continue
		}
		budget -= c.size
		files[c.rel] = c.data
	}
	return files, skippedN, skippedBytes
}

// SignatureSkillDir 低成本目录签名：过滤后文件的 (rel, size, mtime) 哈希，不读内容
func SignatureSkillDir(skillDir string, opt Options) string {
	h := sha256.New()
	_ = filepath.WalkDir(skillDir, func(p string, fd os.DirEntry, ferr error) error {
		if ferr != nil || fd.IsDir() {
			if ferr == nil && fd.IsDir() && skipDirs(fd.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(skillDir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(rel))
		if !opt.IncludeBinaries && mediaExts[ext] {
			return nil
		}
		info, err := fd.Info()
		if err != nil {
			return nil
		}
		if opt.MaxFileBytes > 0 && info.Size() > opt.MaxFileBytes {
			return nil
		}
		fmt.Fprintf(h, "%s|%d|%d\n", rel, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

// buildBundle 把所有 skill 打成 tar.gz：<skillDir>/<relPath>
func buildBundle(skills map[string]map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for dir, files := range skills {
		for rel, data := range files {
			name := dir + "/" + rel
			hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), ModTime: time.Now()}
			if err := tw.WriteHeader(hdr); err != nil {
				return nil, err
			}
			if _, err := tw.Write(data); err != nil {
				return nil, err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UploadBundle 上传一个已收集的 skill 集合（增量推送复用）
func UploadBundle(server, token, hostname string, skills map[string]map[string][]byte) (string, error) {
	bundle, err := buildBundle(skills)
	if err != nil {
		return "", fmt.Errorf("打包失败: %w", err)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("hostname", hostname); err != nil {
		return "", err
	}
	if err := mw.WriteField("skill_count", fmt.Sprint(len(skills))); err != nil {
		return "", err
	}
	part, err := mw.CreateFormFile("bundle", "skills.tar.gz")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(bundle); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	url := strings.TrimRight(server, "/") + "/api/push"
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Token", token)
	hc := &http.Client{Timeout: 10 * time.Minute}
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 500))
	}
	return string(body), nil
}

// Push 扫描并上传，返回结果摘要
func Push(opt Options) (string, error) {
	if opt.Server == "" || opt.Token == "" || opt.Path == "" {
		return "", fmt.Errorf("需要 --server --token --path 三个参数")
	}
	skills, n, skippedN, skippedBytes, err := ScanSkills(opt.Path, opt)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", fmt.Errorf("在 %s 下未找到任何 SKILL.md", opt.Path)
	}
	if skippedN > 0 {
		fmt.Printf("==> 已按策略跳过 %d 个文件（共 %.1f MB：媒体/超限/超出单skill预算；--include-binaries 或调 --max-file-mb/--max-skill-mb 可放开）\n",
			skippedN, float64(skippedBytes)/1048576)
	}
	return UploadBundle(opt.Server, opt.Token, opt.hostnameOrDefault(), skills)
}

func (o Options) hostnameOrDefault() string {
	if o.Hostname != "" {
		return o.Hostname
	}
	h, _ := os.Hostname()
	if h == "" {
		h = "unknown"
	}
	return h
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
