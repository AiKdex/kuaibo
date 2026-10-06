// tool_webclip.go 实现 web_clip 网页剪藏工具：对话中给出 URL → 抓取正文 → 入库知识库。
//
// 对齐上游《AgentTools开发规范》官方工具注册矩阵中的 tool_web_clip（AiKlog 侧补注册）。
// 复用内核 impex.CreateURLImport（零私有依赖），任务创建后由 impex 执行器在后台抓取解析。
// 构造器签名与上游主系统保持一致（server/cmd/aikmap/main.go:226），便于后续对齐/移植。
package ai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// NewWebClipTool 创建 web_clip 工具。
//
// 参数对齐上游：impex=导入导出服务；files=文件库（预留，后续接入 blogPostCreate 草稿态）；
// ownerID/homeSpaceID 为单用户阶段固定归属（SystemOwnerID / SystemHomeSpaceID），
// 多用户阶段按请求上下文解析。
func NewWebClipTool(impex *service.ImpexStore, files *service.FileStore, ownerID, homeSpaceID string) *Tool {
	_ = files // 预留：后续接入「剪藏为草稿文章（blogPostCreate 草稿态）」时使用
	return &Tool{
		Name: "web_clip",
		Description: "把指定网页抓取并剪藏到知识库：给定一个 URL，系统会抓取网页正文并转为 Markdown 条目存入知识库，可指定目标目录。" +
			"适合用户说「把这篇网页存下来」「剪藏这个链接」「收藏这个页面」「帮我保存这篇文章的网页」时调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":    map[string]any{"type": "string", "description": "要剪藏的网页地址（仅支持 http/https）"},
				"parent": map[string]any{"type": "string", "description": "可选：目标目录路径；缺省落到知识库根目录"},
				"ua":     map[string]any{"type": "string", "description": "可选：自定义 User-Agent（部分站点需要）"},
			},
			"required": []string{"url"},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args struct {
				URL    string `json:"url"`
				Parent string `json:"parent"`
				UA     string `json:"ua"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			u := strings.TrimSpace(args.URL)
			if u == "" {
				return `{"ok":false,"error":"url 不能为空"}`, nil
			}
			if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
				return `{"ok":false,"error":"仅支持 http/https 链接"}`, nil
			}
			job, err := impex.CreateURLImport(ctx, ownerID, homeSpaceID, args.Parent, u, args.UA)
			if err != nil {
				b, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
				return string(b), nil
			}
			b, _ := json.Marshal(map[string]any{
				"ok":      true,
				"job_id":  job.ID,
				"status":  job.Status,
				"source":  job.Source,
				"url":     u,
				"message": "已创建剪藏任务，后台将抓取正文并入库；可用导入任务列表查看进度",
			})
			return string(b), nil
		},
	}
}
