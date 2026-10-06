// search.go 实现全库检索 API：GET /api/v1/search?q=xxx&limit=N
// 关键词（files.Search 文件名/内容）+ 向量语义（ai.VectorSearch）RRF 融合，
// 与 AI 工具的 search_files 共用同一套后端能力；前端检索、知识库检索、博客检索都可复用。
package handler

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// searchChunk 溯源定位块（知识库可信度：回答/结果可点回原文段落）。
type searchChunk struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Anchor  string `json:"anchor,omitempty"` // 定位锚点：渲染后可匹配的稳定纯文本
}

// searchItem 检索结果项（前端直接可渲染）。
type searchItem struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Kind      string       `json:"kind"`
	Size      int64        `json:"size"`
	Mime      string       `json:"mime"`
	ParentID  string       `json:"parent_id"`
	Path      string       `json:"path"`     // 完整路径，如 /我的空间/docs/a.md
	Score     int          `json:"score"`    // 3=文件名 2=内容 1=过滤/仅向量命中 0=占位
	VecScore  float64      `json:"vec_score,omitempty"` // 语义相似度 0..1（向量召回时带入，供同类内精排）
	HitIn     string       `json:"hit_in"`   // name | content | filter | vector
	Chunk     *searchChunk `json:"chunk,omitempty"` // 命中块（content/vector 命中时返回，供片段展示与定位）
	UpdatedAt int64        `json:"updated_at"`
}

