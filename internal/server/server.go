package server

import (
	"archive/tar"
	"bytes"
	"context"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"skillshooter/internal/analysis"
	"skillshooter/internal/daemon"
	"skillshooter/internal/diffutil"
	"skillshooter/internal/llm"
	"skillshooter/internal/store"
)

type Server struct {
	st        *store.Store
	llm       *llm.Client
	nonces    *nonceCache
	loginGate loginGate
}

func New(st *store.Store, lc *llm.Client) *Server {
	return &Server{st: st, llm: lc, nonces: newNonceCache()}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	if w == nil {
		return // signedToken 在 principal() 预检路径下无 ResponseWriter，错误由调用方统一写
	}
	writeJSON(w, code, map[string]string{"error": msg})
}

// Serve HTTP 入口（含静态资源）
func (s *Server) Serve(mux *http.ServeMux) {
	mux.HandleFunc("/api/", s.handleAPI)
	mux.HandleFunc("/dl/", s.handleFileDL)
	mux.HandleFunc("/api/client/", s.handleClientDL)
	mux.HandleFunc("/", s.handleStatic)
}

// ---- 简易路由 ----

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api"))
	// 匿名端点：登录/登出/自检
	if len(parts) == 1 && parts[0] == "login" && r.Method == "POST" {
		s.handleLogin(w, r)
		return
	}
	if len(parts) == 1 && parts[0] == "logout" && r.Method == "POST" {
		if ck, err := r.Cookie("Session"); err == nil {
			s.st.DeleteSession(ck.Value)
		}
		http.SetCookie(w, &http.Cookie{Name: "Session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		writeJSON(w, 200, map[string]string{"ok": "1"})
		return
	}
	// whoami 自检：会话或 token 任一即可，返回身份供 UI/客户端判断
	if len(parts) == 1 && parts[0] == "whoami" && r.Method == "GET" {
		if ck, err := r.Cookie("Session"); err == nil {
			if _, username, role, ok := s.st.SessionPrincipal(ck.Value); ok {
				writeJSON(w, 200, map[string]interface{}{"ok": true, "role": role, "user": username, "auth": "session"})
				return
			}
		}
		ok, role := s.st.TokenRole(r.Header.Get("X-Token"))
		writeJSON(w, 200, map[string]interface{}{"ok": ok, "role": role, "auth": "token"})
		return
	}
	// 认证统一在 resolvePrincipal：会话 cookie > X-Sign 签名 > X-Token 明文（旧客户端兼容，待淘汰）
	// 公网部署：所有 API（含读）都要身份，防内容裸奔
	r, ok, _ := s.authGate(r)
	if !ok {
		writeErr(w, 401, "未认证（浏览器请登录，API 请带 token 或签名）")
		return
	}
	// parts[0] 是资源名
	switch {
	case len(parts) == 1 && parts[0] == "push" && r.Method == "POST":
		s.handlePush(w, r)
	case parts[0] == "stats" && r.Method == "GET":
		st, err := s.st.GetStats()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, st)
	case parts[0] == "machines" && r.Method == "GET":
		ms, err := s.st.ListMachines()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, ms)
	case parts[0] == "skills":
		s.handleSkills(w, r, parts)
	case parts[0] == "dup":
		s.handleDup(w, r, parts)
	case parts[0] == "graph" && r.Method == "GET":
		skills, err := s.st.ListSkills("", "")
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		edges, err := s.st.ListEdges()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]interface{}{"skills": skills, "edges": edges})
	case parts[0] == "edges":
		if len(parts) == 2 && parts[1] == "scan" && r.Method == "POST" {
			if !s.requireRole(w, r, true) {
				writeErr(w, 401, "token 无效")
				return
			}
			refN, err := analysis.ScanReferenceEdges(s.st)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			resp := map[string]interface{}{"reference_edges": refN}
			if s.llm.Config().Enabled() {
				ctx := r.Context()
				n, err := analysis.ScanLLMRelations(ctx, s.st, s.llm, 80)
				if err != nil {
					resp["llm_error"] = err.Error()
				} else {
					resp["llm_edges"] = n
				}
				fn, err := analysis.ScanFamilies(ctx, s.st, s.llm)
				if err != nil {
					resp["families_error"] = err.Error()
				} else {
					resp["families"] = fn
				}
			} else {
				resp["llm_error"] = "GLM 未配置"
			}
			writeJSON(w, 200, resp)
			return
		}
		edges, err := s.st.ListEdges()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, edges)
	case parts[0] == "families" && r.Method == "GET":
		// 只读固化结果，不触发任何 LLM/静态分析
		fams, err := s.st.ListFamilies()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out := []map[string]interface{}{}
		for _, f := range fams {
			names := []string{}
			members := []map[string]interface{}{}
			seen := map[string]bool{}
			for _, m := range f.Members {
				if !seen[m.Name] {
					seen[m.Name] = true
					names = append(names, m.Name)
				}
				members = append(members, map[string]interface{}{"id": m.ID, "name": m.Name, "machine": m.Machine})
			}
			out = append(out, map[string]interface{}{"key": f.Key, "note": f.Note, "kind": "llm", "names": names, "members": members})
		}
		writeJSON(w, 200, out)
	case parts[0] == "tokens" && r.Method == "GET":
		if !s.requireRole(w, r, true) {
			return
		}
		ts, err := s.st.ListTokens()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, ts)
	case parts[0] == "tokens" && len(parts) == 2 && parts[1] == "create" && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			return
		}
		var body struct {
			Role string `json:"role"`
			Note string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Role != "admin" && body.Role != "user") {
			writeErr(w, 400, "role 必须是 admin 或 user")
			return
		}
		tok, err := s.st.CreateRoleToken(body.Role, body.Note)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"token": tok, "role": body.Role})
	case parts[0] == "tokens" && len(parts) == 2 && parts[1] == "revoke" && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			return
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
			writeErr(w, 400, "需要 token")
			return
		}
		if err := s.st.RevokeToken(body.Token); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "1"})
	case parts[0] == "canonical" && r.Method == "GET":
		cs, err := s.st.CanonicalMerges()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, cs)
	case parts[0] == "heartbeat" && r.Method == "POST":
		s.handleHeartbeat(w, r)
	case parts[0] == "assignments":
		s.handleAssignments(w, r, parts)
	case parts[0] == "dispatches":
		s.handleDispatches(w, r, parts)
	case parts[0] == "jobs":
		s.handleJobs(w, r, parts)
	case parts[0] == "settings" && r.Method == "GET":
		cfg := s.llm.Config()
		out := map[string]interface{}{
			"llm": map[string]interface{}{
				"enabled": cfg.Enabled(), "base_url": cfg.BaseURL, "model": cfg.Model,
				"key_masked": maskKey(cfg.APIKey),
			},
		}
		// 管理员主 token 只回给 admin（user/匿名不可见，防提权读取）
		if ok, role := s.principal(r); ok && role == "admin" {
			tok, _ := s.st.EnsureToken()
			out["token"] = tok
		}
		writeJSON(w, 200, out)
	case parts[0] == "settings" && len(parts) == 2 && (parts[1] == "llm-test" || parts[1] == "glm-test") && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			return
		}
		if !s.llm.Config().Enabled() {
			writeErr(w, 400, "LLM 未配置（api_key 与 model 必填；config.yaml / env / 设置页任一来源）")
			return
		}
		out, err := s.llm.Test(r.Context())
		if err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"reply": out})
	case parts[0] == "settings" && len(parts) == 2 && parts[1] == "llm" && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			return
		}
		var body struct {
			APIKey  string `json:"api_key"`
			BaseURL string `json:"base_url"`
			Model   string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		cfg := s.llm.Config()
		if body.APIKey != "" { // 留空 = 保留现有 key
			cfg.APIKey = strings.TrimSpace(body.APIKey)
			s.st.SetMeta("llm_api_key", cfg.APIKey)
		}
		if body.BaseURL != "" {
			cfg.BaseURL = strings.TrimSpace(body.BaseURL)
			s.st.SetMeta("llm_base_url", cfg.BaseURL)
		}
		if body.Model != "" {
			cfg.Model = strings.TrimSpace(body.Model)
			s.st.SetMeta("llm_model", cfg.Model)
		}
		s.llm.SetConfig(cfg)
		writeJSON(w, 200, map[string]interface{}{"ok": true, "enabled": cfg.Enabled()})
	case parts[0] == "password" && r.Method == "POST":
		// 本人改密：仅会话身份（token 身份没有用户归属）
		ck, err := r.Cookie("Session")
		if err != nil {
			writeErr(w, 400, "改密需要登录会话")
			return
		}
		uid, _, _, ok := s.st.SessionPrincipal(ck.Value)
		if !ok {
			writeErr(w, 401, "会话已过期")
			return
		}
		var body struct {
			Old string `json:"old"`
			New string `json:"new"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.New == "" {
			writeErr(w, 400, "需要 old/new")
			return
		}
		if err := s.st.ChangePassword(uid, body.Old, body.New); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "1"})
	case parts[0] == "users" && r.Method == "GET":
		if !s.requireRole(w, r, true) {
			return
		}
		us, err := s.st.ListUsers()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, us)
	case parts[0] == "users" && len(parts) == 2 && parts[1] == "create" && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if body.Role == "" {
			body.Role = "user"
		}
		if err := s.st.CreateUser(body.Username, body.Password, body.Role); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "1"})
	case parts[0] == "users" && len(parts) == 2 && parts[1] == "delete" && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			return
		}
		var body struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.ID == 0 && body.Username == "") {
			writeErr(w, 400, "需要 id 或 username")
			return
		}
		id := body.ID
		if id == 0 {
			id = s.st.UserIDByName(body.Username)
		}
		if s.st.CountAdmins() == 1 && s.st.UserRoleByID(id) == "admin" {
			writeErr(w, 400, "不能删除最后一个管理员")
			return
		}
		if err := s.st.DeleteUser(id); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "1"})
	case parts[0] == "users" && len(parts) == 2 && parts[1] == "password" && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			return
		}
		var body struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
			writeErr(w, 400, "需要 password")
			return
		}
		id := body.ID
		if id == 0 && body.Username != "" {
			id = s.st.UserIDByName(body.Username)
		}
		if err := s.st.SetUserPassword(id, body.Password); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "1"})
	default:
		writeErr(w, 404, "未知 API")
	}
}

// maskKey API key 脱敏展示（前 4 后 4）
func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "****"
	}
	return k[:4] + "..." + k[len(k)-4:]
}

func splitPath(p string) []string {
	var out []string
	for _, seg := range strings.Split(p, "/") {
		if seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// ---- 认证 ----

// principalRole 每请求缓存一次身份解析结论（签名校验含 nonce 一次性消耗，
// 网关与 requireRole 多次解析必须复用同一结论，否则第二次会误判重放 401）
type principalCacheKey struct{}

// principal 读缓存的身份结论（须先经 authGate 解析）
func (s *Server) principal(r *http.Request) (bool, string) {
	if v, ok := r.Context().Value(principalCacheKey{}).([2]interface{}); ok {
		return v[0].(bool), v[1].(string)
	}
	return s.resolvePrincipal(r)
}

// authGate 网关校验：解析身份并写入 context 缓存
func (s *Server) authGate(r *http.Request) (*http.Request, bool, string) {
	ok, role := s.resolvePrincipal(r)
	if ok {
		r = r.WithContext(context.WithValue(r.Context(), principalCacheKey{}, [2]interface{}{ok, role}))
	}
	return r, ok, role
}

func (s *Server) resolvePrincipal(r *http.Request) (bool, string) {
	if ck, err := r.Cookie("Session"); err == nil {
		if _, _, role, ok := s.st.SessionPrincipal(ck.Value); ok {
			return true, role
		}
	}
	if r.Header.Get("X-Sign") != "" {
		tok, ok := s.signedToken(nil, r)
		if !ok {
			return false, ""
		}
		_, role := s.st.TokenRole(tok)
		return true, role
	}
	return s.st.TokenRole(r.Header.Get("X-Token"))
}

// requireRole 通用门：校验并写 401/403
func (s *Server) requireRole(w http.ResponseWriter, r *http.Request, admin bool) bool {
	ok, role := s.principal(r)
	if !ok {
		writeErr(w, 401, "未认证（浏览器请登录，API 请带 token 或签名）")
		return false
	}
	if admin && role != "admin" {
		writeErr(w, 403, "需要管理员权限")
		return false
	}
	return true
}

// ---- skills ----

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case len(parts) == 1 && r.Method == "GET":
		q := r.URL.Query().Get("q")
		machine := r.URL.Query().Get("machine")
		ss, err := s.st.ListSkills(q, machine)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, ss)
	case len(parts) == 2 && r.Method == "GET":
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			writeErr(w, 400, "bad id")
			return
		}
		sk, err := s.st.GetSkill(id)
		if err != nil {
			writeErr(w, 404, "not found")
			return
		}
		vers, _ := s.st.ListVersions(id)
		edges, _ := s.st.ListEdges()
		var in, out []store.Edge
		for _, e := range edges {
			if e.SrcID == id {
				out = append(out, e)
			}
			if e.DstID == id {
				in = append(in, e)
			}
		}
		writeJSON(w, 200, map[string]interface{}{"skill": sk, "versions": vers, "edges_out": out, "edges_in": in})
	case len(parts) == 3 && parts[2] == "file" && r.Method == "GET":
		vid, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
		if err != nil {
			writeErr(w, 400, "bad version")
			return
		}
		path := r.URL.Query().Get("path")
		data, err := s.st.ReadSkillVersion(vid, path)
		if err != nil {
			writeErr(w, 404, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(data)
	case len(parts) == 3 && parts[2] == "versions" && r.Method == "GET":
		id, _ := strconv.ParseInt(parts[1], 10, 64)
		vs, err := s.st.ListVersions(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, vs)
	case len(parts) == 3 && parts[2] == "tree" && r.Method == "GET":
		id, _ := strconv.ParseInt(parts[1], 10, 64)
		vid, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
		if err != nil {
			vers, _ := s.st.ListVersions(id)
			if len(vers) == 0 {
				writeErr(w, 404, "no versions")
				return
			}
			vid = vers[len(vers)-1].ID
		}
		files, err := s.st.ListVersionFiles(vid)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, files)
	case len(parts) == 3 && parts[2] == "system" && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			return
		}
		var body struct {
			IsSystem bool `json:"is_system"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			writeErr(w, 400, "bad id")
			return
		}
		if err := s.st.SetSkillSystem(id, body.IsSystem); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case len(parts) == 3 && parts[2] == "diff" && r.Method == "GET":
		id, _ := strconv.ParseInt(parts[1], 10, 64)
		from, err1 := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
		to, err2 := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
		if err1 != nil || err2 != nil {
			writeErr(w, 400, "需要 from/to 版本号")
			return
		}
		sk, err := s.st.GetSkill(id)
		if err != nil {
			writeErr(w, 404, "not found")
			return
		}
		diff, err := s.versionDiff(sk.Name, from, to)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"diff": diff})
	case len(parts) == 3 && parts[2] == "download" && r.Method == "GET":
		s.handleVersionDownload(w, r, parts[1])
	default:
		writeErr(w, 404, "未知 skills API")
	}
}

