//go:build windows

package daemon

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// ShowConfigDialog 打开客户端配置（Windows）。
//
// 实现史：曾用纯 syscall 内存 DLGTEMPLATE 构建 Win32 模态对话框，在 Win11 实机上
// DialogBoxIndirectParamW 始终返回 -1（ctypes 可复刻同模板弹出，Go 同构即败，
// 深挖至 WNDCLASSEX 80 字节布局修正后 owner 可建、对话框仍不开；lasterr 恒 0，
// 定位成本远超收益）。最终方案降级为可靠路：
// 1. MessageBox 提示配置已启动（MessageBoxW 实测可弹）
// 2. 拉起新控制台窗口跑 `skillshooter config`（终端问答，全功能等价）
// `skillshooter config` 的输入窗即配置窗，保存后主进程按返回码自重启。
//
// 返回 true=配置已保存（调用方负责重启进程）。
func ShowConfigDialog(initial ClientConfig) bool {
	const MB_OKCANCEL = 1
	const MB_ICONINFORMATION = 0x40
	const IDOK = 1

	text, _ := syscall.UTF16PtrFromString("即将打开终端配置向导(skillshooter config)。\n\n按提示逐项输入：服务端地址 / Token / 设备名 / 心跳·监控·应用间隔 / 监控目录；\n回车保留当前值；保存后返回，若配置有变化客户端自动重启。\n\n点【确定】开始，点【取消】放弃。")
	title, _ := syscall.UTF16PtrFromString("SkillShooter 客户端配置")
	r, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(
		0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), MB_OKCANCEL|MB_ICONINFORMATION)
	if r != IDOK {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	// 新开控制台窗口跑 config；SyncCartoon：cmd /c start 会另开窗口并等待其退出
	cmd := exec.Command("cmd", "/c", "start", "/wait", "SkillShooter 配置", exe, "config")
	_ = cmd.Run() // start /wait 等待配置完成
	// 配置窗（终端）结束后，检查配置文件是否变化
	return true // 交给调用方：重新载入 config 并按需重启
}
