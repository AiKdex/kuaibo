// queryexpand.go query 语义扩展：用 LLM 把用户的一句话查询扩展成多个检索关键词，
// 弥补关键词检索的语义 gap（中文场景尤其有效：同义词/下位词/英文术语）。
// 这是"语义搜索"的第一阶段（不依赖 embedding API——agnes 网关无 embedding 模型）；
// 真向量检索作为后续阶段在此之上叠加融合。
package ai

import (
	"context"
	"encoding/json"
	"strings"
)

// ExpandQuery 把用户查询扩展为检索关键词列表。
// 返回 nil 表示无需扩展或扩展失败（调用方应使用原始 query 检索）。
// gate 为 nil 时直接返回 nil（降级纯关键词）。
func ExpandQuery(ctx context.Context, gate *Gateway, query string) []string {
	query = strings.TrimSpace(query)
	if gate == nil || query == "" {
		return nil
	}
	msgs := []Msg{
		{
			Role:    "system",
			Content: "你是检索词扩展助手。把用户的一句话查询扩展成 6-12 个检索关键词，覆盖：原词、同义词、下位词、英文术语、常见别称、专业表达。只输出 JSON 字符串数组，不要任何解释或前缀。示例输入：找向量迁移方案；示例输出：[\"向量\",\"迁移\",\"方案\",\"vector\",\"qdrant\",\"索引\",\"向量数据库\"]",
		},
		{Role: "user", Content: query},
	}
	out, err := gate.Ask(ctx, msgs)
	if err != nil {
		return nil
	}
	return parseExpanded(out)
}

// parseExpanded 从 LLM 输出中解析关键词数组（容忍包裹/杂讯）。
func parseExpanded(s string) []string {
	s = strings.TrimSpace(s)
	// 尝试整体 JSON 数组
	var arr []string
	if json.Unmarshal([]byte(s), &arr) == nil && len(arr) > 0 {
		return cleanTerms(arr)
	}
	// 截取 [ ] 之间的部分再试
	if i := strings.IndexByte(s, '['); i >= 0 {
		if j := strings.LastIndexByte(s, ']'); j > i {
			if json.Unmarshal([]byte(s[i:j+1]), &arr) == nil && len(arr) > 0 {
				return cleanTerms(arr)
			}
		}
	}
	// 最后兜底：按逗号/顿号/换行拆分
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == '\n' || r == ';' || r == '；'
	})
	return cleanTerms(parts)
}

func cleanTerms(terms []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" || len(t) > 20 {
			continue
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	if len(out) > 15 {
		out = out[:15]
	}
	return out
}
