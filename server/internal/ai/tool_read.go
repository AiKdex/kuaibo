// tool_read.go 实现 read_file 工具：读取指定文件的正文内容。
// 与 search_files 组合成"检索→精读"闭环：先按关键词定位文件，再读取正文供 LLM 总结/引用。
// 只读文本类文件，超长截断；非文本（音视频/图片）返回明确提示。
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// NewReadFileTool 创建 read_file 工具。sum 可选：已入库解读的文件优先返回摘要
// （秒回），未解读/解读中才临时读正文兜底。
func NewReadFileTool(files *service.FileStore, sum *Summarizer) *Tool {
	return &Tool{
		Name:        "read_file",
		Description: "读取指定文件的内容（正文或 AI 已入库解读摘要）。当用户想了解文件里写了什么、要求总结文档或引用原文时调用；file_id 通常来自 search_files 的检索结果。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_id":   map[string]any{"type": "string", "description": "要读取的文件 ID"},
				"max_chars": map[string]any{"type": "integer", "description": "返回正文的最大字符数（默认 3000，最大 8000；仅临时读正文时生效）"},
			},
			"required": []string{"file_id"},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args struct {
				FileID   string `json:"file_id"`
				MaxChars int    `json:"max_chars"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			if args.FileID == "" {
				return "", errors.New("file_id 不能为空")
			}
			if args.MaxChars <= 0 {
				args.MaxChars = 3000
			}
			if args.MaxChars > 8000 {
				args.MaxChars = 8000
			}
			f, err := files.Get(ctx, args.FileID)
			if err != nil {
				return "", err
			}
			if f.Kind != "file" {
				return "", errors.New("目标不是文件")
			}
			// 优先复用入库解读（AI 已提前解读过 → 秒回，不临时读正文）
			if sum != nil {
				if row, _ := sum.Get(ctx, args.FileID); row != nil && row.Status == "done" {
					b, _ := json.Marshal(map[string]any{
						"file_id": f.ID, "name": f.Name, "mime": f.Mime, "size": f.Size,
						"readable":   true,
						"ai_summary": true,
						"summary":    row.Summary,
						"tags":       row.Tags,
						"meta":       row.Meta,
						"source":     row.Source,
						"note":       "以下为 AI 入库解读摘要（未包含全文）",
					})
					return string(b), nil
				}
			}
			if !service.IndexableMIME(f.Mime) {
				b, _ := json.Marshal(map[string]any{
					"file_id": f.ID, "name": f.Name, "mime": f.Mime, "size": f.Size,
					"readable": false,
					"note":     "该文件不是文本类（" + f.Mime + "），无法读取正文。可尝试用 search_files 按文件名/类型检索。",
				})
				return string(b), nil
			}
			rc, _, err := files.Content(ctx, args.FileID)
			if err != nil {
				return "", err
			}
			defer rc.Close()
			data, err := io.ReadAll(io.LimitReader(rc, int64(args.MaxChars*4)))
			if err != nil {
				return "", err
			}
			content := string(data)
			truncated := false
			if r := []rune(content); len(r) > args.MaxChars {
				content = string(r[:args.MaxChars])
				truncated = true
			}
			b, err := json.Marshal(map[string]any{
				"file_id": f.ID, "name": f.Name, "mime": f.Mime, "size": f.Size,
				"readable": true, "content": content, "truncated": truncated,
			})
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
	}
}
