package server

// v2.2 设备域测试：user token 绑定设备、认领锁死、设备域过滤、跨设备 403

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// newGzipW/ writeTarEntry 测试辅助
// newGzipW 测试辅助
func newGzipW(buf *bytes.Buffer) *gzip.Writer { return gzip.NewWriter(buf) }

// writeTarEntry 写一个 tar 条目并关闭 writer
func writeTarEntry(w io.Writer, name string, data []byte) {
	tw := tar.NewWriter(w)
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))})
	tw.Write(data)
	tw.Close()
}

// pushOneSkill 用 multipart push 一个含 SKILL.md 的 bundle（同 auth_test 的构造思路）
func pushOneSkill(t *testing.T, tsURL, tok, hostname, name, content string) int {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("hostname", hostname)
	fw, _ := mw.CreateFormFile("skills.tar.gz", "skills.tar.gz")
	// tar.gz
	var gz bytes.Buffer
	zw := newGzipW(&gz)
	writeTarEntry(zw, name+"/SKILL.md", []byte(content))
	zw.Close()
	fw.Write(gz.Bytes())
	mw.Close()

	req, _ := http.NewRequest("POST", tsURL+"/api/push", &buf)
	req.Header.Set("X-Token", tok)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("push: got %d body=%s", resp.StatusCode, string(body))
	}
	var out struct {
		Skills []struct {
			Skill   string `json:"skill"`
			Version int64  `json:"version_id"`
		} `json:"skills"`
	}
	json.Unmarshal(body, &out)
	for _, sk := range out.Skills {
		if sk.Skill == name {
			return int(sk.Version)
		}
	}
	t.Fatalf("pushed skill %s not in response: %s", name, string(body))
	return 0
}