// handleVersionDownload 打包某 skill 某版本的全部文件为 tar.gz（pull 用）
func (s *Server) handleVersionDownload(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeErr(w, 400, "bad id")
		return
	}
	sk, err := s.st.GetSkill(id)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	vid := sk.LatestVer
	if v := r.URL.Query().Get("version"); v != "" {
		vid, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeErr(w, 400, "bad version")
			return
		}
	}
	files, err := s.st.ListVersionFiles(vid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for path, bh := range files {
		data, err := s.st.ReadBlob(bh)
		if err != nil {
			continue
		}
		if err := tw.WriteHeader(&tar.Header{
			Name: path, Mode: 0o644, Size: int64(len(data)), ModTime: time.Now(),
		}); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if _, err := tw.Write(data); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	tw.Close()
	gz.Close()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.tar.gz", sk.Name))
	w.Write(buf.Bytes())
}

// versionDiff 对两个版本的 SKILL.md 生成 unified diff
func (s *Server) versionDiff(name string, from, to int64) (string, error) {
	a, err := s.st.ReadSkillVersion(from, "SKILL.md")
	if err != nil {
		return "", fmt.Errorf("读 from 版本失败: %w", err)
	}
	b, err := s.st.ReadSkillVersion(to, "SKILL.md")
	if err != nil {
		return "", fmt.Errorf("读 to 版本失败: %w", err)
	}
	// 无实质变化则给个提示
	if string(a) == string(b) {
		return "", nil
	}
	return diffutil.Unified(fmt.Sprintf("%s@v%d", name, from), fmt.Sprintf("%s@v%d", name, to), string(a), string(b), 3), nil
}

// ---- push ----

// handlePush 接收 tar.gz 打包的 skill 集合（entry 名 = "<skillDir>/<relPath>"）
// 认证已在 handleAPI 统一处理（X-Sign 签名或 X-Token 明文），此处不再重复校验
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	hostname := r.FormValue("hostname")
	if hostname == "" {
		hostname = "unknown"
	}
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeErr(w, 400, "解析上传失败: "+err.Error())
		return
	}
	var bundleFile *multipart.FileHeader
	for _, headers := range r.MultipartForm.File {
		for _, h := range headers {
			if h.Filename == "skills.tar.gz" || bundleFile == nil {
				bundleFile = h
			}
		}
	}
	if bundleFile == nil {
		writeErr(w, 400, "缺少 bundle 文件")
		return
	}
	f, err := bundleFile.Open()
	if err != nil {
		writeErr(w, 400, "读 bundle 失败: "+err.Error())
		return
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		writeErr(w, 400, "gzip 解压失败: "+err.Error())
		return
	}
	defer gz.Close()

	allFiles := map[string][]byte{}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, 400, "tar 解析失败: "+err.Error())
			return
		}
		name := strings.ReplaceAll(hdr.Name, "\\", "/")
		name = strings.TrimPrefix(name, "./")
		// 防路径穿越
		if strings.Contains(name, "..") || strings.HasPrefix(name, "/") || hdr.Typeflag == tar.TypeDir {
			continue
		}
		if strings.HasSuffix(name, "/") {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			continue
		}
		allFiles[name] = data
	}

	// 来源根目录 → 系统 skill 分类（ZCode/Claude Code 内置插件缓存下的算系统）
	isSys := store.ClassifySystem(r.FormValue("source_root"))

	// 每个 SKILL.md 所在目录即一个 skill；每个文件归属其最近祖先 skill 目录
	skillDirs := map[string]bool{}
	for path := range allFiles {
		if strings.HasSuffix(path, "/SKILL.md") {
			skillDirs[path[:len(path)-len("/SKILL.md")]] = true
		}
	}
	bySkill := map[string]map[string][]byte{}
	for path, data := range allFiles {
		// 沿祖先链从近到远找第一个 skill 目录
		dir := path
		owner := ""
		for {
			i := strings.LastIndex(dir, "/")
			if i < 0 {
				dir = ""
			} else {
				dir = dir[:i]
			}
			if skillDirs[dir] {
				owner = dir
				break
			}
			if dir == "" {
				break
			}
		}
		if owner == "" {
			continue
		}
		if bySkill[owner] == nil {
			bySkill[owner] = map[string][]byte{}
		}
		rel := strings.TrimPrefix(path, owner+"/")
		bySkill[owner][rel] = data
	}

	type pushed struct {
		Skill   string `json:"skill"`
		Version int64  `json:"version_id"`
		Created bool   `json:"created"`
	}
	results := []pushed{}
	for dirName, files := range bySkill {
		if _, ok := files["SKILL.md"]; !ok {
			continue
		}
		name, desc := parseFrontmatter(string(files["SKILL.md"]))
		if name == "" {
			// fallback 用目录名（可能是嵌套路径，取最后一段）
			seg := strings.Split(dirName, "/")
			name = seg[len(seg)-1]
		}
		sid, vid, created, err := s.st.SaveSkill(hostname, name, desc, files, isSys)
		if err != nil {
			writeErr(w, 500, "保存 "+name+" 失败: "+err.Error())
			return
		}
		results = append(results, pushed{Skill: name, Version: vid, Created: created})
		_ = sid
	}
	writeJSON(w, 200, map[string]interface{}{"machine": hostname, "skills": results, "count": len(results)})
	debug.FreeOSMemory() // 大包推送后立即归还峰值内存
}

