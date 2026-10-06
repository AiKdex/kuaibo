// Package ai 的 tools.go 实现 Tool 框架：工具注册表 + OpenAI function-calling schema。
//
// 设计要点（实施文档 §13.3 工具层 / 商业价值点）：
//   - 工具即能力：名称 + 描述 + JSON Schema 参数 + 执行函数，注册后可被 Agent/知识库/图谱复用；
//   - 可插拔：本地新增工具只需 Register，LLM 侧自动透出（gateway 层拼 tools 参数）；
//   - 面向商业化：工具可作为"下载+授权 / 网关调用"的最小单元分发（§13.3 可选策略）。
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// ToolFunc 工具执行函数：入参为 JSON RawMessage，返回结果 JSON 字符串。
type ToolFunc func(ctx context.Context, args json.RawMessage) (string, error)

// Tool 注册的工具定义。
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema（object 型）
	Run         ToolFunc       `json:"-"`
}

// Registry 工具注册表（并发安全）。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Tool
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{tools: map[string]*Tool{}}
}

// Register 注册工具（同名覆盖）。
func (r *Registry) Register(t *Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
}

// Get 按名取工具。
func (r *Registry) Get(name string) (*Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List 全部工具（按名排序，稳定输出）。
func (r *Registry) List() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Schema 输出 OpenAI function-calling 的 tools 数组（透传给 LLM）。
func (r *Registry) Schema() []map[string]any {
	ts := r.List()
	out := make([]map[string]any, 0, len(ts))
	for _, t := range ts {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}
	return out
}

// Exec 执行工具，返回结果 JSON 字符串。
func (r *Registry) Exec(ctx context.Context, name string, args json.RawMessage) (string, error) {
	t, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("tool %q not registered", name)
	}
	return t.Run(ctx, args)
}