func apiStatus(t *testing.T, method, tsURL, path, tok, body string) (int, string) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, tsURL+path, rd)
	if tok != "" {
		req.Header.Set("X-Token", tok)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// TestDeviceScope 全链路：两 token 两设备；认领锁死；列表/详情/下载域过滤；heartbeat 绑定校验
func TestDeviceScope(t *testing.T) {
	_, ts, adminTok := newTestServer(t)

	// 建 token 允许留空 hostname（待认领）
	st, body := apiStatus(t, "POST", ts.URL, "/api/tokens/create", adminTok, `{"role":"user","note":"d1"}`)
	if st != 200 {
		t.Fatalf("create d1 token: %d %s", st, body)
	}
	var tr1 struct{ Token string `json:"token"` }
	json.Unmarshal([]byte(body), &tr1)
	st, body = apiStatus(t, "POST", ts.URL, "/api/tokens/create", adminTok, `{"role":"user","note":"d2"}`)
	if st != 200 {
		t.Fatalf("create d2 token: %d %s", st, body)
	}
	var tr2 struct{ Token string `json:"token"` }
	json.Unmarshal([]byte(body), &tr2)


	// 1) 未绑定先 push：自动认领 dev1
	pushOneSkill(t, ts.URL, tr1.Token, "dev1", "alpha", "---\nname: alpha\ndescription: a\n---\nA")
	// 2) 同 token 换 hostname 再推：403
	st, body = apiStatus(t, "GET", ts.URL, "/api/whoami", tr1.Token, "")
	var wm struct{ Machine string `json:"machine"` }
	json.Unmarshal([]byte(body), &wm)
	if wm.Machine != "dev1" {
		t.Fatalf("after adoption whoami.machine: got %q want dev1", wm.Machine)
	}
	// push with wrong hostname via multipart would be大段；用 heartbeat 验证同逻辑
	st, _ = apiStatus(t, "POST", ts.URL, "/api/heartbeat", tr1.Token, `{"hostname":"evil"}`)
	if st != 403 {
		t.Fatalf("heartbeat wrong host: got %d want 403", st)
	}
	// 3) 正确心跳 200
	st, _ = apiStatus(t, "POST", ts.URL, "/api/heartbeat", tr1.Token, `{"hostname":"dev1"}`)
	if st != 200 {
		t.Fatalf("heartbeat own: got %d want 200", st)
	}

	// 4) token2 推 dev2 的 skill
	pushOneSkill(t, ts.URL, tr2.Token, "dev2", "beta", "---\nname: beta\ndescription: b\n---\nB")

	// 5) dev1 列表只见自己的 skill
	st, body = apiStatus(t, "GET", ts.URL, "/api/skills", tr1.Token, "")
	if st != 200 {
		t.Fatalf("d1 skills: %d %s", st, body)
	}
	var skills []struct {
		Name string `json:"name"`
		ID   int64  `json:"id"`
	}
	json.Unmarshal([]byte(body), &skills)
	if len(skills) != 1 || skills[0].Name != "alpha" {
		t.Fatalf("d1 scope: got %+v", skills)
	}
	alphaID := skills[0].ID
	st, body = apiStatus(t, "GET", ts.URL, "/api/skills", tr2.Token, "")
	json.Unmarshal([]byte(body), &skills)
	if len(skills) != 1 || skills[0].Name != "beta" {
		t.Fatalf("d2 scope: got %+v", skills)
	}
	betaID := skills[0].ID

	// 6) 跨设备读详情/下载 403
	st, _ = apiStatus(t, "GET", ts.URL, fmt.Sprintf("/api/skills/%d", betaID), tr1.Token, "")
	if st != 403 {
		t.Fatalf("d1 read beta detail: got %d want 403", st)
	}
	st, _ = apiStatus(t, "GET", ts.URL, fmt.Sprintf("/api/skills/%d/download", betaID), tr1.Token, "")
	if st != 403 {
		t.Fatalf("d1 download beta: got %d want 403", st)
	}
	// 自己的详情/下载 200
	st, _ = apiStatus(t, "GET", ts.URL, fmt.Sprintf("/api/skills/%d", alphaID), tr1.Token, "")
	if st != 200 {
		t.Fatalf("d1 read own detail: got %d want 200", st)
	}

	// 7) machines 只回本机
	st, body = apiStatus(t, "GET", ts.URL, "/api/machines", tr1.Token, "")
	if st != 200 {
		t.Fatalf("d1 machines: %d", st)
	}
	var ms []struct{ Hostname string `json:"hostname"` }
	json.Unmarshal([]byte(body), &ms)
	if len(ms) != 1 || ms[0].Hostname != "dev1" {
		t.Fatalf("d1 machines: got %+v", ms)
	}

	// 8) admin 全见（三 skill? 这里两台机器各1）且 user 会话全局只读回归
	st, body = apiStatus(t, "GET", ts.URL, "/api/skills", adminTok, "")
	json.Unmarshal([]byte(body), &skills)
	if len(skills) != 2 {
		t.Fatalf("admin sees all: got %d skills", len(skills))
	}

	// 9) rebind：admin 把 token2 改绑 dev1 → token2 变着 dev1 域（beta 不可见）
	// 拿掩码（ListTokens 输出）
	st, body = apiStatus(t, "GET", ts.URL, "/api/tokens", adminTok, "")
	var toks []struct{ Masked string `json:"token_masked"`; Hostname string `json:"hostname"` }
	json.Unmarshal([]byte(body), &toks)
	var masked2 string
	for _, tk := range toks {
		if tk.Hostname == "dev2" {
			masked2 = tk.Masked
		}
	}
	if masked2 == "" {
		t.Fatalf("no masked token for dev2: %+v", toks)
	}
	st, _ = apiStatus(t, "POST", ts.URL, "/api/tokens/rebind", adminTok, fmt.Sprintf(`{"token":%q,"hostname":"dev1"}`, masked2))
	if st != 200 {
		t.Fatalf("rebind: got %d", st)
	}
	// token2 现绑 dev1：skills 列表按 dev1 域过滤（只应见 alpha，不见 beta）
	st, body = apiStatus(t, "GET", ts.URL, "/api/skills", tr2.Token, "")
	if st != 200 {
		t.Fatalf("d2 after rebind: got %d want 200 (bound to dev1, scope-filtered)", st)
	}
	var reSkills []struct {
		Name string `json:"name"`
	}
	json.Unmarshal([]byte(body), &reSkills)
	for _, sk := range reSkills {
		if sk.Name == "beta" {
			t.Fatalf("rebound token2 still sees beta: %+v", reSkills)
		}
	}
}

// TestUserTokenCannotWeb user token 打任何管理页 API（需会话的路径）都拿不到 web 视角
func TestUserTokenCannotWeb(t *testing.T) {
	_, ts, adminTok := newTestServer(t)
	st, body := apiStatus(t, "POST", ts.URL, "/api/tokens/create", adminTok, `{"role":"user","note":"web"}`)
	if st != 200 {
		t.Fatalf("create token: %d %s", st, body)
	}
	var tr struct{ Token string `json:"token"` }
	json.Unmarshal([]byte(body), &tr)
	// 管理视图全部 403
	for _, path := range []string{"/api/stats", "/api/graph", "/api/families", "/api/dup/groups?status=all", "/api/edges"} {
		st, _ = apiStatus(t, "GET", ts.URL, path, tr.Token, "")
		if st != 403 {
			t.Fatalf("user token on %s: got %d want 403", path, st)
		}
	}
	// settings 不回主 token
	st, body = apiStatus(t, "GET", ts.URL, "/api/settings", tr.Token, "")
	if st != 200 {
		t.Fatalf("settings reachable: got %d", st)
	}
	var m map[string]interface{}
	json.Unmarshal([]byte(body), &m)
	if _, has := m["token"]; has {
		t.Fatalf("user token must not read admin token: %s", body)
	}
}
