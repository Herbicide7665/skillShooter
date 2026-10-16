package daemon

import (
	"bytes"
	"encoding/json"
	"os"
)

// 客户端自配置：server/token 追加在二进制文件尾部（PE/ELF/Mach-O 加载器不读尾部，
// 不影响执行）；客户端启动时读自身文件尾部解析，实现"下载即用"。

const cfgMarker = "\nSKILLSHOOTER-CLIENT-CONFIG\n"

// AppendConfig 把配置追加到二进制文件副本尾部，返回带配置的字节流
func AppendConfig(bin []byte, server, token string) ([]byte, error) {
	cfg, err := json.Marshal(map[string]string{"server": server, "token": token})
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(bin)+len(cfg)+len(cfgMarker)+2)
	out = append(out, bin...)
	out = append(out, []byte(cfgMarker)...)
	out = append(out, cfg...)
	out = append(out, '\n')
	return out, nil
}

// SelfConfig 读自身文件尾部解析内嵌配置（找不到返回 ok=false）
func SelfConfig() (server, token string, ok bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", false
	}
	f, err := os.Open(exe)
	if err != nil {
		return "", "", false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", "", false
	}
	const tail = 1 << 16 // 64KB 足够
	off := fi.Size() - tail
	if off < 0 {
		off = 0
	}
	buf := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil {
		return "", "", false
	}
	idx := bytes.LastIndex(buf, []byte(cfgMarker))
	if idx < 0 {
		return "", "", false
	}
	var cfg struct {
		Server string `json:"server"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(buf[idx+len(cfgMarker):]), &cfg); err != nil {
		return "", "", false
	}
	if cfg.Server == "" || cfg.Token == "" {
		return "", "", false
	}
	return cfg.Server, cfg.Token, true
}

// ConfigFilePath 客户端侧配置文件（内嵌配置首次启动时落盘，便于查看/手改）
func ConfigFilePath() string {
	home, _ := os.UserHomeDir()
	return home + string(os.PathSeparator) + ".skillshooter" + string(os.PathSeparator) + "config.json"
}

// SaveConfigFile 配置落盘
func SaveConfigFile(server, token string) {
	cfg, _ := json.MarshalIndent(map[string]string{"server": server, "token": token}, "", "  ")
	p := ConfigFilePath()
	os.MkdirAll(dirOf(p), 0o755)
	os.WriteFile(p, cfg, 0o600)
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == os.PathSeparator {
			return p[:i]
		}
	}
	return "."
}
