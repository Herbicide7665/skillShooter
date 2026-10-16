// HMAC-SHA256 请求签名认证：token 不明文传输（防被动嗅探），带时间窗+nonce 防重放、body 摘要防篡改。
// 兼容旧客户端：无签名头时回落 X-Token 明文校验（宽限期），详见 PROTOCOL.md。
package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	signTimeWindow = 600 * time.Second // ±10 分钟（容忍设备时钟漂移）
	nonceTTL       = 22 * time.Minute  // 覆盖两倍时间窗
)

type nonceCache struct {
	mu    sync.Mutex
	seen  map[string]time.Time
	order []string // FIFO 淘汰
}

func newNonceCache() *nonceCache {
	return &nonceCache{seen: map[string]time.Time{}}
}

// Check 首次见到返回 true；过期项惰性清理
func (c *nonceCache) Check(nonce string) bool {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.order) > 4096 {
		keep := c.order[:0]
		for _, k := range c.order {
			if t, ok := c.seen[k]; ok && now.Sub(t) < nonceTTL {
				keep = append(keep, k)
			} else {
				delete(c.seen, k)
			}
		}
		c.order = keep
	}
	if t, ok := c.seen[nonce]; ok && now.Sub(t) < nonceTTL {
		return false
	}
	c.seen[nonce] = now
	c.order = append(c.order, nonce)
	return true
}

func keyIDOf(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])[:16]
}

// tokenByKeyID 遍历候选 token 找 key-id 匹配者（X-Key-ID = sha256(token) 前 16 hex）
func (s *Server) tokenByKeyID(keyID string) (string, bool) {
	if keyID == "" {
		return "", false
	}
	cands := append([]string{}, s.st.AllTokens()...)
	for _, tok := range cands {
		if keyIDOf(tok) == keyID {
			return tok, true
		}
	}
	return "", false
}

// signString 构造待签名串：METHOD\npath?query\ntimestamp\nnonce\nbodySHA256
func signString(r *http.Request, bodySHA string) string {
	pq := r.URL.Path
	if r.URL.RawQuery != "" {
		pq += "?" + r.URL.RawQuery
	}
	return strings.Join([]string{r.Method, pq, r.Header.Get("X-Timestamp"), r.Header.Get("X-Nonce"), bodySHA}, "\n")
}

// signedToken 校验一个带签名头的请求；成功返回匹配到的 token（供角色判定）并恢复 r.Body。
// 失败时把 401 写到 w 并返回 ok=false（不降级到明文模式）。
// 校验链：Key-ID 找 token → 时间窗 → nonce 防重放 → HMAC → X-Body-SHA256 比对。
func (s *Server) signedToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	keyID := r.Header.Get("X-Key-ID")
	ts := r.Header.Get("X-Timestamp")
	nonce := r.Header.Get("X-Nonce")
	sign := r.Header.Get("X-Sign")
	declared := r.Header.Get("X-Body-SHA256")
	if keyID == "" || ts == "" || nonce == "" || sign == "" {
		writeErr(w, 401, "缺少签名头")
		return "", false
	}
	tsSec, err := strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
	if err != nil {
		writeErr(w, 401, "时间戳格式错误")
		return "", false
	}
	if d := time.Since(time.Unix(tsSec, 0)); d > signTimeWindow || d < -signTimeWindow {
		writeErr(w, 401, "签名时间窗过期")
		return "", false
	}
	if !s.nonces.Check(nonce) {
		writeErr(w, 401, "nonce 已使用（疑似重放）")
		return "", false
	}
	tok, ok := s.tokenByKeyID(keyID)
	if !ok {
		writeErr(w, 401, "token 无效")
		return "", false
	}
	// body 摘要：读进内存计算（客户端各端点 body 均为 JSON/小表单；大包 push 由客户端声明 X-Body-SHA256
	// 并在签名串中引用该声明值，服务端同样以声明值参与 HMAC，再读全量比对摘要防篡改）
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, 400, "读取请求体失败")
		return "", false
	}
	sum := sha256.Sum256(body)
	computed := hex.EncodeToString(sum[:])
	if declared == "" {
		declared = computed
	} else if declared != computed {
		writeErr(w, 401, "body 摘要不匹配（内容被篡改）")
		return "", false
	}
	mac := hmac.New(sha256.New, []byte(tok))
	mac.Write([]byte(signString(r, declared)))
	if !hmac.Equal(mac.Sum(nil), mustHexDecode(sign)) {
		writeErr(w, 401, "签名校验失败")
		return "", false
	}
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	return tok, true
}

func mustHexDecode(s string) []byte {
	b, err := hex.DecodeString(strings.ToLower(s))
	if err != nil {
		return []byte(s)
	}
	return b
}
