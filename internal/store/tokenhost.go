package store

import (
	"database/sql"
	"errors"
	"strings"
)

// tokHostCol tokens.hostname 幂等迁移（一 token 一设备，空 = 待认领）
func (s *Store) migrateTokensHostname() {
	_, _ = s.db.Exec(`ALTER TABLE tokens ADD COLUMN hostname TEXT NOT NULL DEFAULT ''`)
}

// TokenPrincipal token → (ok, role, machine)
// admin 主 token → machine=""（不绑设备，API 全权）；tokens 表 → 绑定设备
func (s *Store) TokenPrincipal(tok string) (bool, string, string) {
	real, err := s.GetMeta("push_token")
	if err == nil && real != "" && tok == real {
		return true, "admin", ""
	}
	if tok == "" {
		return false, "", ""
	}
	var role, host string
	err = s.db.QueryRow(`SELECT role, hostname FROM tokens WHERE token=?`, tok).Scan(&role, &host)
	if err != nil {
		return false, "", ""
	}
	if role != "admin" && role != "user" {
		role = "user"
	}
	return true, role, host
}

// CreateRoleTokenWithHost 创建带设备绑定的角色 token（hostname 可空 = 首次使用自动认领）
func (s *Store) CreateRoleTokenWithHost(role, note, hostname string) (string, error) {
	tok, err := s.CreateRoleToken(role, note)
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`UPDATE tokens SET hostname=? WHERE token=?`, strings.TrimSpace(hostname), tok)
	return tok, err
}

// AdoptTokenHostname 首次使用自动认领：仅当当前绑定为空时写入并锁死
func (s *Store) AdoptTokenHostname(tok, hostname string) error {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return errors.New("hostname 不能为空")
	}
	res, err := s.db.Exec(`UPDATE tokens SET hostname=? WHERE token=? AND hostname=''`, hostname, tok)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var cur string
		s.db.QueryRow(`SELECT hostname FROM tokens WHERE token=?`, tok).Scan(&cur)
		if cur == "" {
			return errors.New("token 不存在或不是可认领的 user token")
		}
		return errors.New("token 已绑定设备 " + cur)
	}
	return nil
}

// SetTokenHostname admin 改绑（空 = 解绑回待认领态）
func (s *Store) SetTokenHostname(tok, hostname string) error {
	hostname = strings.TrimSpace(hostname)
	// 拒绝把主 token 当成表内 token 改绑
	if real, _ := s.GetMeta("push_token"); real != "" && tok == real {
		return errors.New("主 token 不绑定设备")
	}
	if tok == "" || len(tok) < 6 {
		return errors.New("缺少 token")
	}
	// 支持传掩码（设置页表格里只有掩码）。
	// MaxOpenConns(1) 下 rows 循环内再 Exec 会死锁——掩码反查必须在 rows 关闭后进行。
	var full string
	err := s.db.QueryRow(`SELECT token FROM tokens WHERE token=?`, tok).Scan(&full)
	if err == sql.ErrNoRows {
		var all []string
		rows, qerr := s.db.Query(`SELECT token FROM tokens`)
		if qerr != nil {
			return qerr
		}
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err == nil {
				all = append(all, t)
			}
		}
		rows.Close()
		if qerr := rows.Err(); qerr != nil {
			return qerr
		}
		for _, t := range all {
			if TokenMask(t) == tok {
				full = t
				break
			}
		}
		if full == "" {
			return errors.New("token 不存在")
		}
	} else if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE tokens SET hostname=? WHERE token=?`, hostname, full)
	return err
}

// TokenInfo 列表行（含绑定设备）
type TokenRowHost struct {
	Token    string
	Role    string
	Note    string
	Host    string
	Created string
}

// ListTokensWithHost 供 admin 设置页展示
func (s *Store) ListTokensWithHost() ([]TokenRowHost, error) {
	rows, err := s.db.Query(`SELECT token, role, note, hostname, created_at FROM tokens ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TokenRowHost{}
	for rows.Next() {
		var r TokenRowHost
		var ts interface{}
		if err := rows.Scan(&r.Token, &r.Role, &r.Note, &r.Host, &ts); err != nil {
			return nil, err
		}
		switch t := ts.(type) {
		case string:
			r.Created = t
		case []byte:
			r.Created = string(t)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeviceSkillScope 设备域：本机推送的 skill ∪ 分配给本机的 skill（统一权限过滤查询）
// 返回 skill id 集合（供 handler 内逐个校验或全量过滤）
func (s *Store) DeviceSkillScope(hostname string) (map[int64]bool, error) {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return map[int64]bool{}, nil
	}
	rows, err := s.db.Query(`
		SELECT s.id FROM skills s JOIN machines m ON s.machine_id = m.id WHERE m.hostname = ?
		UNION
		SELECT a.skill_id FROM assignments a JOIN machines m ON a.machine_id = m.id WHERE m.hostname = ?`,
		hostname, hostname)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
