package handler

// blog_related.go 相关文章推荐（公开只读）。
//
// 与插件端「同分类 + 时间排序」的兜底逻辑不同，这里走后端语义向量：
// 以源文章各索引块向量的**质心**为查询，对同空间其他已发布文章做余弦相似度，
// 每篇取其最高分块（口径与 search 的向量召回一致，见 ai.RelatedByVector）。
//
// 设计约束：
//   - 只在「已发布 + 公开可见」的文章集合内推荐（复用 blogPostsHTML 收集口径，
//     草稿/定时未到/附件天然不在其中），不泄露私密内容；
//   - 源文章未 embedding（冷启动）或向量维度不一致时返回空列表，由前端降级到
//     原有同分类排序——推荐是增强项，永不阻断文章页渲染。

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
)

// publicBlogRelated GET /api/v1/public/blog/related?slug=...&limit=5
func (a *API) publicBlogRelated(w http.ResponseWriter, r *http.Request) {
	if !a.blogOpen(r) {
		writeErr(w, http.StatusNotFound, "BLOG_CLOSED", "博客已关闭")
		return
	}
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	if slug == "" {
		writeErr(w, http.StatusBadRequest, "RELATED_NO_SLUG", "slug 必填")
		return
	}
	limit := 5
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 20 {
		limit = v
	}
	// 公开轨已发布文章集合（与公开列表/正文/TTS 同源）
	posts, err := a.blogPostsHTML(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	var src *dirFile
	byID := map[string]*dirFile{}
	for i := range posts {
		byID[posts[i].f.ID] = &posts[i]
		if slugOf(posts[i].f, posts[i].path) == slug {
			src = &posts[i]
		}
	}
	if src == nil {
		writeErr(w, http.StatusNotFound, "RELATED_POST_NOT_FOUND", "文章不存在")
		return
	}
	// 锁定态（密码/付费）不参与推荐外泄：源被锁则不给推荐
	if a.articleLocked(r, src.f.ID) || a.paidLocked(r, src.f.ID) {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	model := a.ai.EmbeddingModel()
	spaceID := src.f.SpaceID
	hits, err := ai.RelatedByVector(r.Context(), a.db, model, spaceID, src.f.ID, limit)
	if err != nil || len(hits) == 0 {
		// 冷启动/无向量：返回空，前端降级同分类排序
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	items := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		p, ok := byID[h.FileID]
		if !ok {
			continue // 非公开/已删（理论上集合已过滤，保险起见）
		}
		// 目标也被密码/付费锁 → 不外泄（不泄露其存在与标题）
		if a.articleLocked(r, p.f.ID) || a.paidLocked(r, p.f.ID) {
			continue
		}
		items = append(items, map[string]any{
			"id":    p.f.ID,
			"slug":  slugOf(p.f, p.path),
			"title": p.f.Name,
			"path":  p.path,
			"score": h.Score,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
