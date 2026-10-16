//go:build !windows

package daemon

import "fmt"

// ShowConfigDialog 非 Windows 无原生弹窗，提示走 CLI 配置；返回 false = 未保存。
func ShowConfigDialog(initial ClientConfig) bool {
	fmt.Println("当前平台无图形配置窗，请运行: skillshooter config 进行终端配置")
	return false
}