// parseFrontmatter 从 SKILL.md 解析 name/description
func parseFrontmatter(md string) (name, desc string) {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}
	inFM := false
	for i := 1; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		if l == "---" {
			inFM = true
			break
		}
		if strings.HasPrefix(l, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(l, "name:"))
		} else if strings.HasPrefix(l, "description:") {
			desc = strings.TrimSpace(strings.TrimPrefix(l, "description:"))
			// 处理 yaml 折叠标记
			desc = strings.Trim(desc, `"'`)
			if desc == ">" || desc == "|" || desc == ">-" || desc == "|-" {
				// 继续收后续缩进行
				var parts []string
				for j := i + 1; j < len(lines); j++ {
					nl := lines[j]
					if strings.TrimSpace(nl) == "---" || !strings.HasPrefix(nl, " ") {
						break
					}
					parts = append(parts, strings.TrimSpace(nl))
					i = j
				}
				desc = strings.Join(parts, " ")
			}
		}
	}
	_ = inFM
	return name, desc
}

// ---- dup ----

func (s *Server) handleDup(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case len(parts) == 2 && parts[1] == "scan" && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			writeErr(w, 401, "token 无效")
			return
		}
		exactN, err := analysis.ScanExact(s.st)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		resp := map[string]interface{}{"exact_groups": exactN}
		ctx := r.Context()
		fuzzyN, err := analysis.ScanFuzzy(ctx, s.st, s.llm, 0.55)
		if err != nil {
			resp["fuzzy_error"] = err.Error()
		} else {
			resp["fuzzy_groups"] = fuzzyN
		}
		writeJSON(w, 200, resp)
	case (len(parts) == 1 || (len(parts) == 2 && parts[1] == "groups")) && r.Method == "GET":
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "all"
		}
		gs, err := s.st.ListDupGroups(status)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, gs)
	case len(parts) == 4 && parts[1] == "groups" && parts[3] == "merge-proposal" && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			writeErr(w, 401, "token 无效")
			return
		}
		gid, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			writeErr(w, 400, "bad id")
			return
		}
		p, err := analysis.ProposeMerge(r.Context(), s.st, s.llm, gid)
		if err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, p)
	case len(parts) == 4 && parts[1] == "groups" && parts[3] == "adopt" && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			writeErr(w, 401, "token 无效")
			return
		}
		gid, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			writeErr(w, 400, "bad id")
			return
		}
		var body struct {
			SkillMD     string `json:"skill_md"`
			Description string `json:"description"`
			Notes       string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		vid, err := analysis.AdoptMerge(r.Context(), s.st, gid, &analysis.MergeProposal{
			SkillMD: body.SkillMD, Description: body.Description, Notes: body.Notes,
		})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]int64{"version_id": vid})
	case len(parts) == 4 && parts[1] == "groups" && parts[3] == "dismiss" && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			writeErr(w, 401, "token 无效")
			return
		}
		gid, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			writeErr(w, 400, "bad id")
			return
		}
		if err := s.st.SetDupGroupStatus(gid, "dismissed"); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "1"})
	default:
		writeErr(w, 404, "未知 dup API")
	}
}

