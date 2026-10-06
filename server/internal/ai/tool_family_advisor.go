// tool_family_advisor.go 实现 family_advisor 工具：AI 人生参谋（家族传承二期①）。
// 能力：读取当前用户的家族档案（成员/关系/纪念日/一生时间轴摘要），供 LLM/Agent
// 在人生规划、纪念日策划、家族历史整理等参谋场景引用；档案按 user_id 隔离。
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// NewFamilyAdvisorTool 创建 family_advisor 工具。
// fam 为家族传承模块服务（nil=模块未启用则不注册，由 main 装配判断）。
func NewFamilyAdvisorTool(fam *service.FamilyStore) *Tool {
	return &Tool{
		Name:        "family_advisor",
		Description: "读取当前用户的家族档案摘要（家族成员、关系、纪念日、一生时间轴），用于人生规划参谋、纪念日策划、家族历史整理、长辈记忆梳理等场景。当用户问及家族/家庭/人生相关规划时调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{"type": "string", "description": "用户当前问题（可选，用于聚焦摘要范围）"},
				"limit":    map[string]any{"type": "integer", "description": "时间轴返回条数上限（默认 20，最大 50）"},
			},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args struct {
				Question string `json:"question"`
				Limit    int    `json:"limit"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			if fam == nil {
				return "", fmt.Errorf("家族传承模块未启用")
			}
			if args.Limit <= 0 {
				args.Limit = 20
			}
			if args.Limit > 50 {
				args.Limit = 50
			}
			// 单用户阶段 owner 固定（与 search_files 等工具同约定）；多用户扩展时由调用方注入 uid。
			summary, err := BuildFamilyContext(ctx, fam, service.SystemOwnerID, args.Limit)
			if err != nil {
				return "", err
			}
			b, _ := json.Marshal(map[string]any{
				"family_context": summary,
				"note":           "以上为当前用户家族档案摘要；如用户问题需要更细的某位成员/某个纪念日，可继续追问。",
			})
			return string(b), nil
		},
	}
}

// BuildFamilyContext 组装家族档案摘要文本（成员/关系/纪念日/时间轴）。
// 供 family_advisor 工具与 /api/v1/family/advise 端点共用。
func BuildFamilyContext(ctx context.Context, fam *service.FamilyStore, uid string, limit int) (string, error) {
	var sb strings.Builder

	members, err := fam.ListMembers(ctx, uid)
	if err != nil {
		return "", err
	}
	sb.WriteString("【家族成员】\n")
	if len(members) == 0 {
		sb.WriteString("（暂无成员记录）\n")
	}
	for _, m := range members {
		sb.WriteString(fmt.Sprintf("- %s（%s，%s；%s）\n",
			m.Name, genderLabel(m.Gender), orDash(m.BirthDate), orDash(m.Note)))
	}

	rels, err := fam.ListRelations(ctx, uid)
	if err != nil {
		return "", err
	}
	nameByID := map[string]string{}
	for _, m := range members {
		nameByID[m.ID] = m.Name
	}
	sb.WriteString("\n【家族关系】\n")
	if len(rels) == 0 {
		sb.WriteString("（暂无关系记录）\n")
	}
	for _, r := range rels {
		from := nameByID[r.FromID]
		to := nameByID[r.ToID]
		if from == "" {
			from = r.FromID[:8]
		}
		if to == "" {
			to = r.ToID[:8]
		}
		sb.WriteString(fmt.Sprintf("- %s → %s（%s）\n", from, to, relationLabel(r.Relation)))
	}

	annivs, err := fam.ListAnniversaries(ctx, uid)
	if err != nil {
		return "", err
	}
	sb.WriteString("\n【纪念日】\n")
	if len(annivs) == 0 {
		sb.WriteString("（暂无纪念日记录）\n")
	}
	for _, a := range annivs {
		ref := ""
		if a.RefMemberID != "" {
			ref = "，" + nameByID[a.RefMemberID]
		}
		remind := ""
		if a.RemindDays > 0 {
			remind = fmt.Sprintf("，提前%d天提醒", a.RemindDays)
		}
		sb.WriteString(fmt.Sprintf("- %s（%s，%s%s%s）\n", a.Title, annivKindLabel(a.Kind), a.Date, ref, remind))
	}

	tl, err := fam.Timeline(ctx, uid)
	if err != nil {
		return "", err
	}
	sb.WriteString("\n【一生时间轴（最近" + fmt.Sprintf("%d", minInt(limit, len(tl))) + "条）】\n")
	if len(tl) == 0 {
		sb.WriteString("（暂无人生事件记录；提示：在知识库文档中把文章类型设为「人生事件」并填写发生时间即入轴）\n")
	}
	for i := 0; i < len(tl) && i < limit; i++ {
		it := tl[i]
		sb.WriteString(fmt.Sprintf("- %s｜%s%s（%s）\n",
			time.Unix(it.OccurredAt, 0).Format("2006-01-02"), it.Title, stageSuffix(it.Stage), orDash(it.Preview)))
	}
	return sb.String(), nil
}

func genderLabel(g string) string {
	switch g {
	case "male":
		return "男"
	case "female":
		return "女"
	default:
		return "性别未填"
	}
}

func relationLabel(r string) string {
	switch r {
	case "parent_of":
		return "父母"
	case "child_of":
		return "子女"
	case "spouse_of":
		return "配偶"
	case "sibling_of":
		return "兄弟姐妹"
	default:
		return "其他"
	}
}

func annivKindLabel(k string) string {
	switch k {
	case "birthday":
		return "生日"
	case "anniversary":
		return "纪念日"
	default:
		return "自定义"
	}
}

func stageSuffix(s string) string {
	if s == "" {
		return ""
	}
	return "｜阶段:" + s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
