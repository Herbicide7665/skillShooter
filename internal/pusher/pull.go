package pusher

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PullOptions pull 参数
type PullOptions struct {
	Server    string
	Token     string
	Dest      string
	Machine   string
	Overwrite bool
}

type pullSkill struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Machine    string `json:"machine"`
	LatestVer  int64  `json:"latest_version_id"`
	VerCount   int    `json:"version_count"`
}

// Pull 从 hub 拉取全部（或某机器）skill 的最新版本，落盘到 dest 目录。
// 同名多机实体目录名加 @machine 后缀区分；默认跳过已存在目录，--overwrite 覆盖。
func Pull(opt PullOptions) (string, error) {
	if opt.Server == "" || opt.Token == "" || opt.Dest == "" {
		return "", fmt.Errorf("需要 --server --token --dest 参数")
	}
	base := strings.TrimRight(opt.Server, "/")
	hc := &http.Client{Timeout: 5 * time.Minute}

	// 列表
	req, _ := http.NewRequest("GET", base+fmt.Sprintf("/api/skills?machine=%s", opt.Machine), nil)
	req.Header.Set("X-Token", opt.Token)
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	var skills []pullSkill
	if err := json.Unmarshal(body, &skills); err != nil {
		return "", fmt.Errorf("解析列表失败: %w", err)
	}
	if len(skills) == 0 {
		return "hub 上没有可拉的 skill", nil
	}

	// 同名冲突检测
	nameCount := map[string]int{}
	for _, s := range skills {
		nameCount[s.Name]++
	}

	if err := os.MkdirAll(opt.Dest, 0o755); err != nil {
		return "", err
	}
	pulled, skipped, failed := 0, 0, 0
	for _, s := range skills {
		dirName := s.Name
		if nameCount[s.Name] > 1 && opt.Machine == "" {
			dirName = s.Name + "@" + s.Machine
		}
		target := filepath.Join(opt.Dest, dirName)
		if !opt.Overwrite {
			if _, err := os.Stat(filepath.Join(target, "SKILL.md")); err == nil {
				skipped++
				continue
			}
		} else {
			if err := os.RemoveAll(target); err != nil {
				failed++
				continue
			}
		}
		req, _ := http.NewRequest("GET", base+fmt.Sprintf("/api/skills/%d/download", s.ID), nil)
		req.Header.Set("X-Token", opt.Token)
		resp, err := hc.Do(req)
		if err != nil {
			failed++
			continue
		}
		bundle, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			failed++
			continue
		}
		if err := ExtractTarGz(bundle, target); err != nil {
			failed++
			continue
		}
		pulled++
	}
	return fmt.Sprintf("拉取完成: %d 个新落盘, %d 个已存在跳过, %d 个失败（共 %d）", pulled, skipped, failed, len(skills)), nil
}

func ExtractTarGz(data []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := strings.ReplaceAll(hdr.Name, "\\", "/")
		if strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
			continue
		}
		p := filepath.Join(dest, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
}
