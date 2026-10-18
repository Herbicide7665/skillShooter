package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"skillshooter/internal/cli"
	"skillshooter/internal/llm"
	"skillshooter/internal/server"
	"skillshooter/internal/store"
)

// skillshooter-server — 服务端（hub）。客户端程序见 skillshooter-client。

// applyLLMOverrides 设置页（meta 表）保存的 LLM 配置覆盖 config.yaml（env 在 cli.LoadConfig 里已最优先）
func applyLLMOverrides(cfg cli.Config, st *store.Store) llm.Config {
	out := llm.Config{APIKey: cfg.LLM.APIKey, BaseURL: cfg.LLM.BaseURL, Model: cfg.LLM.Model}
	for _, m := range []struct{ key string; dst *string }{
		{"llm_api_key", &out.APIKey},
		{"llm_base_url", &out.BaseURL},
		{"llm_model", &out.Model},
	} {
		if v, err := st.GetMeta(m.key); err == nil && v != "" {
			*m.dst = v
		}
	}
	return out
}

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] != "serve" {
		if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
			cli.UsageServer()
			return
		}
		fmt.Fprintf(os.Stderr, "skillshooter-server: 未知命令: %s（服务端只接受 serve）\n\n", args[0])
		cli.UsageServer()
		os.Exit(1)
	}
	var rest []string
	if len(args) > 1 {
		rest = args[1:]
	}
	cmdServe(rest)
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "0.0.0.0:8790", "监听地址")
	dbPath := fs.String("db", "", "SQLite 路径（默认二进制同目录 skillshooter.db）")
	fs.Parse(args)

	dir := cli.ExeDir()
	if *dbPath == "" {
		*dbPath = filepath.Join(dir, "skillshooter.db")
	}
	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()
	if tok, err := st.EnsureToken(); err != nil {
		fmt.Fprintf(os.Stderr, "生成 token 失败: %v\n", err)
		os.Exit(1)
	} else {
		fmt.Printf("[skillshooter] API token: %s\n", tok)
	}
	if pw, created, err := st.EnsureUsers(); err != nil {
		fmt.Fprintf(os.Stderr, "初始化用户失败: %v\n", err)
		os.Exit(1)
	} else if created {
		if pw == "" {
			pw = os.Getenv("SKILLSHOOTER_ADMIN_PASSWORD")
		}
		fmt.Printf("[skillshooter] 初始管理员账号: admin  密码: %s （仅此一次打印，请登录后尽快修改）\n", pw)
	}

	cfg := applyLLMOverrides(cli.LoadConfig(), st)
	lc := llm.New(cfg)
	srv := server.New(st, lc)

	mux := http.NewServeMux()
	srv.Serve(mux)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "监听失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[skillshooter] serving http://%s  db=%s  llm=%s(model=%s)\n",
		ln.Addr(), *dbPath, map[bool]string{true: "已配置", false: "未配置"}[cfg.APIKey != ""], cfg.Model)

	httpSrv := &http.Server{Handler: mux}
	go func() {
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "服务异常退出: %v\n", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
}
