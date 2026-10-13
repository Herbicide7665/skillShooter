//go:build !windows

package daemon

import "fmt"

// RunTray 非 Windows 占位：无桌面环境（VPS）直接跑 CLI 模式
func RunTray(opt Options) error {
	fmt.Println("[daemon] --tray 仅 Windows 支持，已按 CLI 模式运行")
	opt.Stop = make(chan struct{})
	select {} // 阻塞，等同前台常驻
}

// AlertNoConfig 非 Windows：打印提示
func AlertNoConfig() {
	fmt.Println("此二进制尚未包含接入配置。请到 Hub 设置页下载带配置的客户端，或使用: skill-hub daemon --server URL --token TOKEN")
}
