// 客户端请求签名（与 server/auth.go 对应，见 PROTOCOL.md）：
// X-Key-ID = sha256(token)[:16]（token 不上 wire）；X-Sign = HMAC-SHA256(token, 签名串)
// 签名串 = METHOD\npath?query\ntimestamp\nnonce\nbodySHA256
package pusher

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func KeyIDOf(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:16]
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// AddAuth 为请求附加签名认证头
func AddAuth(req *http.Request, token string, body []byte) {
	ts := fmt.Sprint(time.Now().Unix())
	nonce := randHex(16)
	sum := sha256.Sum256(body)
	bodySHA := hex.EncodeToString(sum[:])
	pq := req.URL.Path
	if req.URL.RawQuery != "" {
		pq += "?" + req.URL.RawQuery
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(strings.Join([]string{req.Method, pq, ts, nonce, bodySHA}, "\n")))
	req.Header.Set("X-Key-ID", KeyIDOf(token))
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Body-SHA256", bodySHA)
	req.Header.Set("X-Sign", hex.EncodeToString(mac.Sum(nil)))
	return
}
