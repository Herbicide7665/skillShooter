package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"skill-manager/internal/llm"
	"skill-manager/internal/store"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server, string) {
	t.Helper()
	os.Setenv("SKILLMANAGER_ADMIN_PASSWORD", adminTestPassword)
	t.Cleanup(func() { os.Unsetenv("SKILLMANAGER_ADMIN_PASSWORD") })
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, created, err := st.EnsureUsers(); err != nil || !created {
		t.Fatalf("EnsureUsers: created=%v err=%v", created, err)
	}
	tok, err := st.EnsureToken()
	if err != nil {
		t.Fatal(err)
	}
	srv := New(st, llm.New(llm.Config{}))
	ts := httptest.NewServer(http.HandlerFunc(srv.handleAPI))
	t.Cleanup(ts.Close)
	return srv, ts, tok
}

// signReq 按 PROTOCOL.md §2 构造签名请求
func signReq(t *testing.T, token, method, path, body string) *http.Request {
	t.Helper()
	bs := sha256.Sum256([]byte(body))
	bodySHA := hex.EncodeToString(bs[:])
	ts := fmt.Sprintf("%d", time.Now().Unix())
	nonce := "nonce-" + token[:8] + "-" + body[:8] + path
	signStr := strings.Join([]string{method, path, ts, nonce, bodySHA}, "\n")
	m := hmac.New(sha256.New, []byte(token))
	m.Write([]byte(signStr))
	mac := m.Sum(nil)
	req, _ := http.NewRequest(method, "http://x"+path, strings.NewReader(body))
		kid := sha256.Sum256([]byte(token))
	req.Header.Set("X-Key-ID", hex.EncodeToString(kid[:])[:16])
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Body-SHA256", bodySHA)
	req.Header.Set("X-Sign", hex.EncodeToString(mac))
	return req
}

// TestSignedAdminEndpoints 修复回归：X-Sign 签名请求打 admin 端点不再因角色判定读空 X-Token 而 401
func TestSignedAdminEndpoints(t *testing.T) {
	_, ts, tok := newTestServer(t)

	// 签名 admin 请求：POST /api/dispatches（此前会误判 401）
	req := signReq(t, tok, "POST", "/api/dispatches", `{"machine":"t","action":"pull"}`)
	u, _ := url.Parse(ts.URL + "/api/dispatches")
	req.URL = u
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("signed admin dispatch: got %d want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestSignedReplayRejected 防重放：同一 nonce 二次使用必须 401
func TestSignedReplayRejected(t *testing.T) {
	_, ts, tok := newTestServer(t)
	req := signReq(t, tok, "POST", "/api/dispatches", `{"machine":"t","action":"pull"}`)
	u, _ := url.Parse(ts.URL + "/api/dispatches")
	req.URL = u
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	req2 := signReq(t, tok, "POST", "/api/dispatches", `{"machine":"t","action":"pull"}`)
	// 强制同 nonce：重放
	req2.Header.Set("X-Nonce", req.Header.Get("X-Nonce"))
	u2, _ := url.Parse(ts.URL + "/api/dispatches")
	req2.URL = u2
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Fatalf("replay: got %d want 401", resp2.StatusCode)
	}
}

// TestLoginSessionAndRoles 会话登录 + admin/user 角色隔离
func TestLoginSessionAndRoles(t *testing.T) {
	_, ts, tok := newTestServer(t)
	client := &http.Client{}

	// 错误密码 401
	resp, err := client.Post(ts.URL+"/api/login", "application/json",
		bytes.NewBufferString(`{"username":"admin","password":"wrong"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("wrong password: got %d want 401", resp.StatusCode)
	}

	// 正确密码 → cookie 会话
	resp, err = client.Post(ts.URL+"/api/login", "application/json",
		bytes.NewBufferString(`{"username":"admin","password":"`+adminTestPassword+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	var lr struct{ Role string `json:"role"` }
	json.NewDecoder(resp.Body).Decode(&lr)
	resp.Body.Close()
	if lr.Role != "admin" {
		t.Fatalf("login role: got %q want admin", lr.Role)
	}
	ck := resp.Cookies()
	if len(ck) == 0 || ck[0].Name != "Session" {
		t.Fatal("no session cookie")
	}

	// 会话访问 admin 端点 200
	req, _ := http.NewRequest("GET", ts.URL+"/api/users", nil)
	req.AddCookie(ck[0])
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("session admin users: got %d want 200", resp.StatusCode)
	}

	// 创建 user token，user 角色不能写 admin 端点
	utok := createUserToken(t, ts.URL, tok)
	req, _ = http.NewRequest("POST", ts.URL+"/api/dispatches",
		bytes.NewBufferString(`{"machine":"t","action":"pull"}`))
	req.Header.Set("X-Token", utok)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("user dispatch: got %d want 403", resp.StatusCode)
	}

	// user token 读端点 200；jobs/next 返回空任务（不领取）
	req, _ = http.NewRequest("GET", ts.URL+"/api/stats", nil)
	req.Header.Set("X-Token", utok)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("user stats: got %d want 200", resp.StatusCode)
	}

	// 匿名全拒
	resp, err = client.Get(ts.URL + "/api/stats")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("anon stats: got %d want 401", resp.StatusCode)
	}
}

