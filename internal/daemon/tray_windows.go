//go:build windows

package daemon

import (
	_ "embed"
	"fmt"
	"os/exec"
	"runtime/debug"
	"syscall"
	"unsafe"

	"github.com/getlantern/systray"
)

//go:embed icon.ico
var trayIcon []byte

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	getConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	showWindow       = user32.NewProc("ShowWindow")
)

const swHide = 0

// hideConsole 隐藏自身控制台窗口（托盘模式不留黑框）
func hideConsole() {
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, swHide)
	}
}

// RunTray 托盘模式：图标 + 右键菜单（打开 Hub / 立即同步 / 退出），主循环在后台
func RunTray(opt Options) error {
	hideConsole()
	openLogFile()
	log("托盘模式启动")
	onExit := func() {}
	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("Skill Hub")
		systray.SetTooltip("Skill Hub 客户端 — " + opt.Hostname)
		mOpen := systray.AddMenuItem("打开 Hub 页面", "在浏览器打开 "+opt.Server)
		mSync := systray.AddMenuItem("立即同步", "推送本机变更 + 应用云端合并 + 部署云端分配 skill")
		mUpload := systray.AddMenuItem("上传本机全部 skill", "强制全量推送所有监控目录（不依赖增量签名）")
		mPull := systray.AddMenuItem("下载云端分配 skill", "拉取后台分配给本设备的 skill 集合并落盘")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("退出", "停止客户端")
		if opt.Stop == nil {
			opt.Stop = make(chan struct{})
		}
		if opt.SyncNow == nil {
			opt.SyncNow = make(chan struct{})
		}
		if opt.UploadAll == nil {
			opt.UploadAll = make(chan struct{})
		}
		if opt.PullAssigned == nil {
			opt.PullAssigned = make(chan struct{})
		}
		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					exec.Command("rundll32", "url.dll,FileProtocolHandler", opt.Server).Start()
				case <-mSync.ClickedCh:
					select {
					case opt.SyncNow <- struct{}{}:
					default:
					}
				case <-mUpload.ClickedCh:
					select {
					case opt.UploadAll <- struct{}{}:
					default:
					}
				case <-mPull.ClickedCh:
					select {
					case opt.PullAssigned <- struct{}{}:
					default:
					}
				case <-mQuit.ClickedCh:
					close(opt.Stop)
					systray.Quit()
					return
				}
			}
		}()
		err := Run(opt)
		if err != nil {
			log("主循环退出: %v", err)
		}
		debug.FreeOSMemory()
		systray.Quit()
	}, onExit)
	return nil
}

var _ = fmt.Sprintf

// AlertNoConfig 未配置时弹原生消息框引导
func AlertNoConfig() {
	mbox := user32.NewProc("MessageBoxW")
	title, _ := syscall.UTF16PtrFromString("Skill Hub 客户端")
	text, _ := syscall.UTF16PtrFromString("此二进制尚未包含接入配置。\n\n请到 Hub 网页「设置 → 一键接入」下载带配置的客户端（双击即用），\n或使用命令行：skill-hub daemon --server URL --token TOKEN --tray")
	mbox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x40)
}
