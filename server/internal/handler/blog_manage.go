// blog_manage.go 博客管理 API：博客目录在存储层隐藏后，内容管理的唯一数据源。
// GET /api/v1/blog/manage（需登录）→ {dir_id, categories:[{id,name}], posts:[{id,name,path}]}
// 分类 = 博客目录直接子目录；posts = 博客目录全部非草稿文件（含 path 相对路径）。
package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// blogManage 返回博客目录管理结构（登录态；存储层不可见的博客目录在此全权管理）。
func (a *API) blogManage(w http.ResponseWriter, r *http.Request) {
	// 多用户：仅站点管理员可管理（插件/站点配置/作者白名单）
	if !a.blogAdminOnly(w, r) {
		return
	}
	dirID := service.BlogDirID
	now := time.Now().UnixMilli()

	// 1) 分类（博客目录直接子目录）
	// 1) 分类（博客目录直接子目录；双层排序：手动 sort_order 权重在前→同权重/未设置按分类内最近文章活动时间动态排）
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT f.id, f.name, f.sort_order, COALESCE(MAX(p.updated_at), 0) AS last_activity
		 FROM files f
		 LEFT JOIN files p ON p.parent_id = f.id AND p.kind='file' AND p.deleted_at IS NULL
		 WHERE f.parent_id=? AND f.kind='dir' AND f.deleted_at IS NULL
		 GROUP BY f.id
		 ORDER BY (f.sort_order IS NULL) ASC, f.sort_order ASC, last_activity DESC, f.name COLLATE NOCASE`,
		dirID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "BLOG_MANAGE_FAILED", err.Error())
		return
	}
	cats := []map[string]any{}
	for rows.Next() {
		var id, name string
		var sortOrder any
		var lastAct int64
		if rows.Scan(&id, &name, &sortOrder, &lastAct) == nil {
			so := int64(0)
			if v, ok := sortOrder.(int64); ok {
				so = v
			}
			cats = append(cats, map[string]any{
				"id": id, "name": name, "sort_order": so,
				"last_activity": lastAct,
			})
		}
	}
	rows.Close()
	// 1b) 多级分类：为每个一级分类递归列出子目录相对路径（tech → ["ai", "ai/nlp"]），
	//     分类页过滤已在公开轨按 path 前缀匹配（BlogView），此处仅供管理端展示层级
	for _, c := range cats {
		srows, err := a.db.QueryContext(r.Context(),
			`WITH RECURSIVE sub(id, path) AS (
			   SELECT id, name FROM files WHERE parent_id=? AND kind='dir' AND deleted_at IS NULL
			   UNION ALL
			   SELECT f.id, sub.path || '/' || f.name FROM files f JOIN sub ON f.parent_id = sub.id
			   WHERE f.kind='dir' AND f.deleted_at IS NULL
			 ) SELECT path FROM sub ORDER BY path`, c["id"])
		if err == nil {
			subs := []string{}
			for srows.Next() {
				var sp string
				if srows.Scan(&sp) == nil {
					subs = append(subs, sp)
				}
			}
			srows.Close()
			c["subcats"] = subs
		}
	}

	// 2) 文章（博客目录全部非草稿文件，带相对 path + 公开稳定链接 slug）
	// 先为无 slug 的文件补齐（懒生成：首次管理/发布可见时固定，之后改名不碎链）
	_ = service.EnsureDirSlugs(r.Context(), a.db, dirID)
	files, err := a.collectDirFilesAdmin(r.Context(), dirID, "")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "BLOG_MANAGE_FAILED", err.Error())
		return
	}
	posts := []map[string]any{}
	for _, it := range files {
		posts = append(posts, map[string]any{
			"id": it.f.ID, "name": it.f.Name, "path": it.path, "slug": it.f.Slug,
			"mime": it.f.Mime, "updated_at": it.f.UpdatedAt,
			"pin_scope": it.f.PinScope, "pin_order": it.f.PinOrder,
		})
	}
	// 批量附定时发布/密码状态/阅读量（避免 N+1；publish_at 未到=定时中，has_pwd=已设置密码，view_count=累计阅读）
	if len(posts) > 0 {
		ids := make([]any, len(posts))
		for i, p := range posts {
			ids[i] = p["id"]
		}
		ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		prows, err := a.db.QueryContext(r.Context(),
			`SELECT id, publish_at, view_count, CASE WHEN COALESCE(access_pwd,'')<>'' THEN 1 ELSE 0 END,
			    COALESCE(cover,''), COALESCE(excerpt,''), COALESCE(seo_title,''), COALESCE(seo_desc,''),
			    COALESCE(content_state,'')
			 FROM files WHERE id IN (`+ph+`)`, ids...)
		if err == nil {
			for prows.Next() {
			var id string
			var pa, vc any
			var hp int
			var cover, excerpt, seoT, seoD, cs string
			if prows.Scan(&id, &pa, &vc, &hp, &cover, &excerpt, &seoT, &seoD, &cs) == nil {
				for _, p := range posts {
					if p["id"] == id {
						p["publish_at"] = pa
						p["view_count"] = vc
						p["has_pwd"] = hp > 0
						p["cover"] = cover
						p["excerpt"] = excerpt
						p["seo_title"] = seoT
						p["seo_desc"] = seoD
						p["content_state"] = cs
						break
					}
				}
			}
			}
			prows.Close()
		}
	}

	// 3) 博客对外状态：以 settings.blog.open 为准（共享管理页撤销 blog 分享已被 403 锁定；
	//    关闭/开启博客走本开关，避免"撤销=关博客"被重启复活/永久关闭的语义冲突）
	blogOpen := true
	if v, ok := a.cfg.Get("blog.open"); ok {
		if s, _ := v.(string); s == "false" {
			blogOpen = false
		}
	}
	var shareToken string
	_ = a.db.QueryRowContext(r.Context(),
		`SELECT token FROM shares WHERE owner_id=? AND token=?`,
		a.homeOwnerID(), service.BlogToken).Scan(&shareToken)

	autoPub := false
	if v, ok := a.cfg.Get("blog.auto_publish_on_upload"); ok {
		if s, _ := v.(string); s == "true" {
			autoPub = true
		}
	}
	guestCmt := false
	if v, ok := a.cfg.Get("blog.comments_guest"); ok {
		if s, _ := v.(string); s == "true" {
			guestCmt = true
		}
	}
	pendingCnt := 0
	_ = a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM comments WHERE status='pending'`).Scan(&pendingCnt)

	writeJSON(w, http.StatusOK, map[string]any{
		"dir_id":      dirID,
		"categories":  cats,
		"posts":       posts,
		"authors":     a.cfg.GetString("blog.authors"),
		"blog_status": map[string]any{"active": blogOpen, "token": shareToken, "url": "#/blog?view=public"},
		"auto_publish_on_upload": autoPub,
		"comments_guest":         guestCmt,
		"pending_comments":       pendingCnt,
		"now":         now,
	})
}