// ---- 静态资源 ----

//go:embed all:static
var staticFS embed.FS

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	data, err := staticFS.ReadFile("static/" + path)
	if err != nil {
		// SPA fallback
		data, err = staticFS.ReadFile("static/index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		path = "index.html"
	}
	w.Header().Set("Cache-Control", "no-cache") // UI 更新立即生效，防浏览器陈旧缓存
	switch filepath.Ext(path) {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".sh":
		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	case ".ps1":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Write(data)
}

// handleFileDL 分发二进制本体（免 token，新机器一条命令接入用）
func (s *Server) handleFileDL(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/dl/")
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") ||
		!strings.HasPrefix(name, "skillshooter") {
		writeErr(w, 403, "只允许下载 skillshooter 二进制")
		return
	}
	ex, err := os.Executable()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(ex), name))
	if err != nil {
		writeErr(w, 404, "文件不存在")
		return
	}
	ct := "application/octet-stream"
	if strings.HasSuffix(name, ".exe") {
		ct = "application/vnd.microsoft.portable-executable"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	w.Write(data)
}

// ---- 远程调用 jobs ----

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case len(parts) == 1 && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			return
		}
		var body struct {
			Machine string `json:"machine"`
			Skill   string `json:"skill"`
			Prompt  string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Machine == "" || strings.TrimSpace(body.Prompt) == "" {
			writeErr(w, 400, "需要 machine 与 prompt")
			return
		}
		id, err := s.st.CreateJob(body.Machine, body.Skill, body.Prompt)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]int64{"id": id})
	case len(parts) == 1 && r.Method == "GET":
		jobs, err := s.st.ListJobs(50)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, jobs)
	case len(parts) == 2 && parts[1] == "next" && r.Method == "GET":
		machine := r.URL.Query().Get("machine")
		if machine == "" {
			writeErr(w, 400, "需要 machine")
			return
		}
		if _, role := s.st.TokenRole(r.Header.Get("X-Token")); role != "admin" {
			writeJSON(w, 200, map[string]interface{}{"id": 0}) // user 角色 daemon 不领取远程任务
			return
		}
		job, err := s.st.NextJob(machine)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if job == nil {
			writeJSON(w, 200, map[string]interface{}{"id": 0})
			return
		}
		writeJSON(w, 200, job)
	case len(parts) == 2 && r.Method == "GET":
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			writeErr(w, 400, "bad id")
			return
		}
		job, err := s.st.GetJob(id)
		if err != nil {
			writeErr(w, 404, "not found")
			return
		}
		writeJSON(w, 200, job)
	case len(parts) == 3 && parts[2] == "result" && r.Method == "POST":
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			writeErr(w, 400, "bad id")
			return
		}
		var body struct {
			Status   string                 `json:"status"`
			AgentCLI string                 `json:"agent_cli"`
			Report   map[string]interface{} `json:"report"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		repJSON, _ := json.Marshal(body.Report)
		status := body.Status
		if status == "" {
			status = "done"
		}
		if err := s.st.FinishJob(id, status, body.AgentCLI, string(repJSON)); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "1"})
	default:
		writeErr(w, 404, "未知 jobs API")
	}
}

// ---- 已配置客户端下载（二进制尾部追加 server/token，双击即用）----

func (s *Server) handleClientDL(w http.ResponseWriter, r *http.Request) {
	// 该响应含 token，必须认证后才能取：会话（管理员后台下载）或签名请求（客户端自更新）
	var reqRole, embedToken string
	if r.Header.Get("X-Sign") != "" {
		tok, ok := s.signedToken(w, r)
		if !ok {
			return
		}
		_, reqRole = s.st.TokenRole(tok)
		embedToken = tok
	} else if ck, err := r.Cookie("Session"); err == nil {
		if _, _, role, ok := s.st.SessionPrincipal(ck.Value); ok {
			reqRole = role
		}
	} else if ok, role := s.st.TokenRole(r.Header.Get("X-Token")); ok {
		reqRole = role
		embedToken = r.Header.Get("X-Token")
	}
	if reqRole == "" {
		writeErr(w, 401, "未认证")
		return
	}
	if embedToken == "" {
		// 会话（浏览器）下载：admin 会话内嵌主 token；user 会话只能拿与自身角色一致的既有 token
		if reqRole != "admin" {
			writeErr(w, 403, "浏览器下载客户端请让管理员操作（避免 user 客户端提权）")
			return
		}
		embedToken, _ = s.st.EnsureToken()
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/client/")
	allowed := map[string]bool{
		"skillshooter.exe": true, "skillshooter": true,
		"skillshooter-linux-amd64": true, "skillshooter-linux-arm64": true,
		"skillshooter-darwin-amd64": true, "skillshooter-darwin-arm64": true,
	}
	if !allowed[name] {
		writeErr(w, 403, "非法文件名")
		return
	}
	exe, err := os.Executable()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	bin, err := os.ReadFile(filepath.Join(filepath.Dir(exe), name))
	if err != nil {
		writeErr(w, 404, "服务端缺少该平台二进制: "+err.Error())
		return
	}
	// 内嵌"请求者自己的 token"：普通用户下载的客户端带 user token（不能下发任务），
	// 若用管理员主 token 内嵌则任何持有 user token 的人都能提权下载到 admin 客户端
	if _, embedRole := s.st.TokenRole(embedToken); embedRole == "admin" && reqRole != "admin" {
		writeErr(w, 403, "user 身份不能下载内嵌 admin token 的客户端")
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	server := scheme + "://" + r.Host
	out, err := daemon.AppendConfig(bin, server, embedToken)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	w.Write(out)
}

// ---- 心跳探活（30min 间隔；响应捎带待执行下发指令，见 PROTOCOL.md）----

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Hostname string `json:"hostname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Hostname == "" {
		writeErr(w, 400, "需要 hostname")
		return
	}
	if err := s.st.Heartbeat(body.Hostname); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 捎带：把该机器 pending 的下发指令一并认领返回（省一次轮询）
	dispatches, err := s.st.PendingDispatches(body.Hostname)
	if err != nil {
		dispatches = []store.Dispatch{}
	}
	writeJSON(w, 200, map[string]interface{}{
		"ok":          true,
		"server_time": time.Now().UTC().Format(time.RFC3339),
		"offline_after_min": int(store.OfflineAfter.Minutes()),
		"dispatches":  dispatches,
	})
}