// TestLLMCustomConfig 设置页保存自定义 LLM 配置并热生效（OpenAI 兼容 mock）
func TestLLMCustomConfig(t *testing.T) {
	srv, ts, tok := newTestServer(t)

	// mock OpenAI 兼容端点
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"正常"}}]}`))
	}))
	defer mock.Close()

	client := &http.Client{}
	save, _ := json.Marshal(map[string]string{
		"api_key": "test-key-123", "base_url": mock.URL + "/v1", "model": "mock-model",
	})
	req, _ := http.NewRequest("POST", ts.URL+"/api/settings/llm", bytes.NewReader(save))
	req.Header.Set("X-Token", tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("save llm: got %d", resp.StatusCode)
	}
	if !srv.llm.Config().Enabled() {
		t.Fatal("llm not enabled after save")
	}
	if srv.llm.Config().BaseURL != mock.URL+"/v1" {
		t.Fatalf("base_url: got %q", srv.llm.Config().BaseURL)
	}
	// 测试连通性走 mock
	req, _ = http.NewRequest("POST", ts.URL+"/api/settings/llm-test", nil)
	req.Header.Set("X-Token", tok)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var tr struct{ Reply string `json:"reply"` }
	json.NewDecoder(resp.Body).Decode(&tr)
	resp.Body.Close()
	if tr.Reply != "正常" {
		t.Fatalf("llm-test reply: got %q", tr.Reply)
	}
	// settings 里 key 脱敏
	req, _ = http.NewRequest("GET", ts.URL+"/api/settings", nil)
	req.Header.Set("X-Token", tok)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var sr struct {
		LLM struct{ KeyMasked string `json:"key_masked"` } `json:"llm"`
	}
	json.NewDecoder(resp.Body).Decode(&sr)
	resp.Body.Close()
	if sr.LLM.KeyMasked == "" || strings.Contains(sr.LLM.KeyMasked, "test-key-123") {
		t.Fatalf("key not masked: %q", sr.LLM.KeyMasked)
	}
}

// TestVersionRetention 推送两次不同内容 → 两个版本都在，旧版本文件仍可读
func TestVersionRetention(t *testing.T) {
	_, ts, tok := newTestServer(t)
	client := &http.Client{}

	push := func(content string) {
		bundle := makeTarGz(t, map[string][]byte{
			"demo/SKILL.md": []byte("---\nname: demo\ndescription: test\n---\n\n# " + content),
		})
		req, _ := http.NewRequest("POST", ts.URL+"/api/push", bundle)
		req.Header.Set("X-Token", tok)
		req.Header.Set("Content-Type", "multipart/form-data; boundary=BOUND")
		// 手工拼 multipart
		var buf bytes.Buffer
		buf.WriteString("--BOUND\r\nContent-Disposition: form-data; name=\"hostname\"\r\n\r\ntest-host\r\n")
		buf.WriteString("--BOUND\r\nContent-Disposition: form-data; name=\"bundle\"; filename=\"skills.tar.gz\"\r\nContent-Type: application/gzip\r\n\r\n")
		buf.Write(bundleBytes(t, bundle))
		buf.WriteString("\r\n--BOUND--\r\n")
		req, _ = http.NewRequest("POST", ts.URL+"/api/push", &buf)
		req.Header.Set("X-Token", tok)
		req.Header.Set("Content-Type", "multipart/form-data; boundary=BOUND")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("push: got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	push("v1")
	push("v2")

	// 两个版本都留存
	req, _ := http.NewRequest("GET", ts.URL+"/api/skills", nil)
	req.Header.Set("X-Token", tok)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var skills []struct {
		ID           int64 `json:"id"`
		VersionCount int   `json:"version_count"`
	}
	json.NewDecoder(resp.Body).Decode(&skills)
	resp.Body.Close()
	if len(skills) != 1 || skills[0].VersionCount != 2 {
		t.Fatalf("version retention: got %+v", skills)
	}

	// 旧版本内容仍可读
	req, _ = http.NewRequest("GET", ts.URL+"/api/skills/1/versions", nil)
	req.Header.Set("X-Token", tok)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var vers []struct{ ID int64 `json:"id"` }
	json.NewDecoder(resp.Body).Decode(&vers)
	resp.Body.Close()
	if len(vers) != 2 {
		t.Fatalf("versions list: got %d", len(vers))
	}
}

// ---- helpers ----

const adminTestPassword = "test-admin-pw-123"

func createUserToken(t *testing.T, tsURL, adminTok string) string {
	t.Helper()
	req, _ := http.NewRequest("POST", tsURL+"/api/tokens/create",
		bytes.NewBufferString(`{"role":"user","note":"test"}`))
	req.Header.Set("X-Token", adminTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var r struct{ Token string `json:"token"` }
	json.NewDecoder(resp.Body).Decode(&r)
	if r.Token == "" {
		t.Fatal("no user token returned")
	}
	return r.Token
}

func makeTarGz(t *testing.T, files map[string][]byte) *bytes.Buffer {
	t.Helper()
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	tw := tar.NewWriter(zw)
	for name, data := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))})
		_, _ = tw.Write(data)
	}
	tw.Close()
	zw.Close()
	return &gz
}

func bundleBytes(t *testing.T, buf *bytes.Buffer) []byte { return buf.Bytes() }