// search GET /api/v1/search?q=xxx&limit=N：关键词+向量 RRF 融合检索。
// containsStr 判断字符串切片是否包含指定值（搜索范围过滤用）。
func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func (a *API) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, http.StatusBadRequest, "SEARCH_NO_Q", "缺少检索关键词 q")
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := atoiSafe(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	ctx := r.Context()
	space := a.homeSpaceID()

	// 搜索范围（博客目录在存储层隐藏后的配套）：scope=all（默认，全部）/ file（排除博客子树）/ blog（仅博客子树）
	scope := r.URL.Query().Get("scope")
	var includeIDs, excludeIDs []string
	if scope == "blog" || scope == "file" {
		if ids, ok := a.blogScopeIDs(ctx); ok {
			if scope == "blog" {
				includeIDs = ids
			} else {
				excludeIDs = ids
			}
		}
	}

	// 1) 关键词检索（文件名/内容，命中带 score）
	kwHits, err := a.files.Search(ctx, service.SearchOpts{SpaceID: space, Query: q, Limit: limit, IncludeIDs: includeIDs, ExcludeIDs: excludeIDs})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SEARCH_FAILED", err.Error())
		return
	}
	type kwEntry struct {
		h service.SearchHit
	}
	kwIDs := make([]string, 0, len(kwHits))
	byID := make(map[string]service.SearchHit, len(kwHits))
	for _, h := range kwHits {
		if h.File == nil {
			continue
		}
		byID[h.File.ID] = h
		kwIDs = append(kwIDs, h.File.ID)
	}

	// 2) 向量语义检索（embedding 已配置时），与关键词 RRF 融合
	fused := kwIDs
	vecByID := map[string]ai.VecHit{}
	if model := a.ai.EmbeddingModel(); model != "" {
		if vecHits, verr := ai.VectorSearch(ctx, a.db, a.ai, model, space, q, limit*2); verr == nil && len(vecHits) > 0 {
			// 相关度门槛：过低相似度不进结果（防「无关键词时吐出全库弱相关」）
			minScore := 0.45
			if v, ok := a.cfg.Get("index.vector_min_score"); ok {
				if f, ok2 := v.(float64); ok2 && f > 0 {
					minScore = f
				}
			}
			strong := vecHits[:0]
			for _, v := range vecHits {
				if v.Score >= minScore {
					strong = append(strong, v)
				}
			}
			vecHits = strong
			// 范围过滤：scope=file 剔除博客子树命中；scope=blog 仅保留博客子树命中
			if len(includeIDs) > 0 || len(excludeIDs) > 0 {
				filtered := vecHits[:0]
				for _, v := range vecHits {
					if len(includeIDs) > 0 && !containsStr(includeIDs, v.FileID) {
						continue
					}
					if len(excludeIDs) > 0 && containsStr(excludeIDs, v.FileID) {
						continue
					}
					filtered = append(filtered, v)
				}
				vecHits = filtered
			}
			if len(vecHits) > 0 {
				fused = ai.RRF(kwIDs, vecHits, 60, limit)
				for _, v := range vecHits {
					if _, ok := vecByID[v.FileID]; !ok {
						vecByID[v.FileID] = v
					}
				}
			}
		}
	}

	// 2.5) Rerank 精排（K18）：rerank 已配置时，对 RRF 融合候选取文档片段二次打分重排，
	// 显著提升语义相关性排序；未配置/失败自动降级保持 RRF 原顺序。
	// 注意：精排成功后最终顺序以 rerank 为准，不得被下方命中层级 sort 覆盖（否则语义排序被抹掉）。
	reranked := false
	if len(fused) > 1 && a.rerankEnabled() {
		var ok bool
		fused, ok = a.rerankFuse(ctx, q, fused, vecByID)
		reranked = ok
	}

	// 3) 组装结果（含完整路径与溯源块）
	out := make([]searchItem, 0, len(fused))
	seen := map[string]bool{}
	for _, id := range fused {
		if seen[id] {
			continue
		}
		seen[id] = true
		h, ok := byID[id]
		if !ok {
			// 仅向量命中的文件（关键词未召回）：补查一次
			f, gerr := a.files.Get(ctx, id)
			if gerr != nil || f == nil {
				continue
			}
			out = append(out, searchItem{
				ID: f.ID, Name: f.Name, Kind: f.Kind, Size: f.Size, Mime: f.Mime,
				ParentID: f.ParentID, Path: a.filePath(ctx, f.ID, f.ParentID),
				Score: 1, VecScore: vecByID[id].Score, HitIn: "vector", UpdatedAt: f.UpdatedAt,
				Chunk: a.searchChunkOf(ctx, f.ID, "", vecByID[id]),
			})
			continue
		}
		f := h.File
		out = append(out, searchItem{
			ID: f.ID, Name: f.Name, Kind: f.Kind, Size: f.Size, Mime: f.Mime,
			ParentID: f.ParentID, Path: a.filePath(ctx, f.ID, f.ParentID),
			Score: h.Score, VecScore: vecByID[id].Score, HitIn: h.HitIn, UpdatedAt: f.UpdatedAt,
			Chunk: a.searchChunkOf(ctx, f.ID, q, vecByID[id]),
		})
	}
	// 稳定排序：rerank 已生效则保持精排顺序（语义相关性为最终依据）；
	// 否则命中类型优先（文件名>内容>过滤>仅向量），同类内按语义相似度降序，再按名称
	if !reranked {
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Score != out[j].Score {
				return out[i].Score > out[j].Score
			}
			if out[i].VecScore != out[j].VecScore {
				return out[i].VecScore > out[j].VecScore
			}
			return out[i].Name < out[j].Name
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out), "q": q})
}

// rerankEnabled rerank 能力是否已配置（provider/endpoint 任一就绪）。
func (a *API) rerankEnabled() bool {
	return a.cfg.GetString("ai.rerank.provider") != "" || a.cfg.GetString("ai.rerank.endpoint") != ""
}

