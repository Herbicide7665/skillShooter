package analysis

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"skill-hub/internal/llm"
	"skill-hub/internal/store"
)

// 家族：由 GLM 按【实际功能与内容】聚类（命名相似仅作线索，功能相同才成族）。
// 只在用户点击「更新关系」时重构并固化入库；平时展示读库结果，不触发任何 LLM 调用。

const familyChunkSize = 80 // 每批送检的 skill 数
const familyMinMembers = 3 // 每族最少不同名字数

type FamilyDraft struct {
	Key     string
	Note    string
	Members []store.SkillRow
}

type llmFamilies struct {
	Families []struct {
		Key     string   `json:"key"`
		Note    string   `json:"note"`
		Members []string `json:"members"`
	} `json:"families"`
}

// ScanFamilies GLM 内容验证聚类：分批送检→合并→映射实体→全量替换入库
func ScanFamilies(ctx context.Context, s *store.Store, c *llm.Client) (int, error) {
	if !c.Config().Enabled() {
		return 0, fmt.Errorf("GLM 未配置")
	}
	skills, err := s.ListSkills("", "")
	if err != nil {
		return 0, err
	}
	if len(skills) == 0 {
		return 0, nil
	}
	// 目录按名字去重（跨机变体一份），稳定排序分批
	uniq := map[string]store.SkillRow{}
	for _, sk := range skills {
		if _, ok := uniq[sk.Name]; !ok {
			uniq[sk.Name] = sk
		}
	}
	names := make([]string, 0, len(uniq))
	for n := range uniq {
		names = append(names, n)
	}
	sort.Strings(names)

	merged := map[string]*FamilyDraft{} // key(lower) -> draft
	for start := 0; start < len(names); start += familyChunkSize {
		end := start + familyChunkSize
		if end > len(names) {
			end = len(names)
		}
		var catalog strings.Builder
		for _, n := range names[start:end] {
			desc := uniq[n].Description
			if len(desc) > 200 {
				desc = desc[:200]
			}
			fmt.Fprintf(&catalog, "- %s: %s\n", n, desc)
		}
		var rel llmFamilies
		err := c.ChatJSON(ctx,
			`你是 agent skill 分类器。输入一批 skill 的名称与描述，请按【实际功能与内容】把它们归成若干家族：同一家族 = 服务同一类目/同一场景的 skill。命名模式（如共享词根）只是线索，最终以功能是否同类为准；命名相似但功能不同的不要硬归；不确定或无关的条目直接不分类，宁缺毋滥。每族至少 3 个成员。
输出 JSON：{"families":[{"key":"简短中文类别名","note":"该族共同点一句话","members":["精确 skill 名",...]}]}`,
			catalog.String(), &rel)
		if err != nil {
			return 0, fmt.Errorf("GLM 分批聚类失败(第 %d 批): %w", start/familyChunkSize+1, err)
		}
		for _, f := range rel.Families {
			key := strings.ToLower(strings.TrimSpace(f.Key))
			if key == "" {
				continue
			}
			draft := merged[key]
			if draft == nil {
				draft = &FamilyDraft{Key: strings.TrimSpace(f.Key), Note: strings.TrimSpace(f.Note)}
				merged[key] = draft
			}
			for _, n := range f.Members {
				n = strings.TrimSpace(n)
				if sk, ok := uniq[n]; ok {
					// 同族成员按名字取全部跨机变体
					for _, v := range skills {
						if v.Name == n {
							draft.Members = append(draft.Members, v)
						}
					}
					_ = sk
				}
			}
		}
	}
	// 去重成员 + 最少成员数过滤 + 入库
	var out []store.FamilyRow
	for _, d := range merged {
		uniqNames := map[string]bool{}
		seen := map[int64]bool{}
		var members []store.SkillRow
		for _, m := range d.Members {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			uniqNames[m.Name] = true
			members = append(members, m)
		}
		if len(uniqNames) < familyMinMembers {
			continue
		}
		sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
		out = append(out, store.FamilyRow{Key: d.Key, Note: d.Note, Members: members})
	}
	if len(out) == 0 {
		return 0, fmt.Errorf("GLM 未产出满足最少成员数的家族")
	}
	if err := s.ReplaceFamilies(out); err != nil {
		return 0, err
	}
	return len(out), nil
}
