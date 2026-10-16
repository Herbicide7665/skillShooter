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

// runtimeDirs 运行时产物目录名（任一层级命中即整目录跳过）——
// skill 目录里的这些子目录是业务数据产物，不属于 skill 本体，进版本历史只会造成版本翻滚
var runtimeDirs = map[string]bool{
	"cache": true, "tmp": true, "temp": true, "transcripts": true,
	"logs": true, "output": true, "outputs": true, "downloads": true, "state": true,
}

func skipDirs(name string) bool {
	l := strings.ToLower(name)
	return name == ".git" || name == "node_modules" || name == "__pycache__" || runtimeDirs[l]
}

// runtimeExts 运行时产物文件扩展名
var runtimeExts = map[string]bool{".log": true, ".tmp": true}

// ScanSkills 递归扫描 root 下所有含 SKILL.md 的 skill 目录（支持多层嵌套）。
// 只采集 SKILL.md 核心文档，目录内其余文件（脚本/产物/参考）不进版本系统。
// 注意：root 本身不做 skipDirs 检查（root 常为名字撞 runtimeDirs 的目录，
// 如 .zcode/cli/plugins/cache——按名跳过会把整棵树漏掉），仅对内部子目录生效。
// 返回 skillDir(相对root)->files、skill 数。
func ScanSkills(root string, opt Options) (map[string]map[string][]byte, int, error) {
	out := map[string]map[string][]byte{}
	n := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skipDirs(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}
		files, ok := CollectSkillMD(filepath.Dir(path))
		if !ok {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil || rel == "." {
			// root 本身就是 skill 目录：用目录名做 bundle entry（"./SKILL.md" 会让服务端归属失败）
			rel = filepath.Base(filepath.Dir(path))
		}
		out[filepath.ToSlash(rel)] = files
		n++
		return nil
	})
	return out, n, err
}

// CollectSkillMD 只读 SKILL.md 核心文档（返回 false = 无 SKILL.md）
func CollectSkillMD(skillDir string) (map[string][]byte, bool) {
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return map[string][]byte{"SKILL.md": data}, true
}

// SignatureSkillMD 低成本签名：仅 SKILL.md 的 (size, mtime)，不读内容
func SignatureSkillMD(skillDir string) string {
	fi, err := os.Stat(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return ""
	}
	h := sha256.New()
	fmt.Fprintf(h, "SKILL.md|%d|%d\n", fi.Size(), fi.ModTime().UnixNano())
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
// sourceRoot 为采集来源根目录（服务端按其前缀分类系统 skill；空则不参与分类）
func UploadBundle(server, token, hostname, sourceRoot string, skills map[string]map[string][]byte) (string, error) {
	bundle, err := buildBundle(skills)
	if err != nil {
		return "", fmt.Errorf("打包失败: %w", err)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("hostname", hostname); err != nil {
		return "", err
	}
	if err := mw.WriteField("source_root", sourceRoot); err != nil {
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
	AddAuth(req, token, buf.Bytes())
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
	skills, n, err := ScanSkills(opt.Path, opt)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", fmt.Errorf("在 %s 下未找到任何 SKILL.md", opt.Path)
	}
	return UploadBundle(opt.Server, opt.Token, opt.hostnameOrDefault(), opt.Path, skills)
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
