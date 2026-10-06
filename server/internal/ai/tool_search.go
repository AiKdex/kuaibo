// tool_search.go 实现 search_files 文件检索工具：AI 理解自然语言 → 结构化条件 → 检索文件库。
// 检索融合三层：关键词（FTS LIKE）→ LLM 语义扩展 → 向量相似度（embedding），RRF 加权排序。
// 知识库/图谱/对话 Agent 可直接复用同一工具（Tool 框架的可组合性）。
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// NewSearchFilesTool 创建 search_files 工具。
// spaceID 单用户阶段取 home space；多用户阶段按请求上下文解析。
// gate 可选：语义扩展（LLM）+ 向量检索（需 db/vecModel 齐备）；缺省降级纯关键词。
func NewSearchFilesTool(files *service.FileStore, spaceID string, gate *Gateway, db *sql.DB, vecModel string) *Tool {
	return &Tool{
		Name:        "search_files",
		Description: "在用户的知识库中检索文件/文件夹。根据关键词、类型、MIME、最近修改时间查找匹配的文件，返回文件列表（含名称、类型、大小、路径、修改时间）。适合用户用自然语言描述想找的内容（如“找一下关于向量检索的文档”“最近上传的图片”）时调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "检索关键词（匹配文件名或内容），可留空但需配合其他条件"},
				"type":  map[string]any{"type": "string", "enum": []string{"file", "dir"}, "description": "限定文件类型：file=文件，dir=文件夹"},
				"mime":  map[string]any{"type": "string", "description": "限定文件类型（MIME 前缀），如 image/ 图片、text/markdown Markdown、audio/ 音频、video/ 视频"},
				"days":  map[string]any{"type": "integer", "description": "限定最近 N 天内修改的文件"},
				"limit": map[string]any{"type": "integer", "description": "返回条数上限（默认 10，最大 50）"},
			},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args struct {
				Query string `json:"query"`
				Type  string `json:"type"`
				Mime  string `json:"mime"`
				Days  int    `json:"days"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			// type 白名单
			typ := args.Type
			if typ != "" && typ != "file" && typ != "dir" {
				typ = ""
			}
			// 语义扩展：LLM 生成扩展检索词，与原 query 一起多词合并检索
			var queries []string
			var expanded []string
			if q := strings.TrimSpace(args.Query); q != "" {
				queries = append(queries, q)
				if ex := ExpandQuery(ctx, gate, q); len(ex) > 0 {
					expanded = ex
					queries = append(queries, ex...)
				}
			}
			limit := args.Limit
			if limit <= 0 {
				limit = 10
			}
			if limit > 50 {
				limit = 50
			}
			type agg struct {
				f     *service.File
				score int
				hitIn string
			}
			seen := map[string]*agg{}
			opts := service.SearchOpts{
				SpaceID: spaceID,
				Type:    typ,
				Mime:    args.Mime,
				Days:    args.Days,
				Limit:   limit,
			}
			for _, q := range queries {
				opts.Query = q
				hits, err := files.Search(ctx, opts)
				if err != nil {
					return "", err
				}
				for _, h := range hits {
					if a, ok := seen[h.File.ID]; ok {
						a.score += h.Score
						if hitRank(h.HitIn) > hitRank(a.hitIn) {
							a.hitIn = h.HitIn
						}
					} else {
						seen[h.File.ID] = &agg{f: h.File, score: h.Score, hitIn: h.HitIn}
					}
				}
			}
			// 按总分排序，取 top N
			sorted := make([]*agg, 0, len(seen))
			for _, a := range seen {
				sorted = append(sorted, a)
			}
			sort.Slice(sorted, func(i, j int) bool {
				if sorted[i].score != sorted[j].score {
					return sorted[i].score > sorted[j].score
				}
				return sorted[i].f.Name < sorted[j].f.Name
			})
			if len(sorted) > limit {
				sorted = sorted[:limit]
			}
			kwIDs := make([]string, 0, len(sorted))
			for _, a := range sorted {
				kwIDs = append(kwIDs, a.f.ID)
			}

			// 向量检索（语义相似），与关键词 RRF 融合
			vecUsed := false
			var fusedIDs []string
			q := strings.TrimSpace(args.Query)
			if q != "" && db != nil && vecModel != "" && typ != "dir" {
				if vecHits, err := VectorSearch(ctx, db, gate, vecModel, spaceID, q, limit*2); err == nil && len(vecHits) > 0 {
					vecUsed = true
					fusedIDs = RRF(kwIDs, vecHits, 60, limit)
				}
			}
			// 输出顺序：融合结果（有向量时）或关键词排序
			order := kwIDs
			if vecUsed {
				order = fusedIDs
			}
			// Rerank 精排（K18）：rerank 已配置时对候选文件二次打分重排，失败/未配置保持原序
			if len(order) > 1 && gate != nil && db != nil && q != "" && gate.RerankEnabled() {
				order = toolRerankFuse(ctx, db, gate, q, order)
			}
			byID := map[string]*agg{}
			for _, a := range sorted {
				byID[a.f.ID] = a
			}
			out := make([]map[string]any, 0, len(order))
			for _, id := range order {
				a, ok := byID[id]
				if !ok {
					// 向量独有命中：补文件信息
					f, err := files.Get(ctx, id)
					if err != nil || f == nil {
						continue
					}
					a = &agg{f: f, hitIn: "vector"}
				}
				f := a.f
				out = append(out, map[string]any{
					"id":         f.ID,
					"name":       f.Name,
					"kind":       f.Kind,
					"mime":       f.Mime,
					"size":       f.Size,
					"updated_at": f.UpdatedAt,
					"hit_in":     a.hitIn,
					"score":      a.score,
				})
			}
			b, err := json.Marshal(map[string]any{
				"count":    len(out),
				"files":    out,
				"expanded": expanded,
				"semantic": len(expanded) > 0,
				"vector":   vecUsed,
			})
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
	}
}

// hitRank 命中位置权重：name > content > filter。
func hitRank(hitIn string) int {
	switch hitIn {
	case "name":
		return 3
	case "content":
		return 2
	default:
		return 1
	}
}
