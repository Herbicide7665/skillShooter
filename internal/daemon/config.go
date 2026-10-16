package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ClientConfig 客户端全量配置（~/.skillshooter/config.json）
// 兼容旧 {server, token} 两字段格式；缺省字段由 Run/flag 默认兜底。
type ClientConfig struct {
	Server        string   `json:"server"`
	Token         string   `json:"token"`
	Hostname      string   `json:"hostname"`      // 设备名（空=自动取主机名）
	HeartbeatSec  int      `json:"heartbeat_sec"` // 心跳间隔秒（默认 1800）
	Interval      int      `json:"interval"`      // 监控推送轮询秒（默认 30）
	ApplyInterval int      `json:"apply_interval"` // 应用云端合并秒（默认 300）
	JobPollSec    int      `json:"job_poll"`      // 远程任务轮询秒（默认 5）
	Roots         []string `json:"roots"`         // 监控目录（空=自动发现）
	NoWatch       bool     `json:"no_watch"`
	NoApply       bool     `json:"no_apply"`
	NoJobs        bool     `json:"no_jobs"`
	NoHeartbeat   bool     `json:"no_heartbeat"`
}

// ConfigFilePath 配置文件路径
func ConfigFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".skillshooter", "config.json")
}

// LoadClientConfig 读配置文件（不存在返回零值 ok=false）
func LoadClientConfig() (ClientConfig, bool) {
	var cfg ClientConfig
	data, err := os.ReadFile(ConfigFilePath())
	if err != nil {
		return cfg, false
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, false
	}
	if cfg.Server == "" || cfg.Token == "" {
		return cfg, false
	}
	return cfg, true
}

// SaveClientConfig 写配置文件（0600）
func SaveClientConfig(cfg ClientConfig) error {
	p := ConfigFilePath()
	os.MkdirAll(filepath.Dir(p), 0o755)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0o600)
}

// Options 从配置构建 daemon Options（hostname 缺省取主机名；间隔零值由 Run 兜底）
func (c ClientConfig) Options(stop, sync, upload, pull chan struct{}) (Options, error) {
	hn := strings.TrimSpace(c.Hostname)
	if hn == "" {
		hn, _ = os.Hostname()
	}
	return Options{
		Server: c.Server, Token: c.Token, Hostname: hn,
		Interval: c.Interval, ApplyInterval: c.ApplyInterval, JobPollSec: c.JobPollSec,
		HeartbeatSec: c.HeartbeatSec,
		Roots: c.Roots, NoWatch: c.NoWatch, NoApply: c.NoApply, NoJobs: c.NoJobs, NoHeartbeat: c.NoHeartbeat,
		Stop: stop, SyncNow: sync, UploadAll: upload, PullAssigned: pull,
	}, nil
}

// SaveConfigFile 旧接口保留（内嵌配置首次落盘）
func SaveConfigFile(server, token string) {
	cfg, _ := LoadClientConfig()
	if cfg.Server == "" || cfg.Token == "" {
		cfg = ClientConfig{Server: server, Token: token}
	}
	SaveClientConfig(cfg)
}
