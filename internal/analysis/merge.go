package analysis

import (
	"context"
	"fmt"
	"strings"

	"encoding/json"

	"skillshooter/internal/llm"
	"skillshooter/internal/store"
)

// MergeProposal GLM 生成合并建议
type MergeProposal struct {
	SkillMD     string `json:"skill_md"`
	Description string `json:"description"`
	Notes       string `json:"notes"`
}

// UnmarshalJSON 宽松解析：兼容 GLM 偶发的中文字段名
func (p *MergeProposal) UnmarshalJSON(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	p.SkillMD = strVal(m, "skill_md", "skillmd", "merged_skill_md", "合并后skill", "合并后的SKILL.md")
	p.Description = strVal(m, "description", "描述", "新描述")
	p.Notes = strVal(m, "notes", "合并要点", "说明", "备注")
	return nil
}

// ProposeMerge 对一个重复组生成合并后的 SKILL.md 建议
func ProposeMerge(ctx context.Context, s *store.Store, c *llm.Client, gid int64) (*MergeProposal, error) {
	if !c.Config().Enabled() {
		return nil, fmt.Errorf("GLM 未配置，无法生成合并建议")
	}
	g, err := s.GetDupGroup(gid)
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	for i, m := range g.Members {
		md, _ := s.LatestSkillMD(m.ID)
		fmt.Fprintf(&sb, "===== 变体 %d（id=%d name=%s machine=%s）=====\n%s\n\n", i+1, m.ID, m.Name, m.Machine, md)
	}
	var p MergeProposal
	err = c.ChatJSON(ctx,
		`你是 skill 合并专家。输入是同一个 skill 在不同机器上的多个变体（各含完整 SKILL.md），请生成一份合并后的最佳 SKILL.md。
原则：
1. 融合所有变体中有价值的独特内容，去掉纯冗余；冲突时取更准确、更完整的表述。
2. 保留各变体独有的操作细节、路径、命令（不同机器环境信息用注释标注来源）。
3. 输出 JSON 对象：{"description": "合并后 frontmatter 的 description 单行最优表述", "skill_md": "完整合并后的 SKILL.md 文本（含 --- 包裹的 frontmatter，name 取最规范者）", "notes": "合并要点一两句话"}`,
		sb.String(), &p)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.SkillMD) == "" {
		return nil, fmt.Errorf("GLM 返回了空合并建议")
	}
	return &p, nil
}

// AdoptMerge 用户采纳：把建议落为新版本
func AdoptMerge(ctx context.Context, s *store.Store, gid int64, p *MergeProposal) (int64, error) {
	g, err := s.GetDupGroup(gid)
	if err != nil {
		return 0, err
	}
	if len(g.Members) == 0 {
		return 0, fmt.Errorf("组无成员")
	}
	// 合并版本落在成员列表第一个 skill 上
	target := g.Members[0]
	// 从建议的 SKILL.md 解析 name/description（简化：description 用建议值）
	files := map[string][]byte{"SKILL.md": []byte(p.SkillMD)}
	srcIDs := []int64{}
	for _, m := range g.Members {
		if m.LatestVer > 0 {
			srcIDs = append(srcIDs, m.LatestVer)
		}
	}
	vid, err := s.CreateMergedVersion(target.ID, files, srcIDs, p.Description)
	if err != nil {
		return 0, err
	}
	_ = s.SetDupGroupStatus(gid, "merged")
	return vid, nil
}
