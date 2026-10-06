// tool_memory.go 空间长期记忆工具（WeKnora 借鉴 A3，B27 自上游 55683ab 移植）：
//   memory_context 读取空间记忆汇总（Agent 开场合入回答上下文）；
//   memory_set     写入一条记忆（Agent 沉淀"值得记住"的事实/偏好，供后续任务复用）。
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// NewMemoryContextTool 创建空间记忆读取工具。
func NewMemoryContextTool(kb *service.KBStore, spaceID string) *Tool {
	return &Tool{
		Name:        "memory_context",
		Description: "读取当前空间的长期记忆（用户偏好/关键事实/待办事项/兴趣），回答与任务开工前调用以贴合用户背景。",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			if kb == nil {
				return "", fmt.Errorf("memory_context: KBStore 未注入")
			}
			text, err := kb.MemoryContext(ctx, spaceID, 40)
			if err != nil {
				return "", err
			}
			if text == "" {
				return `{"context": "", "empty": true}`, nil
			}
			b, _ := json.Marshal(map[string]any{"context": text, "empty": false})
			return string(b), nil
		},
	}
}

// NewMemorySetTool 创建空间记忆写入工具。
func NewMemorySetTool(kb *service.KBStore, spaceID, actorID string) *Tool {
	return &Tool{
		Name:        "memory_set",
		Description: "向空间长期记忆写入一条事实/偏好/事项（kind: profile|fact|preference|todo|interest|note；key 为语义键名）。用户明确表达的可复用信息应沉淀，避免后续重复询问。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind":    map[string]any{"type": "string", "enum": []string{"profile", "fact", "preference", "todo", "interest", "note"}, "description": "记忆类型"},
				"key":     map[string]any{"type": "string", "description": "语义键名，如 owner_preferred_lang"},
				"content": map[string]any{"type": "string", "description": "记忆内容"},
			},
			"required": []string{"kind", "key", "content"},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args struct {
				Kind    string `json:"kind"`
				Key     string `json:"key"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", fmt.Errorf("memory_set: bad args: %w", err)
			}
			if kb == nil {
				return "", fmt.Errorf("memory_set: KBStore 未注入")
			}
			if err := kb.MemorySet(ctx, &service.Memory{
				SpaceID: spaceID, Kind: args.Kind, Key: args.Key,
				Content: args.Content, Source: "agent", CreatedBy: actorID,
			}); err != nil {
				return "", err
			}
			return fmt.Sprintf(`{"ok": true, "key": %q}`, args.Key), nil
		},
	}
}
