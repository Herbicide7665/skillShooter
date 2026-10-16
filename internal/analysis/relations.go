package analysis

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"skill-manager/internal/llm"
	"skill-manager/internal/store"
)

// ScanReferenceEdges 解析各 SKILL.md 正文/描述中对库内其他 skill 名字的引用，建引用边
func ScanReferenceEdges(s *store.Store) (int, error) {
	if err := s.ClearEdges("reference"); err != nil {
		return 0, err
	}
	skills, err := s.ListSkills("", "")
	if err != nil {
		return 0, err
	}
	names := map[int64]string{}
	for _, sk := range skills {
		names[sk.ID] = sk.Name
	}
	// 名字长度降序，防止短名误匹配长名子串
	ordered := make([]store.SkillRow, len(skills))
	copy(ordered, skills)
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i].Name) > len(ordered[j].Name) })

	added := 0
	for _, sk := range skills {
		text := sk.Description
		if md, err := s.LatestSkillMD(sk.ID); err == nil {
			text += "\n" + md
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		for _, other := range ordered {
			if other.ID == sk.ID || other.Name == "" || len(other.Name) < 3 {
				continue
			}
			// 同名跨机变体互指无信息量，跳过
			if other.Name == sk.Name {
				continue
			}
			// 字母边界环视，避免子串误中（如 "es" 误中 "VMESS"）
			pat := regexp.MustCompile(`(?i)(^|[^a-z0-9_-])` + regexp.QuoteMeta(other.Name) + `($|[^a-z0-9_-])`)
			if pat.MatchString(text) {
				if err := s.AddEdge(sk.ID, other.ID, "reference", "SKILL.md 正文/描述中引用"); err == nil {
					added++
				}
			}
		}
	}
	return added, nil
}

// ---- GLM 归纳相关与调用链 ----

type ChainEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // related | chain
	Note string `json:"note"`
}

type llmRelation struct {
	Edges []ChainEdge `json:"edges"`
}

// ScanLLMRelations GLM 归纳"相关 skill"与典型调用链，落 kind=related/chain 边
func ScanLLMRelations(ctx context.Context, s *store.Store, c *llm.Client, maxSkills int) (int, error) {
	if !c.Config().Enabled() {
		return 0, fmt.Errorf("GLM 未配置")
	}
	if err := s.ClearEdges("related"); err != nil {
		return 0, err
	}
	if err := s.ClearEdges("chain"); err != nil {
		return 0, err
	}
	skills, err := s.ListSkills("", "")
	if err != nil {
		return 0, err
	}
	if len(skills) > maxSkills {
		skills = skills[:maxSkills]
	}
	// 目录：name -> id（去重后按最短 description 的那台机器为代表；仅用于生成边，两端点以名字回填）
	nameToID := map[string][]int64{}
	var catalog strings.Builder
	for _, sk := range skills {
		nameToID[sk.Name] = append(nameToID[sk.Name], sk.ID)
		desc := sk.Description
		if len(desc) > 160 {
			desc = desc[:160]
		}
		fmt.Fprintf(&catalog, "- %s: %s\n", sk.Name, desc)
	}
	var rel llmRelation
	err = c.ChatJSON(ctx,
		`你是 agent skill 关系分析器。下面是 skill 清单（name + description）。找出：
1. related：语义上强相关、常一起讨论的 skill 对（不是重复，而是互补/邻近领域）；
2. chain：典型工作流里 A 调用后触发 B 的先后调用链（A→B 表示 A 的输出是 B 的输入或 A 的流程需要 B 跟进）。
只输出基于 description 可判断的关系，宁缺毋滥，最多 25 条。输出 JSON：{"edges":[{"from":"名字","to":"名字","kind":"related|chain","note":"一句话理由"}]}`,
		catalog.String(), &rel)
	if err != nil {
		return 0, err
	}
	added := 0
	for _, e := range rel.Edges {
		kind := strings.ToLower(strings.TrimSpace(e.Kind))
		if kind != "related" && kind != "chain" {
			continue
		}
		if strings.TrimSpace(e.From) == strings.TrimSpace(e.To) {
			continue
		}
		srcs, ok1 := nameToID[strings.TrimSpace(e.From)]
		dsts, ok2 := nameToID[strings.TrimSpace(e.To)]
		if !ok1 || !ok2 {
			continue
		}
		// 同名多机实体只取第一个，避免同一条关系复制多份
		if len(srcs) > 1 {
			srcs = srcs[:1]
		}
		if len(dsts) > 1 {
			dsts = dsts[:1]
		}
		for _, sid := range srcs {
			for _, did := range dsts {
				if sid == did {
					continue
				}
				if err := s.AddEdge(sid, did, kind, e.Note); err == nil {
					added++
				}
			}
		}
	}
	return added, nil
}
