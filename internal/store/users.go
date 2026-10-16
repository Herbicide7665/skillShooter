package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"os"
	"strconv"
	"strings"
	"time"
)

// ---- 口令哈希：PBKDF2-HMAC-SHA256（标准库手写，避免引入 x/crypto）----

const pbkdf2Iters = 60000

func pbkdf2Key(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(func() hash.Hash { return sha256.New() }, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	var out []byte
	buf := make([]byte, 4)
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(buf, uint32(block))
		prf.Write(buf)
		u := prf.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iter; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

// HashPassword 生成 "pbkdf2-sha256$iter$salt$hash" 格式口令哈希
func HashPassword(pw string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	h := pbkdf2Key([]byte(pw), salt, pbkdf2Iters, 32)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iters, b64(salt), b64(h))
}

// VerifyPassword 常数时间比对口令
func VerifyPassword(stored, pw string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1000 || iter > 10_000_000 {
		return false
	}
	b64 := base64.RawStdEncoding.DecodeString
	salt, err1 := b64(parts[2])
	want, err2 := b64(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got := pbkdf2Key([]byte(pw), salt, iter, len(want))
	return hmac.Equal(got, want)
}

func randToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return strings.ReplaceAll(base64.RawURLEncoding.EncodeToString(b), "-", "x")
}

// ---- 用户与会话 ----

// EnsureUsers 首次启动（users 空表）创建管理员账号。
// 返回 (初始密码, 是否新建, err)；密码来源：SKILLSHOOTER_ADMIN_PASSWORD env，否则随机生成（仅此一次打印到日志）。
func (s *Store) EnsureUsers() (string, bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return "", false, err
	}
	if n > 0 {
		return "", false, nil
	}
	pw := os.Getenv("SKILLSHOOTER_ADMIN_PASSWORD")
	if pw == "" {
		pw = randToken(12)
	}
	if err := s.CreateUser("admin", pw, "admin"); err != nil {
		return "", false, err
	}
	return pw, true, nil
}

// VerifyLogin 校验用户名口令，返回 (userID, role)
func (s *Store) VerifyLogin(username, password string) (int64, string, error) {
	var id int64
	var hash, role string
	err := s.db.QueryRow(`SELECT id, password_hash, role FROM users WHERE username=?`, username).
		Scan(&id, &hash, &role)
	if err != nil {
		return 0, "", errors.New("用户名或密码错误")
	}
	if !VerifyPassword(hash, password) {
		return 0, "", errors.New("用户名或密码错误")
	}
	return id, role, nil
}

func (s *Store) CreateUser(username, password, role string) error {
	if username == "" || len(password) < 6 {
		return errors.New("用户名不能为空且密码至少 6 位")
	}
	if role != "admin" && role != "user" {
		return errors.New("role 必须是 admin 或 user")
	}
	_, err := s.db.Exec(`INSERT INTO users(username, password_hash, role) VALUES(?,?,?)`,
		username, HashPassword(password), role)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return errors.New("用户名已存在")
	}
	return err
}

func (s *Store) DeleteUser(id int64) error {
	res, err := s.db.Exec(`DELETE FROM users WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("用户不存在")
	}
	s.db.Exec(`DELETE FROM sessions WHERE user_id=?`, id)
	// 防止删光管理员
	var adminN int
	s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&adminN)
	if adminN == 0 {
		return errors.New("不能删除最后一个管理员")
	}
	return nil
}

func (s *Store) SetUserPassword(id int64, password string) error {
	if len(password) < 6 {
		return errors.New("密码至少 6 位")
	}
	_, err := s.db.Exec(`UPDATE users SET password_hash=? WHERE id=?`, HashPassword(password), id)
	return err
}

// ChangePassword 本人改密：校验旧密码
func (s *Store) ChangePassword(id int64, oldPw, newPw string) error {
	var hash string
	if err := s.db.QueryRow(`SELECT password_hash FROM users WHERE id=?`, id).Scan(&hash); err != nil {
		return errors.New("用户不存在")
	}
	if !VerifyPassword(hash, oldPw) {
		return errors.New("旧密码错误")
	}
	return s.SetUserPassword(id, newPw)
}

type UserRow struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) ListUsers() ([]UserRow, error) {
	rows, err := s.db.Query(`SELECT id, username, role, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserRow{}
	for rows.Next() {
		var u UserRow
		var ts interface{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &ts); err != nil {
			return nil, err
		}
		switch t := ts.(type) {
		case string:
			u.CreatedAt = t
		case []byte:
			u.CreatedAt = string(t)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---- 会话 ----

const sessionTTL = 30 * 24 * time.Hour

// CreateSession 签发会话（expires_at 用 Unix 秒存 INTEGER，避免 SQLite 时间格式比较坑）
func (s *Store) CreateSession(userID int64) (string, error) {
	tok := randToken(32)
	_, err := s.db.Exec(`INSERT INTO sessions(token, user_id, expires_at) VALUES(?,?,?)`,
		tok, userID, time.Now().Add(sessionTTL).Unix())
	if err != nil {
		return "", err
	}
	// 惰性清理过期会话
	s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())
	return tok, nil
}

// SessionPrincipal 会话 → (userID, username, role)
func (s *Store) SessionPrincipal(token string) (int64, string, string, bool) {
	if token == "" {
		return 0, "", "", false
	}
	var userID int64
	var username, role string
	var exp int64
	err := s.db.QueryRow(`SELECT s.user_id, s.expires_at, u.username, u.role
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token=?`, token).
		Scan(&userID, &exp, &username, &role)
	if err != nil || exp < time.Now().Unix() {
		return 0, "", "", false
	}
	return userID, username, role, true
}

func (s *Store) DeleteSession(token string) {
	s.db.Exec(`DELETE FROM sessions WHERE token=?`, token)
}

// CountAdmins 供删除保护
func (s *Store) CountAdmins() int {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&n)
	return n
}

// UserIDByName 用户名 → id（0=不存在）
func (s *Store) UserIDByName(username string) int64 {
	var id int64
	s.db.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&id)
	return id
}

// UserRoleByID id → role（""=不存在）
func (s *Store) UserRoleByID(id int64) string {
	var role string
	s.db.QueryRow(`SELECT role FROM users WHERE id=?`, id).Scan(&role)
	return role
}
