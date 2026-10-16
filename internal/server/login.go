package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// handleLogin 用户名口令登录 → HttpOnly 会话 cookie。
// 带简单防爆破：同一用户名连续失败 5 次锁 60 秒。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" {
		writeErr(w, 400, "需要 username/password")
		return
	}
	if !s.loginGate.allow(body.Username) {
		writeErr(w, 429, "失败次数过多，请 1 分钟后再试")
		return
	}
	uid, role, err := s.st.VerifyLogin(body.Username, body.Password)
	if err != nil {
		s.loginGate.fail(body.Username)
		writeErr(w, 401, err.Error())
		return
	}
	s.loginGate.reset(body.Username)
	tok, err := s.st.CreateSession(uid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "Session", Value: tok, Path: "/",
		MaxAge: 30 * 24 * 3600, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, 200, map[string]interface{}{"ok": true, "role": role, "user": body.Username})
}

// loginGate 极简登录限速（内存态，重启清零）
type loginGate struct {
	mu    sync.Mutex
	fails map[string]*failInfo
}

type failInfo struct {
	n     int
	until time.Time
}

const (
	maxFails    = 5
	lockoutSecs = 60
)

func (g *loginGate) allow(user string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fails == nil {
		g.fails = map[string]*failInfo{}
	}
	fi, ok := g.fails[user]
	return !ok || time.Now().After(fi.until)
}

func (g *loginGate) fail(user string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fails == nil {
		g.fails = map[string]*failInfo{}
	}
	fi := g.fails[user]
	if fi == nil {
		fi = &failInfo{}
		g.fails[user] = fi
	}
	fi.n++
	if fi.n >= maxFails {
		fi.until = time.Now().Add(lockoutSecs * time.Second)
		fi.n = 0
	}
}

func (g *loginGate) reset(user string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fails, user)
}
