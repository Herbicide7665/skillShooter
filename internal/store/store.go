package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc sqlite: 单连接避免写锁竞争，本场景流量足够
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT);

CREATE TABLE IF NOT EXISTS machines (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	hostname TEXT UNIQUE NOT NULL,
	first_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	push_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS skills (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	machine_id INTEGER NOT NULL REFERENCES machines(id),
	description TEXT NOT NULL DEFAULT '',
	latest_version_id INTEGER,
	is_system INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(name, machine_id)
);

CREATE TABLE IF NOT EXISTS versions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	skill_id INTEGER NOT NULL REFERENCES skills(id),
	tree_hash TEXT NOT NULL,
	file_count INTEGER NOT NULL DEFAULT 0,
	total_size INTEGER NOT NULL DEFAULT 0,
	kind TEXT NOT NULL DEFAULT 'push',
	source_version_ids TEXT NOT NULL DEFAULT '[]',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(skill_id, tree_hash)
);

CREATE TABLE IF NOT EXISTS blobs (
	hash TEXT PRIMARY KEY,
	size INTEGER NOT NULL,
	data BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS version_files (
	version_id INTEGER NOT NULL REFERENCES versions(id),
	path TEXT NOT NULL,
	blob_hash TEXT NOT NULL REFERENCES blobs(hash),
	PRIMARY KEY(version_id, path)
);

CREATE TABLE IF NOT EXISTS dup_groups (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	type TEXT NOT NULL,
	similarity REAL NOT NULL DEFAULT 0,
	reason TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'open',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS dup_members (
	group_id INTEGER NOT NULL REFERENCES dup_groups(id),
	skill_id INTEGER NOT NULL REFERENCES skills(id),
	PRIMARY KEY(group_id, skill_id)
);

CREATE TABLE IF NOT EXISTS edges (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	src_id INTEGER NOT NULL REFERENCES skills(id),
	dst_id INTEGER NOT NULL REFERENCES skills(id),
	kind TEXT NOT NULL,
	note TEXT NOT NULL DEFAULT '',
	UNIQUE(src_id, dst_id, kind)
);

CREATE TABLE IF NOT EXISTS merges (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	skill_id INTEGER NOT NULL REFERENCES skills(id),
	version_id INTEGER NOT NULL REFERENCES versions(id),
	source_version_ids TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS families (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	key TEXT UNIQUE NOT NULL,
	note TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS family_members (
	family_id INTEGER NOT NULL REFERENCES families(id),
	skill_id INTEGER NOT NULL REFERENCES skills(id),
	PRIMARY KEY(family_id, skill_id)
);

CREATE TABLE IF NOT EXISTS tokens (
	token TEXT PRIMARY KEY,
	role TEXT NOT NULL DEFAULT 'user',
	note TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS jobs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	machine TEXT NOT NULL,
	skill TEXT NOT NULL DEFAULT '',
	prompt TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	agent_cli TEXT NOT NULL DEFAULT '',
	result TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME,
	finished_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_jobs_machine ON jobs(machine, status);
CREATE INDEX IF NOT EXISTS idx_versions_skill ON versions(skill_id, id);
CREATE INDEX IF NOT EXISTS idx_edges_src ON edges(src_id);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// 幂等迁移：旧库补 is_system 列 + 按已知内置名单回填
	_, _ = s.db.Exec(`ALTER TABLE skills ADD COLUMN is_system INTEGER NOT NULL DEFAULT 0`)
	_, err := s.db.Exec(`UPDATE skills SET is_system=1 WHERE name IN (
		'pdf','docx','xlsx','pptx','computer-use','control-browser','web-gui-tester','env-setup',
		'android-dev','ios-dev','plugin-creator','skill-creator','dynamic-workflows',
		'video2code','video2code-3d','video2fullstack','url2video','web-replicate','zcode-computer-use',
		'diagnosing-commands','diagnosing-hooks','diagnosing-mcp','diagnosing-plugins','diagnosing-skills',
		'zcode-configuration-guide','zcode-guide','restore-legacy-sessions','playground')`)
	return err
}

func HashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// TreeHash 计算目录树哈希：对 (相对路径 + 文件内容哈希) 排序后再哈希
func TreeHash(files map[string][]byte) string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sortStrings(keys)
	h := sha256.New()
	for _, k := range keys {
		fh := sha256.Sum256(files[k])
		fmt.Fprintf(h, "%s:%s\n", k, hex.EncodeToString(fh[:]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func (s *Store) GetMeta(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// EnsureToken 返回接入 token，首次调用时自动生成
func (s *Store) EnsureToken() (string, error) {
	tok, err := s.GetMeta("push_token")
	if err != nil {
		return "", err
	}
	if tok != "" {
		return tok, nil
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok = hex.EncodeToString(b)
	return tok, s.SetMeta("push_token", tok)
}

// CheckToken 检查 token 有效性（兼容 meta 里的主 token = admin，以及 tokens 表角色 token）
func (s *Store) CheckToken(tok string) bool {
	_, role := s.TokenRole(tok)
	return role != ""
}

// TokenRole 返回 (有效, 角色)：主 token→admin，tokens 表→其角色，无效→("", "")
func (s *Store) TokenRole(tok string) (bool, string) {
	if tok == "" {
		return false, ""
	}
	if real, err := s.GetMeta("push_token"); err == nil && real != "" && tok == real {
		return true, "admin"
	}
	var role string
	err := s.db.QueryRow(`SELECT role FROM tokens WHERE token=?`, tok).Scan(&role)
	if err != nil {
		return false, ""
	}
	if role != "admin" && role != "user" {
		return false, ""
	}
	return true, role
}

// CreateRoleToken 新建角色 token（返回明文一次）
func (s *Store) CreateRoleToken(role, note string) (string, error) {
	if role != "admin" && role != "user" {
		return "", fmt.Errorf("role 必须是 admin 或 user")
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b)
	_, err := s.db.Exec(`INSERT INTO tokens(token, role, note) VALUES(?,?,?)`, tok, role, note)
	return tok, err
}

// ListTokens token 列表（脱敏展示）
func (s *Store) ListTokens() ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`SELECT token, role, note, created_at FROM tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var tok, role, note, created string
		if err := rows.Scan(&tok, &role, &note, &created); err != nil {
			return nil, err
		}
		masked := tok
		if len(tok) > 10 {
			masked = tok[:6] + "…" + tok[len(tok)-4:]
		}
		out = append(out, map[string]interface{}{"token_masked": masked, "role": role, "note": note, "created_at": created})
	}
	return out, rows.Err()
}

// RevokeToken 吊销
func (s *Store) RevokeToken(maskedOrFull string) error {
	// 支持传脱敏形式：按前缀+后缀匹配
	if strings.Contains(maskedOrFull, "…") {
		parts := strings.SplitN(maskedOrFull, "…", 2)
		_, err := s.db.Exec(`DELETE FROM tokens WHERE substr(token,1,6)=? AND substr(token,-4)=?`, parts[0], parts[1])
		return err
	}
	_, err := s.db.Exec(`DELETE FROM tokens WHERE token=?`, maskedOrFull)
	return err
}

// UpsertMachine 记录推送来源机器（先查后插，规避 upsert 分支 LastInsertId 脏值）
func (s *Store) UpsertMachine(hostname string) (int64, error) {
	var mid int64
	err := s.db.QueryRow(`SELECT id FROM machines WHERE hostname=?`, hostname).Scan(&mid)
	if err == nil {
		_, err = s.db.Exec(`UPDATE machines SET last_seen=CURRENT_TIMESTAMP, push_count=push_count+1 WHERE id=?`, mid)
		return mid, err
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO machines(hostname) VALUES(?)`, hostname)
	if err != nil {
		// 并发 push 同名机器撞 UNIQUE：重查一次
		err2 := s.db.QueryRow(`SELECT id FROM machines WHERE hostname=?`, hostname).Scan(&mid)
		if err2 != nil {
			return 0, err
		}
		return mid, nil
	}
	return res.LastInsertId()
}

// SaveSkill 落一个 skill 的新版本（内容寻址：tree_hash 相同则跳过）
// 返回 (skillID, versionID, created)
func (s *Store) SaveSkill(hostname, name, description string, files map[string][]byte) (int64, int64, bool, error) {
	mid, err := s.UpsertMachine(hostname)
	if err != nil {
		return 0, 0, false, err
	}
	treeHash := TreeHash(files)

	// skills 先查后插（规避 upsert 分支 LastInsertId 脏值）
	var sid int64
	err = s.db.QueryRow(`SELECT id FROM skills WHERE name=? AND machine_id=?`, name, mid).Scan(&sid)
	if err == sql.ErrNoRows {
		res, err := s.db.Exec(`INSERT INTO skills(name, machine_id, description) VALUES(?,?,?)`,
			name, mid, description)
		if err != nil {
			return 0, 0, false, err
		}
		sid, _ = res.LastInsertId()
	} else if err != nil {
		return 0, 0, false, err
	} else {
		if _, err = s.db.Exec(`UPDATE skills SET description=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			description, sid); err != nil {
			return 0, 0, false, err
		}
	}

	// 已存在同 hash 版本？
	var vid int64
	err = s.db.QueryRow(`SELECT id FROM versions WHERE skill_id=? AND tree_hash=?`, sid, treeHash).Scan(&vid)
	if err == nil {
		s.touch(sid, vid)
		return sid, vid, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, 0, false, err
	}

	// 写 blobs（幂等）
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, false, err
	}
	defer tx.Rollback()
	total := 0
	for _, data := range files {
		bh := HashBytes(data)
		_, err = tx.Exec(`INSERT OR IGNORE INTO blobs(hash,size,data) VALUES(?,?,?)`, bh, len(data), data)
		if err != nil {
			return 0, 0, false, err
		}
	}
	vres, err := tx.Exec(`INSERT INTO versions(skill_id, tree_hash, file_count, total_size) VALUES(?,?,?,?)`,
		sid, treeHash, len(files), total)
	if err != nil {
		return 0, 0, false, err
	}
	vid, _ = vres.LastInsertId()
	for path, data := range files {
		_ = path
		total += len(data)
		if _, err = tx.Exec(`INSERT INTO version_files(version_id, path, blob_hash) VALUES(?,?,?)`,
			vid, path, HashBytes(data)); err != nil {
			return 0, 0, false, err
		}
	}
	if _, err = tx.Exec(`UPDATE versions SET total_size=? WHERE id=?`, total, vid); err != nil {
		return 0, 0, false, err
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, false, err
	}
	s.touch(sid, vid)
	return sid, vid, true, nil
}

func (s *Store) touch(skillID, versionID int64) {
	s.db.Exec(`UPDATE skills SET latest_version_id=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, versionID, skillID)
}

// CreateMergedVersion 由合并采纳落新版本
func (s *Store) CreateMergedVersion(skillID int64, files map[string][]byte, sourceVersions []int64, description string) (int64, error) {
	srcJSON, _ := json.Marshal(sourceVersions)
	treeHash := TreeHash(files)
	// 先查后插：OR IGNORE 冲突时 LastInsertId 可能返回脏值（同 SaveSkill 的教训）
	var vid int64
	err := s.db.QueryRow(`SELECT id FROM versions WHERE skill_id=? AND tree_hash=?`, skillID, treeHash).Scan(&vid)
	if err == nil {
		return vid, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO versions(skill_id, tree_hash, file_count, total_size, kind, source_version_ids) VALUES(?,?,?,?, 'merged', ?)`,
		skillID, treeHash, len(files), 0, string(srcJSON))
	if err != nil {
		return 0, err
	}
	vid, _ = res.LastInsertId()
	total := 0
	for path, data := range files {
		bh := HashBytes(data)
		if _, err = s.db.Exec(`INSERT OR IGNORE INTO blobs(hash,size,data) VALUES(?,?,?)`, bh, len(data), data); err != nil {
			return 0, err
		}
		if _, err = s.db.Exec(`INSERT INTO version_files(version_id, path, blob_hash) VALUES(?,?,?)`, vid, path, bh); err != nil {
			return 0, err
		}
		total += len(data)
	}
	s.db.Exec(`UPDATE versions SET total_size=? WHERE id=?`, total, vid)
	s.touch(skillID, vid)
	if description != "" {
		s.db.Exec(`UPDATE skills SET description=? WHERE id=?`, description, skillID)
	}
	s.db.Exec(`INSERT INTO merges(skill_id, version_id, source_version_ids) VALUES(?,?,?)`, skillID, vid, string(srcJSON))
	return vid, nil
}

type SkillRow struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	MachineID   int64  `json:"machine_id"`
	Machine     string `json:"machine"`
	Description string `json:"description"`
	LatestVer   int64  `json:"latest_version_id"`
	TreeHash    string `json:"tree_hash"`
	VerCount    int    `json:"version_count"`
	UpdatedAt   string `json:"updated_at"`
	CreatedAt   string `json:"created_at"`
	IsSystem    bool   `json:"is_system"`
}

func (s *Store) ListSkills(q, machine string) ([]SkillRow, error) {
	sqlStr := `SELECT sk.id, sk.name, sk.machine_id, m.hostname, sk.description,
		COALESCE(sk.latest_version_id,0), COALESCE(v.tree_hash,''), sk.created_at, sk.updated_at,
		(SELECT COUNT(*) FROM versions vv WHERE vv.skill_id=sk.id), sk.is_system
		FROM skills sk JOIN machines m ON m.id=sk.machine_id
		LEFT JOIN versions v ON v.id=sk.latest_version_id WHERE 1=1`
	args := []interface{}{}
	if q != "" {
		sqlStr += ` AND (sk.name LIKE ? OR sk.description LIKE ?)`
		args = append(args, "%"+q+"%", "%"+q+"%")
	}
	if machine != "" {
		sqlStr += ` AND m.hostname=?`
		args = append(args, machine)
	}
	sqlStr += ` ORDER BY sk.updated_at DESC`
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillRow{}
	for rows.Next() {
		var r SkillRow
		if err := rows.Scan(&r.ID, &r.Name, &r.MachineID, &r.Machine, &r.Description,
			&r.LatestVer, &r.TreeHash, &r.CreatedAt, &r.UpdatedAt, &r.VerCount, &r.IsSystem); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type VersionRow struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	TreeHash  string `json:"tree_hash"`
	FileCount int    `json:"file_count"`
	TotalSize int    `json:"total_size"`
	CreatedAt string `json:"created_at"`
	Sources   []int64 `json:"source_version_ids"`
}

func (s *Store) ListVersions(skillID int64) ([]VersionRow, error) {
	rows, err := s.db.Query(`SELECT id, kind, tree_hash, file_count, total_size, created_at, source_version_ids
		FROM versions WHERE skill_id=? ORDER BY id`, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VersionRow{}
	for rows.Next() {
		var r VersionRow
		var src string
		if err := rows.Scan(&r.ID, &r.Kind, &r.TreeHash, &r.FileCount, &r.TotalSize, &r.CreatedAt, &src); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(src), &r.Sources)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListVersionFiles(versionID int64) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT path, blob_hash FROM version_files WHERE version_id=? ORDER BY path`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, h string
		if err := rows.Scan(&p, &h); err != nil {
			return nil, err
		}
		out[p] = h
	}
	return out, rows.Err()
}

func (s *Store) ReadBlob(hash string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM blobs WHERE hash=?`, hash).Scan(&data)
	return data, err
}

func (s *Store) GetSkill(skillID int64) (*SkillRow, error) {
	var r SkillRow
	err := s.db.QueryRow(`SELECT sk.id, sk.name, sk.machine_id, m.hostname, sk.description,
		COALESCE(sk.latest_version_id,0), COALESCE(v.tree_hash,''), sk.created_at, sk.updated_at,
		(SELECT COUNT(*) FROM versions vv WHERE vv.skill_id=sk.id), sk.is_system
		FROM skills sk JOIN machines m ON m.id=sk.machine_id
		LEFT JOIN versions v ON v.id=sk.latest_version_id WHERE sk.id=?`, skillID).
		Scan(&r.ID, &r.Name, &r.MachineID, &r.Machine, &r.Description,
			&r.LatestVer, &r.TreeHash, &r.CreatedAt, &r.UpdatedAt, &r.VerCount, &r.IsSystem)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ReadSkillVersion 读某版本下某文件内容
func (s *Store) ReadSkillVersion(versionID int64, path string) ([]byte, error) {
	var h string
	err := s.db.QueryRow(`SELECT blob_hash FROM version_files WHERE version_id=? AND path=?`, versionID, path).Scan(&h)
	if err != nil {
		return nil, err
	}
	return s.ReadBlob(h)
}

type MachineRow struct {
	ID        int64  `json:"id"`
	Hostname  string `json:"hostname"`
	FirstSeen string `json:"first_seen"`
	LastSeen  string `json:"last_seen"`
	PushCount int    `json:"push_count"`
	SkillN    int    `json:"skill_count"`
}

func (s *Store) ListMachines() ([]MachineRow, error) {
	rows, err := s.db.Query(`SELECT m.id, m.hostname, m.first_seen, m.last_seen, m.push_count,
		(SELECT COUNT(*) FROM skills sk WHERE sk.machine_id=m.id) FROM machines m ORDER BY m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MachineRow{}
	for rows.Next() {
		var r MachineRow
		if err := rows.Scan(&r.ID, &r.Hostname, &r.FirstSeen, &r.LastSeen, &r.PushCount, &r.SkillN); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type Stats struct {
	SkillN     int            `json:"skill_count"`
	VersionN   int            `json:"version_count"`
	MachineN   int            `json:"machine_count"`
	OpenDupN   int            `json:"open_dup_groups"`
	EdgeN      int            `json:"edge_count"`
	BlobBytes  int64          `json:"blob_bytes"`
	Recent     []RecentChange `json:"recent"`
}

type RecentChange struct {
	SkillID   int64  `json:"skill_id"`
	Name      string `json:"name"`
	Machine   string `json:"machine"`
	Kind      string `json:"kind"`
	VerID     int64  `json:"version_id"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) GetStats() (*Stats, error) {
	st := &Stats{Recent: []RecentChange{}}
	q := func(dst *int, sqlStr string) error {
		return s.db.QueryRow(sqlStr).Scan(dst)
	}
	if err := q(&st.SkillN, `SELECT COUNT(*) FROM skills`); err != nil {
		return nil, err
	}
	if err := q(&st.VersionN, `SELECT COUNT(*) FROM versions`); err != nil {
		return nil, err
	}
	if err := q(&st.MachineN, `SELECT COUNT(*) FROM machines`); err != nil {
		return nil, err
	}
	if err := q(&st.OpenDupN, `SELECT COUNT(*) FROM dup_groups WHERE status='open'`); err != nil {
		return nil, err
	}
	if err := q(&st.EdgeN, `SELECT COUNT(*) FROM edges`); err != nil {
		return nil, err
	}
	s.db.QueryRow(`SELECT COALESCE(SUM(size),0) FROM blobs`).Scan(&st.BlobBytes)
	rows, err := s.db.Query(`SELECT sk.id, sk.name, m.hostname, v.kind, v.id, v.created_at
		FROM versions v JOIN skills sk ON sk.id=v.skill_id JOIN machines m ON m.id=sk.machine_id
		ORDER BY v.id DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r RecentChange
		if err := rows.Scan(&r.SkillID, &r.Name, &r.Machine, &r.Kind, &r.VerID, &r.CreatedAt); err != nil {
			return nil, err
		}
		st.Recent = append(st.Recent, r)
	}
	return st, nil
}

// ---- 去重 ----

type DupGroup struct {
	ID         int64      `json:"id"`
	Type       string     `json:"type"`
	Similarity float64    `json:"similarity"`
	Reason     string     `json:"reason"`
	Status     string     `json:"status"`
	CreatedAt  string     `json:"created_at"`
	Members    []SkillRow `json:"members"`
}

func (s *Store) CreateDupGroup(typ string, sim float64, reason string, skillIDs []int64) (int64, error) {
	if len(skillIDs) < 2 {
		return 0, fmt.Errorf("need >=2 members")
	}
	// 幂等：同 type 且成员完全一致的 open 组先查重
	existing, err := s.ListDupGroups("all")
	if err != nil {
		return 0, err
	}
	for _, g := range existing {
		if g.Type != typ || g.Status != "open" {
			continue
		}
		if sameIDSet(g.Members, skillIDs) {
			return g.ID, nil
		}
	}
	res, err := s.db.Exec(`INSERT INTO dup_groups(type, similarity, reason) VALUES(?,?,?)`, typ, sim, reason)
	if err != nil {
		return 0, err
	}
	gid, _ := res.LastInsertId()
	for _, sid := range skillIDs {
		if _, err := s.db.Exec(`INSERT INTO dup_members(group_id, skill_id) VALUES(?,?)`, gid, sid); err != nil {
			return 0, err
		}
	}
	return gid, nil
}

func sameIDSet(members []SkillRow, ids []int64) bool {
	if len(members) != len(ids) {
		return false
	}
	set := map[int64]bool{}
	for _, m := range members {
		set[m.ID] = true
	}
	for _, id := range ids {
		if !set[id] {
			return false
		}
	}
	return true
}

func (s *Store) ListDupGroups(status string) ([]DupGroup, error) {
	sqlStr := `SELECT id, type, similarity, reason, status, created_at FROM dup_groups`
	var args []interface{}
	if status != "all" && status != "" {
		sqlStr += ` WHERE status=?`
		args = append(args, status)
	}
	sqlStr += ` ORDER BY id DESC`
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DupGroup{}
	groupIDs := []int64{}
	for rows.Next() {
		var g DupGroup
		if err := rows.Scan(&g.ID, &g.Type, &g.Similarity, &g.Reason, &g.Status, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
		groupIDs = append(groupIDs, g.ID)
	}
	for i := range out {
		mrows, err := s.db.Query(`SELECT sk.id, sk.name, sk.machine_id, m.hostname, sk.description,
			COALESCE(sk.latest_version_id,0), COALESCE(v.tree_hash,''), sk.created_at, sk.updated_at,
			(SELECT COUNT(*) FROM versions vv WHERE vv.skill_id=sk.id), sk.is_system
			FROM dup_members dm JOIN skills sk ON sk.id=dm.skill_id
			JOIN machines m ON m.id=sk.machine_id
			LEFT JOIN versions v ON v.id=sk.latest_version_id WHERE dm.group_id=?`, out[i].ID)
		if err != nil {
			return nil, err
		}
		for mrows.Next() {
			var r SkillRow
			if err := mrows.Scan(&r.ID, &r.Name, &r.MachineID, &r.Machine, &r.Description,
				&r.LatestVer, &r.TreeHash, &r.CreatedAt, &r.UpdatedAt, &r.VerCount, &r.IsSystem); err != nil {
				mrows.Close()
				return nil, err
			}
			out[i].Members = append(out[i].Members, r)
		}
		mrows.Close()
	}
	return out, nil
}

func (s *Store) GetDupGroup(id int64) (*DupGroup, error) {
	groups, err := s.ListDupGroups("all")
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].ID == id {
			return &groups[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *Store) SetDupGroupStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE dup_groups SET status=? WHERE id=?`, status, id)
	return err
}

// ---- 关联关系 ----

func (s *Store) AddEdge(src, dst int64, kind, note string) error {
	if src == dst {
		return nil
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO edges(src_id, dst_id, kind, note) VALUES(?,?,?,?)`,
		src, dst, kind, note)
	return err
}

func (s *Store) ClearEdges(kind string) error {
	_, err := s.db.Exec(`DELETE FROM edges WHERE kind=?`, kind)
	return err
}

type Edge struct {
	ID    int64  `json:"id"`
	SrcID int64  `json:"src_id"`
	DstID int64  `json:"dst_id"`
	Src   string `json:"src"`
	Dst   string `json:"dst"`
	Kind  string `json:"kind"`
	Note  string `json:"note"`
}

func (s *Store) ListEdges() ([]Edge, error) {
	rows, err := s.db.Query(`SELECT e.id, e.src_id, e.dst_id, s1.name, s2.name, e.kind, e.note
		FROM edges e JOIN skills s1 ON s1.id=e.src_id JOIN skills s2 ON s2.id=e.dst_id
		ORDER BY e.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Edge{}
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.ID, &e.SrcID, &e.DstID, &e.Src, &e.Dst, &e.Kind, &e.Note); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AllSkillNames 返回 id->name 映射（建引用边用）
func (s *Store) AllSkillNames() (map[int64]string, error) {
	rows, err := s.db.Query(`SELECT id, name FROM skills`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var n string
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// LatestSkillMD 返回 skill 最新版 SKILL.md 内容
func (s *Store) LatestSkillMD(skillID int64) (string, error) {
	var vid int64
	err := s.db.QueryRow(`SELECT COALESCE(latest_version_id,0) FROM skills WHERE id=?`, skillID).Scan(&vid)
	if err != nil || vid == 0 {
		return "", err
	}
	b, err := s.ReadSkillVersion(vid, "SKILL.md")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func Now() string { return time.Now().Format(time.RFC3339) }

func SplitSkillPath(p string) (dir, base string) {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return ".", p
	}
	return p[:i], p[i+1:]
}

// ---- 远程调用 jobs ----

type Job struct {
	ID        int64  `json:"id"`
	Machine   string `json:"machine"`
	Skill     string `json:"skill"`
	Prompt    string `json:"prompt"`
	Status    string `json:"status"`
	AgentCLI  string `json:"agent_cli"`
	Result    string `json:"result"`
	CreatedAt string `json:"created_at"`
	StartedAt string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

func (s *Store) CreateJob(machine, skill, prompt string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO jobs(machine, skill, prompt) VALUES(?,?,?)`, machine, skill, prompt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// NextJob 认领某机器最早的 pending 任务（置 running）
func (s *Store) NextJob(machine string) (*Job, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM jobs WHERE machine=? AND status='pending' ORDER BY id LIMIT 1`, machine).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	res, err := s.db.Exec(`UPDATE jobs SET status='running', started_at=CURRENT_TIMESTAMP WHERE id=? AND status='pending'`, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil // 被并发抢走
	}
	return s.GetJob(id)
}

func (s *Store) FinishJob(id int64, status, cli, result string) error {
	_, err := s.db.Exec(`UPDATE jobs SET status=?, agent_cli=?, result=?, finished_at=CURRENT_TIMESTAMP WHERE id=?`,
		status, cli, result, id)
	return err
}

func (s *Store) ListJobs(limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, machine, skill, prompt, status, agent_cli, result, created_at,
		COALESCE(started_at,''), COALESCE(finished_at,'') FROM jobs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Machine, &j.Skill, &j.Prompt, &j.Status, &j.AgentCLI, &j.Result,
			&j.CreatedAt, &j.StartedAt, &j.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) GetJob(id int64) (*Job, error) {
	var j Job
	err := s.db.QueryRow(`SELECT id, machine, skill, prompt, status, agent_cli, result, created_at,
		COALESCE(started_at,''), COALESCE(finished_at,'') FROM jobs WHERE id=?`, id).
		Scan(&j.ID, &j.Machine, &j.Skill, &j.Prompt, &j.Status, &j.AgentCLI, &j.Result,
			&j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// CanonEntry 云端已合并（canonical）的 skill 版本，供客户端 apply
type CanonEntry struct {
	Name         string   `json:"name"`
	VersionID    int64    `json:"version_id"`
	TreeHash     string   `json:"tree_hash"`
	SkillID      int64    `json:"skill_id"`
	Machine      string   `json:"machine"`
	SourceHashes []string `json:"source_hashes"` // 合并前各源版本的树哈希；本地内容命中其一才应用
}

// CanonicalMerges 返回每个名字最后一次合并版（按名字去重取最新 merged 版本）。
// 不要求它是当前 latest（后续普通推送不推翻合并结论）。
func (s *Store) CanonicalMerges() ([]CanonEntry, error) {
	rows, err := s.db.Query(`SELECT sk.name, v.id, v.tree_hash, sk.id, m.hostname, v.source_version_ids
		FROM skills sk
		JOIN versions v ON v.id = (SELECT MAX(v2.id) FROM versions v2 WHERE v2.skill_id = sk.id AND v2.kind = 'merged')
		JOIN machines m ON m.id = sk.machine_id
		WHERE v.id IS NOT NULL
		ORDER BY v.id DESC`)
	if err != nil {
		return nil, err
	}
	// 注意：MaxOpenConns(1)，不能在 rows 循环内再查询（会死锁）——先收完再查
	type rawEntry struct {
		e      CanonEntry
		srcIDs []int64
	}
	var raws []rawEntry
	seen := map[string]bool{}
	for rows.Next() {
		var r rawEntry
		var srcIDs string
		if err := rows.Scan(&r.e.Name, &r.e.VersionID, &r.e.TreeHash, &r.e.SkillID, &r.e.Machine, &srcIDs); err != nil {
			rows.Close()
			return nil, err
		}
		if seen[r.e.Name] {
			continue
		}
		seen[r.e.Name] = true
		json.Unmarshal([]byte(srcIDs), &r.srcIDs)
		raws = append(raws, r)
	}
	rows.Close()
	out := []CanonEntry{}
	for _, r := range raws {
		for _, id := range r.srcIDs {
			var h string
			if err := s.db.QueryRow(`SELECT tree_hash FROM versions WHERE id=?`, id).Scan(&h); err == nil {
				r.e.SourceHashes = append(r.e.SourceHashes, h)
			}
		}
		out = append(out, r.e)
	}
	return out, nil
}

// ---- 家族（GLM 重构后固化存储）----

type FamilyRow struct {
	Key     string `json:"key"`
	Note    string `json:"note"`
	Members []SkillRow `json:"members"`
}

// ReplaceFamilies 全量替换家族（重构时调用）
func (s *Store) ReplaceFamilies(fams []FamilyRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM family_members`); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM families`); err != nil {
		return err
	}
	for _, f := range fams {
		res, err := tx.Exec(`INSERT INTO families(key, note) VALUES(?,?)`, f.Key, f.Note)
		if err != nil {
			return err
		}
		fid, _ := res.LastInsertId()
		for _, m := range f.Members {
			if _, err = tx.Exec(`INSERT OR IGNORE INTO family_members(family_id, skill_id) VALUES(?,?)`, fid, m.ID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ListFamilies 读取固化的家族（按族内成员数降序）
func (s *Store) ListFamilies() ([]FamilyRow, error) {
	rows, err := s.db.Query(`SELECT f.id, f.key, f.note,
		(SELECT COUNT(*) FROM family_members fm2 WHERE fm2.family_id=f.id) AS n
		FROM families f ORDER BY n DESC, f.key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type fid struct{ id, idx int64 }
	var out []FamilyRow
	var ids []fid
	for rows.Next() {
		var id int64
		var n int
		var f FamilyRow
		if err := rows.Scan(&id, &f.Key, &f.Note, &n); err != nil {
			return nil, err
		}
		out = append(out, f)
		ids = append(ids, fid{id: id, idx: int64(len(out) - 1)}) // 索引而非指针：防 append 扩容后指向旧数组
	}
	rows.Close()
	for _, x := range ids {
		mrows, err := s.db.Query(`SELECT sk.id, sk.name, sk.machine_id, m.hostname, sk.description,
			COALESCE(sk.latest_version_id,0), COALESCE(v.tree_hash,''), sk.created_at, sk.updated_at,
			(SELECT COUNT(*) FROM versions vv WHERE vv.skill_id=sk.id), sk.is_system
			FROM family_members fm JOIN skills sk ON sk.id=fm.skill_id
			JOIN machines m ON m.id=sk.machine_id
			LEFT JOIN versions v ON v.id=sk.latest_version_id
			WHERE fm.family_id=? ORDER BY sk.name`, x.id)
		if err != nil {
			return nil, err
		}
		for mrows.Next() {
			var r SkillRow
			if err := mrows.Scan(&r.ID, &r.Name, &r.MachineID, &r.Machine, &r.Description,
				&r.LatestVer, &r.TreeHash, &r.CreatedAt, &r.UpdatedAt, &r.VerCount, &r.IsSystem); err != nil {
				mrows.Close()
				return nil, err
			}
			out[x.idx].Members = append(out[x.idx].Members, r)
		}
		mrows.Close()
	}
	return out, nil
}

// SetSkillSystem 标记/取消系统 skill
func (s *Store) SetSkillSystem(id int64, isSystem bool) error {
	v := 0
	if isSystem {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE skills SET is_system=? WHERE id=?`, v, id)
	return err
}
