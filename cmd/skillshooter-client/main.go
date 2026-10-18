package main

import (
	"fmt"
	"os"

	"skillshooter/internal/cli"
	"skillshooter/internal/daemon"
)

// skillshooter-client — 客户端（push/pull/daemon）。服务端程序见 skillshooter-server。

func main() {
	if len(os.Args) < 2 {
		// 无参数 = 客户端模式（双击已配置的二进制）
		cli.RunClientDefault()
		return
	}
	switch os.Args[1] {
	case "push":
		cli.CmdPush(os.Args[2:])
	case "upload":
		cli.CmdPush(os.Args[2:])
	case "pull":
		cli.CmdPull(os.Args[2:])
	case "daemon":
		cli.CmdDaemon(os.Args[2:])
	case "config":
		cli.CmdConfig(os.Args[2:])
	case "dlgtest":
		daemon.ShowConfigDialog(daemon.ClientConfig{})
		fmt.Println("dialog flow done")
	case "-h", "--help", "help":
		cli.UsageClient()
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", os.Args[1])
		cli.UsageClient()
		os.Exit(1)
	}
}