// blogCategoriesSort POST /api/v1/blog/categories/sort {"orders":[{"id":"分类id","sort_order":0|1|6...}]}
// 批量设置分类手动排序权重：sort_order=NULL 表示"未设置→动态排序"（传 null）；
// 0 为显式最小权重（排最前）；相同权重回退动态（分类内最近文章活动时间）。
func (a *API) blogCategoriesSort(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	var req struct {
		Orders []struct {
			ID        string `json:"id"`
			SortOrder *int64  `json:"sort_order"` // nil=清除手动排序（回动态）
		} `json:"orders"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	dirID := service.BlogDirID
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "CATEGORY_SORT_FAILED", err.Error())
		return
	}
	defer tx.Rollback()
	updated := 0
	for _, o := range req.Orders {
		if o.ID == "" {
			continue
		}
		var cnt int
		if err := tx.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM files WHERE id=? AND parent_id=? AND kind='dir' AND deleted_at IS NULL`,
			o.ID, dirID).Scan(&cnt); err != nil || cnt == 0 {
			writeErr(w, http.StatusNotFound, "CATEGORY_NOT_FOUND", "分类不存在或不属于博客："+o.ID)
			return
		}
		var so any
		if o.SortOrder != nil {
			so = *o.SortOrder
		}
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE files SET sort_order=? WHERE id=?`, so, o.ID); err != nil {
			writeErr(w, http.StatusInternalServerError, "CATEGORY_SORT_FAILED", err.Error())
			return
		}
		updated++
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, "CATEGORY_SORT_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "blog.categories_sort", "files",
		map[string]any{"updated": updated})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": updated})
}

// blogPostsPin POST /api/v1/blog/posts/pin {"pins":[{"id":"文章id","scope":"none|category|global","order":0|null}]}
// 文章置顶底座（2.1）：批量设置置顶范围与顺序。
//   scope=none 清除置顶；scope=category 该文仅在其所属分类列表置顶；scope=global 全站列表（含 RSS）置顶。
//   order 为同 scope 内排序权重（越小越靠前；null=未设，按更新时间）。
// 守卫用本仓 blogAdminOnly（与分类排序一致）；归属校验确保文章属于博客目录。
func (a *API) blogPostsPin(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	var req struct {
		Pins []struct {
			ID    string `json:"id"`
			Scope string `json:"scope"` // none|category|global
			Order *int64 `json:"order"` // null=未设
		} `json:"pins"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	dirID := service.BlogDirID
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "BLOG_PIN_FAILED", err.Error())
		return
	}
	defer tx.Rollback()
	updated := 0
	for _, p := range req.Pins {
		if p.ID == "" {
			continue
		}
		scope := p.Scope
		if scope != "none" && scope != "category" && scope != "global" {
			writeErr(w, http.StatusBadRequest, "BAD_SCOPE", "scope 必须为 none|category|global")
			return
		}
		// 校验：文章必须是博客目录子树内的文件（递归，兼容二级及以上子目录）
		if !a.isBlogPostFile(tx, p.ID, dirID) {
			writeErr(w, http.StatusNotFound, "POST_NOT_FOUND", "文章不存在或不属于博客："+p.ID)
			return
		}
		var po any
		if p.Order != nil {
			po = *p.Order
		}
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE files SET pin_scope=?, pin_order=?, updated_at=? WHERE id=?`,
			scope, po, time.Now().UnixMilli(), p.ID); err != nil {
			writeErr(w, http.StatusInternalServerError, "BLOG_PIN_FAILED", err.Error())
			return
		}
		updated++
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, "BLOG_PIN_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "blog.posts_pin", "files",
		map[string]any{"updated": updated})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": updated})
}

// isBlogPostFile 递归校验 id 是否为博客目录（dirID）子树内的文件（含任意层级子目录）。
// 用递归 CTE 沿 parent_id 上溯；旧 pin 校验只查一层子目录，二级及以上目录的文章会被误判，
// 本 helper 供 meta/pin 等按文章 id 写元数据的接口统一使用。
func (a *API) isBlogPostFile(q queryRower, id, dirID string) bool {
	var cnt int
	_ = q.QueryRow(`WITH RECURSIVE anc(id) AS (
		  SELECT id FROM files WHERE id=? AND deleted_at IS NULL
		  UNION ALL
		  SELECT f.parent_id FROM files f JOIN anc ON f.id=anc.id WHERE f.deleted_at IS NULL
		) SELECT COUNT(*) FROM anc WHERE id=?`, id, dirID).Scan(&cnt)
	return cnt > 0
}

// queryRower 最小查询接口（*sql.DB / *sql.Tx 皆满足），便于事务内外复用。
type queryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

// blogPostMeta POST /api/v1/blog/posts/meta {"id","cover","excerpt","seo_title","seo_desc","node_type","fields"}
// 保存单篇元数据：封面图（文件 URL/file_id 引用）、自定义摘要、单篇 SEO 标题/描述。
// 另支持 A-G 生产站底座：node_type（内容类型）+ fields（自定义业务字段），存入 content_state（与上游契约一致）。
// 空字符串 = 清除；缺省字段 = 保持不变（只更新出现的键，用指针判存）。
func (a *API) blogPostMeta(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	var req struct {
		ID       string         `json:"id"`
		Cover    *string        `json:"cover"`
		Excerpt  *string        `json:"excerpt"`
		SeoTitle *string        `json:"seo_title"`
		SeoDesc  *string        `json:"seo_desc"`
		NodeType *string        `json:"node_type"`
		Fields   map[string]any `json:"fields"`
		Growth   *string        `json:"growth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "id 必填")
		return
	}
	if !a.isBlogPostFile(a.db, req.ID, service.BlogDirID) {
		writeErr(w, http.StatusNotFound, "POST_NOT_FOUND", "文章不存在或不属于博客："+req.ID)
		return
	}
	clip := func(s string, max int) string { s = strings.TrimSpace(s); if len(s) > max { s = s[:max] }; return s }
	sets, args := []string{}, []any{}
	if req.Cover != nil {
		sets, args = append(sets, "cover=?"), append(args, clip(*req.Cover, 1024))
	}
	if req.Excerpt != nil {
		sets, args = append(sets, "excerpt=?"), append(args, clip(*req.Excerpt, 600))
	}
	if req.SeoTitle != nil {
		sets, args = append(sets, "seo_title=?"), append(args, clip(*req.SeoTitle, 200))
	}
	if req.SeoDesc != nil {
		sets, args = append(sets, "seo_desc=?"), append(args, clip(*req.SeoDesc, 400))
	}
	// A-G 内容类型 + 自定义字段：合并进 content_state（visibility/status 等既有键不受影响）
	csExpr, csArgs := "", []any{}
	if req.NodeType != nil {
		nt := clip(*req.NodeType, 64)
		csExpr = "json_set(COALESCE(content_state,'{}'), '$.node_type', ?)"
		csArgs = append(csArgs, nt)
	}
	if req.Fields != nil {
		fb, _ := json.Marshal(req.Fields)
		if csExpr == "" {
			csExpr = "json_set(COALESCE(content_state,'{}'), '$.fields', ?)"
		} else {
			csExpr = "json_set(" + csExpr + ", '$.fields', ?)"
		}
		csArgs = append(csArgs, string(fb))
	}
	// B3 生长策略：allow（允许评论收录，默认）| collect（+采集线索）| auto（全自动）| frozen（冻结）。
	// 非法值一律回落 allow（避免写入无意义状态）；frozen 时收录端点返回 INGEST_FROZEN。
	if req.Growth != nil {
		g := clip(*req.Growth, 16)
		switch g {
		case "allow", "collect", "auto", "frozen":
		default:
			g = "allow"
		}
		if csExpr == "" {
			csExpr = "json_set(COALESCE(content_state,'{}'), '$.growth', ?)"
		} else {
			csExpr = "json_set(" + csExpr + ", '$.growth', ?)"
		}
		csArgs = append(csArgs, g)
	}
	if csExpr != "" {
		sets = append(sets, "content_state="+csExpr)
		args = append(args, csArgs...)
	}
	if len(sets) == 0 {
		writeErr(w, http.StatusBadRequest, "NOTHING_TO_UPDATE", "未提供任何可更新字段")
		return
	}
	args = append(args, time.Now().UnixMilli(), req.ID)
	if _, err := a.db.Exec(`UPDATE files SET `+strings.Join(sets, ", ")+`, updated_at=? WHERE id=?`, args...); err != nil {
		writeErr(w, http.StatusInternalServerError, "BLOG_META_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "blog.posts_meta", "files",
		map[string]any{"id": req.ID, "fields": len(sets)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// blogPostsBatch POST /api/v1/blog/posts/batch
// A-G 批量编辑：对一组文章批量设置 node_type / fields / cover / excerpt / seo_title / seo_desc。
// 每篇独立执行与 blogPostMeta 相同的 json_set 合并（content_state 既有键不受影响），
// 单条失败不影响其余，返回逐条结果。
func (a *API) blogPostsBatch(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	var req struct {
		Items []struct {
			ID       string         `json:"id"`
			Cover    *string        `json:"cover"`
			Excerpt  *string        `json:"excerpt"`
			SeoTitle *string        `json:"seo_title"`
			SeoDesc  *string        `json:"seo_desc"`
			NodeType *string        `json:"node_type"`
			Fields   map[string]any `json:"fields"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Items) == 0 {
		writeErr(w, http.StatusBadRequest, "BAD_REQ", "items 必填且非空")
		return
	}
	if len(req.Items) > 200 {
		writeErr(w, http.StatusBadRequest, "TOO_MANY", "单次最多 200 篇")
		return
	}
	clip := func(s string, max int) string { s = strings.TrimSpace(s); if len(s) > max { s = s[:max] }; return s }
	type itemRes struct {
		ID    string `json:"id"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	results := make([]itemRes, 0, len(req.Items))
	okCount := 0
	for _, it := range req.Items {
		r0 := itemRes{ID: it.ID}
		if it.ID == "" {
			r0.Error = "id 为空"
			results = append(results, r0)
			continue
		}
		if !a.isBlogPostFile(a.db, it.ID, service.BlogDirID) {
			r0.Error = "文章不存在或不属于博客"
			results = append(results, r0)
			continue
		}
		sets, args := []string{}, []any{}
		if it.Cover != nil {
			sets, args = append(sets, "cover=?"), append(args, clip(*it.Cover, 1024))
		}
		if it.Excerpt != nil {
			sets, args = append(sets, "excerpt=?"), append(args, clip(*it.Excerpt, 600))
		}
		if it.SeoTitle != nil {
			sets, args = append(sets, "seo_title=?"), append(args, clip(*it.SeoTitle, 200))
		}
		if it.SeoDesc != nil {
			sets, args = append(sets, "seo_desc=?"), append(args, clip(*it.SeoDesc, 400))
		}
		csExpr, csArgs := "", []any{}
		if it.NodeType != nil {
			nt := clip(*it.NodeType, 64)
			csExpr = "json_set(COALESCE(content_state,'{}'), '$.node_type', ?)"
			csArgs = append(csArgs, nt)
		}
		if it.Fields != nil {
			fb, _ := json.Marshal(it.Fields)
			if csExpr == "" {
				csExpr = "json_set(COALESCE(content_state,'{}'), '$.fields', ?)"
			} else {
				csExpr = "json_set(" + csExpr + ", '$.fields', ?)"
			}
			csArgs = append(csArgs, string(fb))
		}
		if csExpr != "" {
			sets = append(sets, "content_state="+csExpr)
			args = append(args, csArgs...)
		}
		if len(sets) == 0 {
			r0.Error = "未提供任何可更新字段"
			results = append(results, r0)
			continue
		}
		args = append(args, time.Now().UnixMilli(), it.ID)
		if _, err := a.db.Exec(`UPDATE files SET `+strings.Join(sets, ", ")+`, updated_at=? WHERE id=?`, args...); err != nil {
			r0.Error = err.Error()
			results = append(results, r0)
			continue
		}
		r0.OK = true
		okCount++
		results = append(results, r0)
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "blog.posts_batch", "files",
		map[string]any{"count": len(req.Items), "ok": okCount})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ok_count": okCount, "results": results})
}

// blogStats GET /api/v1/blog/stats —— 统计看板聚合。
// 口径：全部数据取自既有表（files/view_count/comments/file_tags），不做逐日流水（后续如需趋势图再加 rollup 表）。
// 子树口径 = 博客目录（含任意层级子目录，递归 CTE）。
func (a *API) blogStats(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	ctx := r.Context()
	dirID := service.BlogDirID
	sub := `WITH RECURSIVE sub(id) AS (
		  SELECT id FROM files WHERE id=? AND deleted_at IS NULL
		  UNION ALL
		  SELECT f.id FROM files f JOIN sub ON f.parent_id=sub.id WHERE f.deleted_at IS NULL
		)`

	var posts, views, published int
	if err := a.db.QueryRowContext(ctx, sub+` SELECT COUNT(*), COALESCE(SUM(view_count),0),
		COALESCE(SUM(CASE WHEN COALESCE(content_state,'')='published' THEN 1 ELSE 0 END),0)
		FROM files WHERE id IN (SELECT id FROM sub) AND kind='file' AND LOWER(name) LIKE '%.md'`, dirID).
		Scan(&posts, &views, &published); err == nil {
		writeStats(ctx, a, w, dirID, sub, posts, views, published)
		return
	}
	writeErr(w, http.StatusInternalServerError, "BLOG_STATS_FAILED", "聚合查询失败")
}

// writeStats 组装看板其余部分（评论/分类/热门/标签）。
func writeStats(ctx context.Context, a *API, w http.ResponseWriter, dirID, sub string, posts, views, published int) {
	var cTotal, cPending int
	_ = a.db.QueryRowContext(ctx, sub+` SELECT COUNT(*), COALESCE(SUM(CASE WHEN c.status='pending' THEN 1 ELSE 0 END),0)
		FROM comments c JOIN files f ON f.id=c.file_id
		WHERE f.id IN (SELECT id FROM sub) AND f.kind='file'`, dirID).Scan(&cTotal, &cPending)

	// 分类：一级子目录（管理端同口径），各自递归计文章数与浏览
	type catRow struct {
		id, name string
		posts    int
		views    int
	}
	var cats []catRow
	rows, err := a.db.QueryContext(ctx, `SELECT id, name FROM files WHERE parent_id=? AND kind='dir' AND deleted_at IS NULL ORDER BY name`, dirID)
	if err == nil {
		for rows.Next() {
			var c catRow
			if rows.Scan(&c.id, &c.name) == nil {
				cats = append(cats, c)
			}
		}
		rows.Close()
	}
	catOut := make([]map[string]any, 0, len(cats))
	var catPosts int
	for _, c := range cats {
		var p, v int
		_ = a.db.QueryRowContext(ctx, sub+` SELECT COUNT(*), COALESCE(SUM(view_count),0)
			FROM files WHERE id IN (SELECT id FROM sub) AND kind='file' AND LOWER(name) LIKE '%.md'`, c.id).Scan(&p, &v)
		catOut = append(catOut, map[string]any{"id": c.id, "name": c.name, "posts": p, "views": v})
		catPosts += p
	}

	// 热门文章 top10
	type topRow struct {
		id, name, path string
		vc             int
	}
	var tops []topRow
	trows, err := a.db.QueryContext(ctx, sub+` SELECT id, name, COALESCE(slug,''), view_count
		FROM files WHERE id IN (SELECT id FROM sub) AND kind='file' AND LOWER(name) LIKE '%.md'
		ORDER BY view_count DESC, updated_at DESC LIMIT 10`, dirID)
	if err == nil {
		for trows.Next() {
			var t topRow
			if trows.Scan(&t.id, &t.name, &t.path, &t.vc) == nil {
				tops = append(tops, t)
			}
		}
		trows.Close()
	}
	topOut := make([]map[string]any, 0, len(tops))
	for _, t := range tops {
		topOut = append(topOut, map[string]any{"id": t.id, "name": t.name, "views": t.vc})
	}

	// 标签 top10（公开口径：全子树文章的 file_tags）
	tagOut := make([]map[string]any, 0, 10)
	grows, err := a.db.QueryContext(ctx, sub+` SELECT tg.name, COUNT(*) n
		FROM file_tags ft JOIN tags tg ON tg.id=ft.tag_id
		WHERE ft.file_id IN (SELECT id FROM sub) AND ft.file_id IN (SELECT id FROM files WHERE kind='file' AND deleted_at IS NULL)
		GROUP BY tg.name ORDER BY n DESC LIMIT 10`, dirID)
	if err == nil {
		for grows.Next() {
			var name string
			var n int
			if grows.Scan(&name, &n) == nil {
				tagOut = append(tagOut, map[string]any{"name": name, "count": n})
			}
		}
		grows.Close()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"totals": map[string]any{
			"posts": posts, "published": published, "drafts": posts - published, "views": views,
			"comments_total": cTotal, "comments_pending": cPending,
			"categories": len(catOut), "categorized_posts": catPosts, "uncategorized": posts - catPosts,
		},
		"top_posts":  topOut,
		"categories": catOut,
		"tags":       tagOut,
	})
}