// ---- 设备-skill 分配 ----

func (s *Server) handleAssignments(w http.ResponseWriter, r *http.Request, parts []string) {
	// GET /api/assignments?machine=<hostname> — 列某机器分配（user 可读，客户端拉取用）
	if len(parts) == 1 && r.Method == "GET" {
		machine := r.URL.Query().Get("machine")
		if machine == "" {
			writeErr(w, 400, "需要 machine")
			return
		}
		mid, err := s.st.MachineIDByName(machine)
		if err != nil {
			writeJSON(w, 200, []store.SkillRow{})
			return
		}
		ss, err := s.st.AssignedSkills(mid)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, ss)
		return
	}
	// PUT /api/assignments/<machine> — 全量设置分配（admin）
	if len(parts) == 2 && r.Method == "PUT" {
		if !s.requireRole(w, r, true) {
			return
		}
		mid, err := s.st.MachineIDByName(parts[1])
		if err != nil {
			writeErr(w, 404, "机器不存在: "+parts[1])
			return
		}
		var body struct {
			SkillIDs []int64 `json:"skill_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if err := s.st.SetAssignments(mid, body.SkillIDs); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]interface{}{"ok": true, "count": len(body.SkillIDs)})
		return
	}
	writeErr(w, 404, "未知 assignments API")
}

// ---- 下发指令（后台主动发起同步/部署）----

func (s *Server) handleDispatches(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	// POST /api/dispatches {machine, action} — 后台下发（admin）
	case len(parts) == 1 && r.Method == "POST":
		if !s.requireRole(w, r, true) {
			return
		}
		var body struct {
			Machine string `json:"machine"`
			Action  string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Machine == "" {
			writeErr(w, 400, "需要 machine")
			return
		}
		if body.Action == "" {
			body.Action = "sync"
		}
		if body.Action != "sync" && body.Action != "pull" {
			writeErr(w, 400, "action 只支持 sync（同步合并+部署分配）或 pull（仅拉取部署）")
			return
		}
		id, err := s.st.CreateDispatch(body.Machine, body.Action)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]int64{"id": id})
	// GET /api/dispatches?machine=x — 客户端认领 pending 指令（心跳外的补拉通道）
	case len(parts) == 1 && r.Method == "GET":
		machine := r.URL.Query().Get("machine")
		if machine == "" {
			writeErr(w, 400, "需要 machine")
			return
		}
		ds, err := s.st.PendingDispatches(machine)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, ds)
	// POST /api/dispatches/<id>/result — 客户端回执
	case len(parts) == 3 && parts[2] == "result" && r.Method == "POST":
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			writeErr(w, 400, "bad id")
			return
		}
		var body struct {
			Status string `json:"status"`
			Result string `json:"result"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if body.Status == "" {
			body.Status = "done"
		}
		if err := s.st.FinishDispatch(id, body.Status, body.Result); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "1"})
	// GET /api/dispatches/list — 后台查看最近指令（admin）
	case len(parts) == 2 && parts[1] == "list" && r.Method == "GET":
		if !s.requireRole(w, r, true) {
			return
		}
		ds, err := s.st.ListDispatches(50)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, ds)
	default:
		writeErr(w, 404, "未知 dispatches API")
	}
}