// rerankFuse 对 RRF 融合后的候选文件按内容与 query 相关性二次精排（K18）。
// 候选文档取每文件最佳命中块内容（向量块优先，无则取首个索引块）；rerank 失败时保持原序。
// 返回 (ids, ok)：ok=true 表示精排成功，调用方应以返回顺序为最终结果（不再按命中层级重排）。
func (a *API) rerankFuse(ctx context.Context, q string, ids []string, vecByID map[string]ai.VecHit) ([]string, bool) {
	topN := a.cfg.GetInt("ai.rerank.top_n")
	if topN < 5 || topN > 100 {
		topN = 15
	}
	if len(ids) > topN {
		ids = ids[:topN]
	}
	docs := make([]string, 0, len(ids))
	for _, id := range ids {
		doc := ""
		if v, ok := vecByID[id]; ok && v.Content != "" {
			doc = v.Content
		} else {
			doc = a.firstChunkOf(ctx, id)
		}
		// 单文档限长：截断到 300 rune，避免长文拖慢 rerank 推理
		if r := []rune(doc); len(r) > 300 {
			doc = string(r[:300])
		}
		docs = append(docs, doc)
	}
	scores, err := a.ai.Rerank(ctx, q, docs)
	if err != nil || len(scores) != len(ids) {
		return ids, false // 精排失败降级：保持 RRF 原顺序
	}
	type pair struct {
		id string
		s  float32
	}
	ps := make([]pair, len(ids))
	for i, id := range ids {
		ps[i] = pair{id, scores[i]}
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].s > ps[j].s })
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.id
	}
	return out, true
}

// firstChunkOf 取文件首个索引块内容（rerank 候选文档兜底来源）。
func (a *API) firstChunkOf(ctx context.Context, fileID string) string {
	var content string
	if err := a.db.QueryRowContext(ctx,
		`SELECT content FROM index_chunks WHERE file_id=? ORDER BY chunk_idx LIMIT 1`,
		fileID).Scan(&content); err != nil {
		return ""
	}
	return content
}

// searchChunkOf 取结果文件的一个溯源块：关键词命中优先 LIKE 首块；向量命中直接用最佳块。
func (a *API) searchChunkOf(ctx context.Context, fileID, q string, v ai.VecHit) *searchChunk {
	// 关键词命中优先：锚点用查询词本身，定位到实际命中段落（比向量块的块首更准）。
	if q != "" {
		var id, content string
		like := "%" + q + "%"
		err := a.db.QueryRowContext(ctx,
			`SELECT id, content FROM index_chunks WHERE file_id=? AND content LIKE ? ORDER BY chunk_idx LIMIT 1`,
			fileID, like).Scan(&id, &content)
		if err == nil {
			return &searchChunk{ID: id, Content: clipChunk(content), Anchor: anchorOf(q)}
		}
	}
	if v.ChunkID != "" && v.Content != "" {
		return &searchChunk{ID: v.ChunkID, Content: clipChunk(v.Content), Anchor: firstParaOf(v.Content)}
	}
	return nil
}

// firstParaOf 取首个非空段落（清洗后 ≤40 rune）作定位锚点：
// 避免把标题与正文拼接成跨文本节点的长串，保证阅读页 DOM 内可子串匹配。
func firstParaOf(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if a := anchorOf(ln); a != "" {
			return a
		}
	}
	return ""
}

// anchorOf 生成定位锚点：清洗 markdown 标记后的稳定纯文本（≤40 rune）。
// 目标是在阅读页渲染后的 DOM 文本中可被子串匹配到（markdown 语法在渲染后不存在）。
func anchorOf(s string) string {
	linkRe := regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`) // 链接 [text](url) → text
	markRe := regexp.MustCompile(`!?\[\[|\]\]|\*\*|__|~~|` + "`")
	lineRe := regexp.MustCompile(`^#{1,6}\s*|^>\s*|^[-*+]\s+|^\d+[.、]\s*`)
	s = linkRe.ReplaceAllString(s, "$1")
	s = markRe.ReplaceAllString(s, "")
	var parts []string
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(lineRe.ReplaceAllString(ln, ""))
		if ln != "" {
			parts = append(parts, ln)
		}
	}
	out := strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
	r := []rune(out)
	if len(r) > 40 {
		return string(r[:40])
	}
	return out
}

// clipChunk 截断块内容为展示片段（保持 UTF-8 安全）。
func clipChunk(s string) string {
	r := []rune(s)
	if len(r) <= 140 {
		return s
	}
	return string(r[:140]) + "…"
}

