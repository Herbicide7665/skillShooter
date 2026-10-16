package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"skill-manager/internal/llm"
	"skill-manager/internal/store"
)

func TrimPercent(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	return s
}

// ---- 相似度 ----

func tokens(s string) map[string]bool {
	out := map[string]bool{}
	var word strings.Builder
	var cjk []rune
	flushWord := func() {
		if word.Len() > 0 {
			out[word.String()] = true
			word.Reset()
		}
	}
	flushCjk := func() {
		// 中文按二元组（bigram）切，否则整句成单 token 导致 Jaccard 失真
		for i := 0; i+1 < len(cjk); i++ {
			out[string(cjk[i:i+2])] = true
		}
		if len(cjk) == 1 {
			out[string(cjk)] = true
		}
		cjk = cjk[:0]
	}
	for _, r := range strings.ToLower(s) {
		isCJK := r >= 0x4e00 && r <= 0x9fff
		switch {
		case isCJK:
			flushWord()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCjk()
			word.WriteRune(r)
		default:
			flushWord()
			flushCjk()
		}
	}
	flushWord()
	flushCjk()
	return out
}

func Jaccard(a, b string) float64 {
	ta, tb := tokens(a), tokens(b)
	if len(ta) == 0 && len(tb) == 0 {
		return 0
	}
	inter := 0
	for k := range ta {
		if tb[k] {
			inter++
		}
	}
	union := len(ta) + len(tb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func editDist(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// NameSim 编辑距离归一化相似度
func NameSim(a, b string) float64 {
	if a == b {
		return 1
	}
	d := editDist(strings.ToLower(a), strings.ToLower(b))
	m := len(a)
	if len(b) > m {
		m = len(b)
	}
	if m == 0 {
		return 0
	}
	return 1 - float64(d)/float64(m)
}

// PairSim 组合相似度：名称编辑距离与描述 Jaccard 加权
func PairSim(nameA, descA, nameB, descB string) float64 {
	ns := NameSim(nameA, nameB)
	ds := Jaccard(descA, descB)
	if ns == 1 {
		return 0.5 + 0.5*ds
	}
	return 0.55*ns + 0.45*ds
}

// ---- SKILL.md 节选 ----

func mdExcerpt(s *store.Store, skill store.SkillRow, limit int) string {
	md, err := s.LatestSkillMD(skill.ID)
	if err != nil || md == "" {
		return ""
	}
	if len(md) > limit {
		return md[:limit] + "\n…(截断)"
	}
	return md
}

// ---- 精确去重 ----

// ScanExact 按 tree_hash 精确分组，返回新建组数
func ScanExact(s *store.Store) (int, error) {
	skills, err := s.ListSkills("", "")
	if err != nil {
		return 0, err
	}
	byHash := map[string][]int64{}
	for _, sk := range skills {
		if sk.TreeHash != "" {
			byHash[sk.TreeHash] = append(byHash[sk.TreeHash], sk.ID)
		}
	}
	created := 0
	seen := map[string]bool{}
	for _, ids := range byHash {
		if len(ids) < 2 {
			continue
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		key := fmt.Sprint(ids)
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, err := s.CreateDupGroup("exact", 1.0, "内容哈希完全一致", ids); err == nil {
			created++
		}
	}
	return created, nil
}

// ---- 近似去重 ----

type FuzzyVerdict struct {
	IsDuplicate bool    `json:"is_duplicate"`
	Confidence  float64 `json:"confidence"`
	Reason      string  `json:"reason"`
}

// UnmarshalJSON 宽松解析：GLM 偶尔输出中文字段名或字符串数字
func (v *FuzzyVerdict) UnmarshalJSON(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	v.IsDuplicate = boolVal(m, "is_duplicate", "duplicate", "same_variant", "是否重复", "是否变体")
	v.Confidence = numVal(m, "confidence", "置信度", "confidence_level")
	v.Reason = strVal(m, "reason", "判定理由", "理由", "说明")
	return nil
}

func boolVal(m map[string]interface{}, keys ...string) bool {
	for _, k := range keys {
		if b, ok := m[k]; ok {
			switch t := b.(type) {
			case bool:
				return t
			case string:
				return strings.Contains(strings.ToLower(t), "true")
			}
		}
	}
	return false
}

func numVal(m map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		if f, ok := m[k]; ok {
			switch t := f.(type) {
			case float64:
				return t
			case string:
				n, _ := strconv.ParseFloat(TrimPercent(t), 64)
				return n
			}
		}
	}
	return 0
}

func strVal(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k]; ok {
			if t, ok := s.(string); ok {
				return t
			}
		}
	}
	return ""
}

// ScanFuzzy 预筛候选对；GLM 可用时逐对判定，不可用时直接落预筛组
func ScanFuzzy(ctx context.Context, s *store.Store, c *llm.Client, threshold float64) (int, error) {
	skills, err := s.ListSkills("", "")
	if err != nil {
		return 0, err
	}
	created := 0
	for i := 0; i < len(skills); i++ {
		for j := i + 1; j < len(skills); j++ {
			a, b := skills[i], skills[j]
			if a.TreeHash == b.TreeHash {
				continue // 已属精确同组
			}
			sim := PairSim(a.Name, a.Description, b.Name, b.Description)
			ns := NameSim(a.Name, b.Name)
			// 同名跨来源即强候选（描述风格差异大也会被 Jaccard 稀释）
			if sim < threshold && ns < 0.99 {
				continue
			}
			reason := fmt.Sprintf("预筛相似度 %.2f（名称 %.2f / 描述 Jaccard %.2f）",
				sim, NameSim(a.Name, b.Name), Jaccard(a.Description, b.Description))
			if c != nil && c.Config().Enabled() {
				v, err := judge(ctx, s, c, a, b)
				if err != nil {
					reason += "；GLM 判定失败: " + err.Error()
					fmt.Printf("[dup] %s#%d vs %s#%d 判定失败: %v\n", a.Name, a.ID, b.Name, b.ID, err)
				} else if !v.IsDuplicate {
					fmt.Printf("[dup] %s#%d vs %s#%d -> 非变体 (%.2f): %s\n", a.Name, a.ID, b.Name, b.ID, v.Confidence, v.Reason)
					continue
				} else {
					fmt.Printf("[dup] %s#%d vs %s#%d -> 变体 (%.2f): %s\n", a.Name, a.ID, b.Name, b.ID, v.Confidence, v.Reason)
					reason = fmt.Sprintf("GLM 置信 %.2f：%s", v.Confidence, v.Reason)
				}
			}
			if _, err := s.CreateDupGroup("fuzzy", sim, reason, []int64{a.ID, b.ID}); err == nil {
				created++
			}
		}
	}
	return created, nil
}

func judge(ctx context.Context, s *store.Store, c *llm.Client, a, b store.SkillRow) (*FuzzyVerdict, error) {
	var v FuzzyVerdict
	err := c.ChatJSON(ctx,
		"你是 skill 去重判定器。判断两个 agent skill 是否应视为重复项（is_duplicate=true）：即它们本质上是同一个 skill（同一功能意图，仅措辞、环境细节或版本差异，典型如同一 skill 分发到不同机器后的变体）。若两者是不同用途的 skill，即便名称相似、领域相邻也算 is_duplicate=false。",
		fmt.Sprintf("Skill A:\nname: %s\ndescription: %s\nSKILL.md 节选:\n%s\n\nSkill B:\nname: %s\ndescription: %s\nSKILL.md 节选:\n%s\n\n若 A 与 B 本质是同一个 skill 则 is_duplicate=true（同名且正文高度一致必须判 true）。\n输出 JSON，字段名必须精确为：{\"is_duplicate\": true或false, \"confidence\": 0到1小数, \"reason\": \"中文一句话理由\"}",
			a.Name, a.Description, mdExcerpt(s, a, 1200), b.Name, b.Description, mdExcerpt(s, b, 1200)),
		&v)
	return &v, err
}
