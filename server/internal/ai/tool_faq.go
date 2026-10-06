// tool_faq.go FAQ 检索工具（WeKnora 借鉴 A2，B27 自上游 bbd6764 移植）：问题即检索单元。
// Agent/知识库问答时可调用 faq_search 精确命中标准问/相似问，答案直接注入回答上下文。
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// faqSearchArgs FAQSearchTool 参数。
type faqSearchArgs struct {
	Query string `json:"query"`
}

// NewFAQSearchTool 创建 FAQ 检索工具（spaceID 固定为当前知识库空间；kb 为 nil 时不注册）。
func NewFAQSearchTool(kb *service.KBStore, spaceID string) *Tool {
	return &Tool{
		Name:        "faq_search",
		Description: "在知识库 FAQ 中检索问题：输入用户问题，返回匹配的标准问与答案（问题即检索单元，适合高频疑问类问答）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "用户问题原文"},
			},
			"required": []string{"query"},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args faqSearchArgs
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", fmt.Errorf("faq_search: bad args: %w", err)
			}
			if kb == nil {
				return "", fmt.Errorf("faq_search: KBStore 未注入")
			}
			items, err := kb.SearchFAQ(ctx, spaceID, args.Query, 5)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return `{"hits": []}`, nil
			}
			type hit struct {
				StandardQ string   `json:"standard_q"`
				SimilarQs []string `json:"similar_qs,omitempty"`
				Answer    string   `json:"answer"`
				Tags      []string `json:"tags,omitempty"`
			}
			hits := make([]hit, 0, len(items))
			for _, f := range items {
				hits = append(hits, hit{f.StandardQ, f.SimilarQs, f.Answer, f.Tags})
			}
			b, _ := json.Marshal(map[string]any{"hits": hits})
			return string(b), nil
		},
	}
}