// chunkGet GET /api/v1/chunks/{id}：取单块内容（阅读页溯源定位用）。
func (a *API) chunkGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "CHUNK_NO_ID", "缺少块 ID")
		return
	}
	var content string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT content FROM index_chunks WHERE id=?`, id).Scan(&content); err != nil {
		writeErr(w, http.StatusNotFound, "CHUNK_NOT_FOUND", "未找到该内容块")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "content": content, "anchor": firstParaOf(content)})
}

// fileLinks GET /api/v1/files/{id}/links：双链/反向链接（cites 出链与入链，带段落锚点 anchor）。
func (a *API) fileLinks(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "LINKS_NO_ID", "缺少文件 ID")
		return
	}
	ctx := r.Context()
	row := func(rel, col string) []map[string]string {
		rows, err := a.db.QueryContext(ctx,
			`SELECT f.id, f.name, IFNULL(e.anchor,'') FROM knowledge_edges e JOIN files f ON f.id = e.`+col+`
			 WHERE e.`+rel+` = ? AND e.relation IN ('cites','related') AND f.deleted_at IS NULL
			 ORDER BY e.weight DESC, f.name LIMIT 50`, id)
		if err != nil {
			return nil
		}
		defer rows.Close()
		out := make([]map[string]string, 0)
		for rows.Next() {
			var fid, name, anchor string
			if err := rows.Scan(&fid, &name, &anchor); err == nil {
				out = append(out, map[string]string{"id": fid, "name": name, "anchor": anchor})
			}
		}
		return out
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"file_id":  id,
		"outgoing": row("source_id", "target_id"), // 本文引用（出链）
		"incoming": row("target_id", "source_id"), // 引用本文（入链/反向链接）
	})
}

// locateChunk GET /api/v1/files/{id}/locate?q=锚点：在目标文件的索引块中定位含锚点文本的 chunk，
// 供阅读页"段落级双链"点击跳转高亮（找不到返回 404，前端回退整文件打开）。
func (a *API) locateChunk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if id == "" || q == "" {
		writeErr(w, http.StatusBadRequest, "LOCATE_BAD_PARAM", "缺少文件 ID 或锚点")
		return
	}
	ctx := r.Context()
	// 锚点可能是标题（#标题）或块ID（#^块ID）；标题按清洗后文本 LIKE 匹配，块ID按原文精确匹配
	var cid int
	var content string
	like := "%" + q + "%"
	err := a.db.QueryRowContext(ctx,
		`SELECT id, content FROM index_chunks WHERE file_id=? AND content LIKE ? ORDER BY chunk_idx LIMIT 1`,
		id, like).Scan(&cid, &content)
	if err != nil {
		writeErr(w, http.StatusNotFound, "LOCATE_NOT_FOUND", "目标文件中未找到该锚点")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"file_id": id, "chunk_id": cid, "anchor": q})
}

// filePath 拼文件完整路径（向上递归 parent 链，最多 40 层防环）。
func (a *API) filePath(ctx context.Context, id, parentID string) string {
	names := []string{}
	cur := parentID
	seen := map[string]bool{}
	for depth := 0; cur != "" && depth < 40; depth++ {
		if seen[cur] {
			break
		}
		seen[cur] = true
		var name string
		err := a.db.QueryRowContext(ctx,
			`SELECT name FROM files WHERE id=? AND deleted_at IS NULL`, cur).Scan(&name)
		if err != nil || name == "" {
			break
		}
		names = append([]string{name}, names...)
		var p string
		if err := a.db.QueryRowContext(ctx,
			`SELECT parent_id FROM files WHERE id=?`, cur).Scan(&p); err != nil {
			break
		}
		cur = p
	}
	path := "/" + strings.Join(names, "/")
	if path == "/" {
		return "/我的空间"
	}
	return path
}

// atoiSafe 安全解析非负整数（非法返回错误）。
func atoiSafe(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid number: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

