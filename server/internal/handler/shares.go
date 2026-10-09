// Package handler：分享/发布（shares）API。
// 底层已随实施文档前置建表（shares：token/permission/expires/revoked），
// 本文件实现最小闭环：创建分享 → 公开读取（信息/内容）→ 撤销。
// 公开端点刻意不暴露内部文件 ID，仅凭 token 访问（后续多用户/商业化可扩展限时、密码、统计）。
package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

type shareRow struct {
	ID         string `json:"id"`
	FileID     string `json:"file_id"`
	OwnerID    string `json:"owner_id"`
	Token      string `json:"token"`
	Permission string `json:"permission"`
	Scope      string `json:"scope"` // file=单文件分享 | dir=文件夹整体分享
	DirID      string `json:"dir_id,omitempty"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	RevokedAt  int64  `json:"revoked_at,omitempty"`
}

// post 博客文章条目（对外首页）
type post struct {
	Token      string         `json:"token"`
	Permission string         `json:"permission"`
	Scope      string         `json:"scope"`          // file|dir
	Path       string         `json:"path,omitempty"` // 目录分享展开时文件相对目录路径
	CreatedAt  int64          `json:"created_at"`
	Preview    string         `json:"preview"`
	File       map[string]any `json:"file"`
}

// POST /api/v1/shares 创建分享（返回 token 与公开 URL）
// 支持两种形态：
//   - 单文件分享：file_id
//   - 文件夹整体分享：dir_id（强授权——必须显式 confirm=true，否则 400 拒绝；
//     该目录及其子目录下所有非草稿文件整体公开，管理页可见、可一键收回）
func (a *API) sharesCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FileID    string `json:"file_id"`
		DirID     string `json:"dir_id"`
		Confirm   bool   `json:"confirm"`
		ExpiresAt int64  `json:"expires_at,omitempty"` // Unix 毫秒，0 = 永久
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "SHARE_BAD_BODY", err.Error())
		return
	}
	// ===== 文件夹整体分享（强授权） =====
	if in.DirID != "" {
		if !in.Confirm {
			writeErr(w, http.StatusForbidden, "SHARE_DIR_CONFIRM_REQUIRED", "目录整体分享需显式确认（confirm=true）：该目录及子目录全部文件将对公开博客可见")
			return
		}
		d, err := a.files.Get(r.Context(), in.DirID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "directory not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, "SHARE_CHECK_DIR_FAILED", err.Error())
			return
		}
		if d.Kind != "dir" {
			writeErr(w, http.StatusBadRequest, "SHARE_NOT_DIR", "dir_id 必须指向文件夹")
			return
		}
		// 越权守卫（审计#1，对齐上游 bd33d97）：创建公开分享=写操作，目录必须归属当前主空间
		if d.SpaceID != a.curHomeSpaceID(r) {
			writeErr(w, http.StatusForbidden, "SHARE_FORBIDDEN", "无权分享该目录：不属于当前空间")
			return
		}
		token, err := newShareToken()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "SHARE_TOKEN_FAILED", err.Error())
			return
		}
		now := time.Now().UnixMilli()
		if _, err := a.db.ExecContext(r.Context(),
			`INSERT INTO shares (id, file_id, dir_id, owner_id, token, permission, scope, expires_at, created_at)
			 VALUES (?, ?, ?, ?, ?, 'read', 'dir', ?, ?)`,
			newID(), in.DirID, in.DirID, a.homeOwnerID(), token, in.ExpiresAt, now); err != nil {
			writeErr(w, http.StatusInternalServerError, "SHARE_CREATE_FAILED", err.Error())
			return
		}
		files, err := a.collectDirFiles(r.Context(), in.DirID, "")
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "SHARE_DIR_SCAN_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"token":      token,
			"url":        "/p/" + token,
			"scope":      "dir",
			"file_count": len(files),
			"share": shareRow{ID: "", FileID: in.DirID, DirID: in.DirID, OwnerID: a.homeOwnerID(), Token: token, Permission: "read", Scope: "dir", ExpiresAt: in.ExpiresAt, CreatedAt: now},
		})
		return
	}
	// ===== 单文件分享 =====
	if in.FileID == "" {
		writeErr(w, http.StatusBadRequest, "SHARE_NO_FILE", "file_id or dir_id required")
		return
	}
	// 校验文件存在且非目录、未删除
	f, err := a.files.Get(r.Context(), in.FileID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "SHARE_CHECK_FILE_FAILED", err.Error())
		return
	}
	// 越权守卫（审计#1，对齐上游 bd33d97）：文件必须归属当前主空间
	if f.SpaceID != a.curHomeSpaceID(r) {
		writeErr(w, http.StatusForbidden, "SHARE_FORBIDDEN", "无权分享该文件：不属于当前空间")
		return
	}
	if f.Kind == "dir" {
		writeErr(w, http.StatusBadRequest, "SHARE_DIR_USE_DIR_ID", "分享文件夹请使用 dir_id（需强授权确认）")
		return
	}
	// 草稿不允许公开分享（防草稿内容直接暴露）
	var state string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT json_extract(content_state, '$.status') FROM files WHERE id=?`, in.FileID).Scan(&state); err == nil && state == "draft" {
		writeErr(w, http.StatusBadRequest, "SHARE_DRAFT", "草稿不能公开分享，请先发布")
		return
	}

	token, err := newShareToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SHARE_TOKEN_FAILED", err.Error())
		return
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(),
		`INSERT INTO shares (id, file_id, owner_id, token, permission, scope, expires_at, created_at)
		 VALUES (?, ?, ?, ?, 'read', 'file', ?, ?)`,
		newID(), in.FileID, a.homeOwnerID(), token, in.ExpiresAt, now)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SHARE_CREATE_FAILED", err.Error())
		return
	}
	id, _ := res.LastInsertId()
	_ = id
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"url":   "/p/" + token,
		"scope": "file",
		"share": shareRow{ID: "", FileID: in.FileID, OwnerID: a.homeOwnerID(), Token: token, Permission: "read", Scope: "file", ExpiresAt: in.ExpiresAt, CreatedAt: now},
	})
}

// GET /api/v1/shares 分享/博客文章列表（管理页：全部分享，含撤销/过期状态）
func (a *API) sharesList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT s.id, s.file_id, s.owner_id, s.token, s.permission, s.scope, COALESCE(s.dir_id,''),
		        COALESCE(s.expires_at,0), s.created_at, COALESCE(s.revoked_at,0),
		        f.name, f.kind, COALESCE(f.mime,''), f.size, f.updated_at
		 FROM shares s LEFT JOIN files f ON f.id = s.file_id
		 WHERE s.owner_id = ? ORDER BY s.created_at DESC LIMIT 200`, a.homeOwnerID())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SHARE_LIST_FAILED", err.Error())
		return
	}
	defer rows.Close()
	type item struct {
		Token      string `json:"token"`
		Permission string `json:"permission"`
		Scope      string `json:"scope"`
		ExpiresAt  int64  `json:"expires_at,omitempty"`
		CreatedAt  int64  `json:"created_at"`
		RevokedAt  int64  `json:"revoked_at,omitempty"`
		Status     string `json:"status"` // active|revoked|expired|missing
		File       any    `json:"file,omitempty"`
	}
	out := []item{}
	now := time.Now().UnixMilli()
	for rows.Next() {
		var (
			id, fileID, ownerID, token, perm, scope, dirID string
			expiresAt, createdAt, revokedAt               int64
			name, kind, mime                              string
			size, updatedAt                               int64
			filePresent                                   bool = true
		)
		if err := rows.Scan(&id, &fileID, &ownerID, &token, &perm, &scope, &dirID, &expiresAt, &createdAt, &revokedAt, &name, &kind, &mime, &size, &updatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "SHARE_LIST_FAILED", err.Error())
			return
		}
		st := "active"
		if revokedAt > 0 {
			st = "revoked"
		} else if expiresAt > 0 && expiresAt < now {
			st = "expired"
		} else if name == "" && fileID != "" {
			st = "missing"
		}
		if name == "" && fileID != "" {
			filePresent = false
		}
		var f any
		if filePresent {
			f = map[string]any{
				"id": fileID, "name": name, "kind": kind, "mime": mime,
				"size": size, "updated_at": updatedAt,
			}
		}
		out = append(out, item{Token: token, Permission: perm, Scope: scope, ExpiresAt: expiresAt, CreatedAt: createdAt, RevokedAt: revokedAt, Status: st, File: f})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// GET /api/v1/public/posts 对外博客首页：已发布文章列表（含摘要，token 访问不暴露内部文件 ID）
// 数据源 = 单文件分享（scope='file'）+ 文件夹整体分享展开（scope='dir'：目录及子目录全部非草稿文件）。
// clipPreview 摘要/正文前 120 个字符为列表预览（空白折叠、超长省略号）。
func clipPreview(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	t := strings.Join(strings.Fields(s), " ")
	r := []rune(t)
	if len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return t
}

// readContentPreview 读文件内容前 280 字节作为预览（摘要缺失时的回退）。
func (a *API) readContentPreview(ctx context.Context, fileID, mime, name string) string {
	if !isTextish(mime, name) {
		return ""
	}
	rc, _, err := a.files.Content(ctx, fileID)
	if err != nil {
		return ""
	}
	defer rc.Close()
	buf := make([]byte, 280)
	n, _ := io.ReadFull(rc, buf)
	if n <= 0 {
		return ""
	}
	return clipPreview(string(buf[:n]))
}

// summaryPreview 优先读 AI 摘要缓存（file_ai_summaries，status=done）作预览，无则空串。
func (a *API) summaryPreview(ctx context.Context, fileID string) string {
	var s string
	if err := a.db.QueryRowContext(ctx,
		`SELECT summary FROM file_ai_summaries WHERE file_id=? AND status='done' ORDER BY updated_at DESC LIMIT 1`,
		fileID).Scan(&s); err != nil || strings.TrimSpace(s) == "" {
		return ""
	}
	return clipPreview(s)
}

// postPreview 博客列表预览：优先 AI 摘要缓存，回退读内容前 280B（P1-6：避免逐文件读全文）。
func (a *API) postPreview(ctx context.Context, fileID, mime, name string) string {
	// M8 修复：付费文章（access_mode=paid）公开列表只出 paid_preview——绝不回退读正文。
	// 否则"列表预览"会绕过付费闸门，把正文开头（默认前 280B）白送给未付费访客。
	// paid_preview 为空（'none'，即站长选择不预览）时返回空串，连摘要缓存也不出。
	// M8b 补强（本地增量，非上游）：密码保护文章同理 —— 见下方分支。
	var mode, pp, pwd, excerpt string
	if err := a.db.QueryRowContext(ctx,
		`SELECT COALESCE(access_mode,'none'), COALESCE(paid_preview,''),
		        COALESCE(access_pwd,''), COALESCE(excerpt,'')
		   FROM files WHERE id=?`, fileID).
		Scan(&mode, &pp, &pwd, &excerpt); err == nil {
		if mode == "paid" {
			if strings.TrimSpace(pp) != "" {
				return clipPreview(pp)
			}
			return ""
		}
		// M8b：密码保护文章此前未被 M8 覆盖 —— 其 preview 会回退到 readContentPreview
		// （读正文前 280B），于是「受保护正文的开头」随公开列表、SSR 列表卡与 SSR
		// description 一起公开，密码墙只挡住了后半篇。与付费分支同口径收口：
		// 只认作者显式填写的摘要（files.excerpt），未填则不出预览。
		if strings.TrimSpace(pwd) != "" {
			return clipPreview(excerpt)
		}
	}
	if pv := a.summaryPreview(ctx, fileID); pv != "" {
		return pv
	}
	return a.readContentPreview(ctx, fileID, mime, name)
}

func (a *API) publicPosts(w http.ResponseWriter, r *http.Request) {
	// 博客对外开关（blog.open=false → 公开页下线，数据保留）
	if v, ok := a.cfg.Get("blog.open"); ok {
		if s, _ := v.(string); s == "false" {
			writeErr(w, http.StatusNotFound, "BLOG_CLOSED", "博客已关闭（博客管理页可重新开启）")
			return
		}
	}
	// 博客源优先（"目录即博客"）：存在未撤销的固定博客分享（token=blog）时，
	// /blog 只展示博客目录内容；其余分享（单文件/其他目录）仍走 /p/:token 访问。
	var blogDirID string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(dir_id,'') FROM shares
		 WHERE owner_id=? AND token=? AND (revoked_at IS NULL OR revoked_at=0)
		   AND (expires_at IS NULL OR expires_at=0 OR expires_at>?)`,
		a.homeOwnerID(), service.BlogToken, time.Now().UnixMilli()).Scan(&blogDirID)
	if err == nil && blogDirID != "" {
		files, err := a.collectBlogArticles(r.Context(), blogDirID, currentSiteID(r))
		if err == nil {
			out := []post{}
			for _, it := range files {
				out = append(out, dirPost(service.BlogToken, "read", 0, it))
			}
						// 分类排序映射：博客子目录 sort_order → 按 path 首段附加（读者端分类组排序；-1=未设置走动态）
			catSort := map[string]int64{}
			if crows, cerr := a.db.QueryContext(r.Context(),
				`SELECT name, COALESCE(sort_order,-1) FROM files WHERE parent_id=? AND kind='dir' AND deleted_at IS NULL`,
				blogDirID); cerr == nil {
				for crows.Next() {
					var cn string
					var so int64
					if crows.Scan(&cn, &so) == nil {
						catSort[cn] = so
					}
				}
				crows.Close()
			}
			for k := range out {
				cat := ""
				if idx := strings.Index(out[k].Path, "/"); idx > 0 {
					cat = out[k].Path[:idx]
				}
				if so, ok := catSort[cat]; ok {
					out[k].File["cat_sort_order"] = so
				}
			}
sortPostsByUpdatedAt(out)
			writeJSON(w, http.StatusOK, map[string]any{"items": out})
			return
		}
		// 目录异常（被删等）：回退全量分享
	}
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT s.token, s.permission, s.created_at, s.scope, COALESCE(s.dir_id,''),
		        f.id, f.name, f.kind, COALESCE(f.mime,''), f.size, f.updated_at, COALESCE(f.content_state,''),
		        COALESCE(f.publish_at,0)
		 FROM shares s JOIN files f ON f.id = s.file_id
		 WHERE s.owner_id = ? AND (s.revoked_at IS NULL OR s.revoked_at = 0)
		   AND (s.expires_at IS NULL OR s.expires_at = 0 OR s.expires_at > ?)
		   AND f.site_id = ?
		 ORDER BY s.created_at DESC LIMIT 100`, a.homeOwnerID(), time.Now().UnixMilli(), currentSiteID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "PUBLIC_POSTS_FAILED", err.Error())
		return
	}
	defer rows.Close()
	out := []post{}
	nowMs := time.Now().UnixMilli()
	for rows.Next() {
		var token, perm, scope, dirID, fileID, name, kind, mime, cs string
		var createdAt, size, updatedAt, publishAt int64
		if err := rows.Scan(&token, &perm, &createdAt, &scope, &dirID, &fileID, &name, &kind, &mime, &size, &updatedAt, &cs, &publishAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "PUBLIC_POSTS_FAILED", err.Error())
			return
		}
		// M7 修复：单文件分享回退分支此前不滤草稿/定时未到，改回草稿的文章仍进公开列表。
		// 口径与目录分支（collectDirFilesOpt）对齐：草稿不公开 + publish_at 未到不公开。
		if cs != "" {
			var st struct {
				Status string `json:"status"`
			}
			if json.Unmarshal([]byte(cs), &st) == nil && st.Status == "draft" {
				continue
			}
		}
		if publishAt > 0 && publishAt > nowMs {
			continue
		}
		if scope == "dir" {
			// 文件夹整体分享：展开目录及子目录全部非草稿文件为文章
			files, err := a.collectDirFiles(r.Context(), dirID, currentSiteID(r))
			if err != nil {
				continue
			}
			for _, it := range files {
				out = append(out, dirPost(token, perm, createdAt, it))
			}
			continue
		}
		pv := a.postPreview(r.Context(), fileID, mime, name)
		var slug string
		_ = a.db.QueryRowContext(r.Context(), `SELECT COALESCE(slug,'') FROM files WHERE id=?`, fileID).Scan(&slug)
		out = append(out, post{
			Token: token, Permission: perm, Scope: "file", CreatedAt: createdAt, Preview: pv,
			File: map[string]any{"id": fileID, "name": name, "kind": kind, "mime": mime, "size": size, "updated_at": updatedAt, "slug": slug, "author": a.postAuthorName(r.Context(), fileID), "tags": stringList(a.postTagNames(r.Context(), fileID)), "content_state": cs},
		})
	}
	sortPostsByUpdatedAt(out)
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// GET /api/v1/public/tags 公开标签聚合：[{name, count}]（文章数倒序、同数按名称字典序）。
// 可见性口径与 publicPosts 一致：只统计「博客目录分享」内非草稿文章的标签；
// 博客未开启或未设博客目录分享时返回空数组（前端标签云据此自动隐藏，不产生死链）。
func (a *API) publicTags(w http.ResponseWriter, r *http.Request) {
	if v, ok := a.cfg.Get("blog.open"); ok {
		if s, _ := v.(string); s == "false" {
			writeErr(w, http.StatusNotFound, "BLOG_CLOSED", "博客已关闭（博客管理页可重新开启）")
			return
		}
	}
	var blogDirID string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(dir_id,'') FROM shares
		 WHERE owner_id=? AND token=? AND (revoked_at IS NULL OR revoked_at=0)
		   AND (expires_at IS NULL OR expires_at=0 OR expires_at>?)`,
		a.homeOwnerID(), service.BlogToken, time.Now().UnixMilli()).Scan(&blogDirID)
	if err != nil || blogDirID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	files, err := a.collectBlogArticles(r.Context(), blogDirID, currentSiteID(r))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	count := map[string]int{}
	for _, it := range files {
		for _, t := range it.tags {
			count[t]++
		}
	}
	out := make([]map[string]any, 0, len(count))
	for name, c := range count {
		out = append(out, map[string]any{"name": name, "count": c})
	}
	sort.Slice(out, func(i, j int) bool {
		ci, cj := out[i]["count"].(int), out[j]["count"].(int)
		if ci != cj {
			return ci > cj
		}
		return out[i]["name"].(string) < out[j]["name"].(string)
	})
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// postAuthorName 返回文章作者展示名（author_id 空或查不到时回落 owner 展示名）。
func (a *API) postAuthorName(ctx context.Context, fileID string) string {
	var aid, name string
	if err := a.db.QueryRowContext(ctx, `SELECT COALESCE(author_id,'') FROM files WHERE id=?`, fileID).Scan(&aid); err != nil || aid == "" {
		_ = a.db.QueryRowContext(ctx, `SELECT COALESCE(display_name,'') FROM users WHERE id=?`, a.homeOwnerID()).Scan(&name)
		return name
	}
	if err := a.db.QueryRowContext(ctx, `SELECT COALESCE(display_name,'') FROM users WHERE id=?`, aid).Scan(&name); err != nil {
		return ""
	}
	return name
}

// dirPost 目录分享展开条目：token 为容器 token，path 为文件相对目录路径。
// 作者与标签由 collectDirFiles 批量补齐（见 dirFile.author / dirFile.tags），无 N+1 查询。
func dirPost(token, perm string, createdAt int64, it dirFile) post {
	return post{
		Token: token, Permission: perm, Scope: "dir", Path: it.path, CreatedAt: createdAt, Preview: it.preview,
		File: map[string]any{"id": it.f.ID, "name": it.f.Name, "kind": it.f.Kind, "mime": it.f.Mime, "size": it.f.Size, "updated_at": it.f.UpdatedAt, "slug": it.f.Slug, "author": it.author, "tags": stringList(it.tags), "pin_scope": it.f.PinScope, "pin_order": it.f.PinOrder, "cover": it.cover, "excerpt": it.excerpt, "content_state": it.f.ContentState},
	}
}

// sortPublicDirFiles 公开博客列表统一排序（全局置顶 → 分类置顶 → 更新时间倒序）。
// 供 RSS/SSR 同源；读者端 SPA 在 PublicHome 内按 pin 二次排序（rank 模型更严谨：
// 全局置顶在分类页也置顶）。与 sortPostsByUpdatedAt（仅时间）互斥，置顶场景用本函数。
func sortPublicDirFiles(files []dirFile) {
	sort.SliceStable(files, func(i, j int) bool {
		pi, pj := files[i].f.PinScope == "global", files[j].f.PinScope == "global"
		if pi != pj {
			return pi
		}
		if pi && pj {
			oi, oj := files[i].f.PinOrder, files[j].f.PinOrder
			if oi != nil && oj != nil && *oi != *oj {
				return *oi < *oj
			}
			if (oi == nil) != (oj == nil) {
				return oj == nil // 有 pin_order 的在前，未设的按时间
			}
		}
		return files[i].f.UpdatedAt > files[j].f.UpdatedAt
	})
}

// normMillis 将可能为「秒」的时间戳规范为毫秒（兼容历史数据中混用的单位）。
// 现代时间戳（>= 1e12）原样返回；0/负值返回 0。博客读取端统一按毫秒解析，
// 历史秒级值（files.updated_at/created_at 曾由 file.go 以秒写入）若不归一会得到 1970 与错序。
func normMillis(ms int64) int64 {
	if ms <= 0 {
		return 0
	}
	if ms < 1e12 {
		return ms * 1000
	}
	return ms
}

// postUpdatedAt 取文章更新时间戳（毫秒，已归一），用于排序与日期输出。
func postUpdatedAt(p post) int64 {
	if p.File == nil {
		return 0
	}
	switch v := p.File["updated_at"].(type) {
	case int64:
		return normMillis(v)
	case float64:
		return normMillis(int64(v))
	default:
		return 0
	}
}

// sortPostsByUpdatedAt 按更新时间倒序稳定排序（统一博客列表顺序，消除跨主题排序不一致）。
func sortPostsByUpdatedAt(out []post) {
	sort.SliceStable(out, func(i, j int) bool {
		return postUpdatedAt(out[i]) > postUpdatedAt(out[j])
	})
}

// dirFile 目录分享展开的文件项
type dirFile struct {
	f        *service.File
	path     string   // 相对分享目录的路径（含子目录名）
	preview  string
	authorID string   // files.author_id（空 = 站点 owner）
	author   string   // 作者展示名（批量补齐）
	tags     []string // 标签名（批量补齐）
	cover    string   // 封面图（files.cover，公开卡片用）
	excerpt  string   // 自定义摘要（files.excerpt；空则回退 preview）
	seoTitle string   // 单篇 SEO 标题（files.seo_title；SSR head 用，不进公开 JSON）
	seoDesc  string   // 单篇 SEO 描述（files.seo_desc；SSR head 用，不进公开 JSON）
}

// stringList 保证 JSON 序列化为数组（nil → []），避免前端拿到 null。
func stringList(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// postTagNames 返回文件已关联的标签名（字典序，最多 32 个）。单篇场景用；
// 目录批量场景由 collectDirFiles 的 IN 查询一次取回，不走此函数。
func (a *API) postTagNames(ctx context.Context, fileID string) []string {
	rows, err := a.db.QueryContext(ctx,
		`SELECT t.name FROM file_tags ft JOIN tags t ON t.id = ft.tag_id
		 WHERE ft.file_id = ? ORDER BY t.name COLLATE NOCASE LIMIT 32`, fileID)
	if err != nil {
		return []string{}
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil && n != "" {
			out = append(out, n)
		}
	}
	return out
}

// fillDirFileAuthors 批量补全目录展开条目的作者展示名（author_id 空 → 回落站点 owner 展示名）。
func (a *API) fillDirFileAuthors(ctx context.Context, items []dirFile) {
	need := map[string]bool{}
	for i := range items {
		if items[i].authorID != "" {
			need[items[i].authorID] = true
		}
	}
	nameByID := map[string]string{}
	if len(need) > 0 {
		ids := make([]any, 0, len(need))
		for id := range need {
			ids = append(ids, id)
		}
		ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		rows, err := a.db.QueryContext(ctx, `SELECT id, COALESCE(display_name,'') FROM users WHERE id IN (`+ph+`)`, ids...)
		if err == nil {
			for rows.Next() {
				var id, name string
				if rows.Scan(&id, &name) == nil {
					nameByID[id] = name
				}
			}
			rows.Close()
		}
	}
	var ownerName string
	_ = a.db.QueryRowContext(ctx, `SELECT COALESCE(display_name,'') FROM users WHERE id=?`, a.homeOwnerID()).Scan(&ownerName)
	for i := range items {
		if n := nameByID[items[i].authorID]; n != "" {
			items[i].author = n
		} else {
			items[i].author = ownerName
		}
	}
}

// fillDirFileTags 批量补全目录展开条目的标签名（一次 IN 查询，按名称字典序）。
func (a *API) fillDirFileTags(ctx context.Context, items []dirFile) {
	if len(items) == 0 {
		return
	}
	idx := make(map[string][]int, len(items))
	ids := make([]any, 0, len(items))
	for i := range items {
		idx[items[i].f.ID] = append(idx[items[i].f.ID], i)
		ids = append(ids, items[i].f.ID)
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := a.db.QueryContext(ctx,
		`SELECT ft.file_id, t.name FROM file_tags ft JOIN tags t ON t.id = ft.tag_id
		 WHERE ft.file_id IN (`+ph+`) ORDER BY t.name COLLATE NOCASE`, ids...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var fid, name string
		if rows.Scan(&fid, &name) != nil || name == "" {
			continue
		}
		for _, i := range idx[fid] {
			items[i].tags = append(items[i].tags, name)
		}
	}
}

// collectDirFiles 递归收集目录（及子目录）下全部非草稿文件，返回相对路径 + 文本摘要。
// 公开轨：定时未到（publish_at>now）的文章不公开。分层 BFS 查询（按 parent_id 逐层拉取；
// modernc 驱动对递归 CTE 支持不稳，不用 CTE）。
// collectBlogArticles 博客公开轨收集（B10）：在公开轨既有过滤（草稿 / 定时未到 / 站点隔离）之上
// 追加内容类型过滤——只返回文章，附件不进列表。供公开列表、热门榜、RSS、sitemap 使用。
//
// 为什么不直接在 collectDirFiles 里过滤：分享任意目录（含博客目录整体分享的文件夹视图）时，
// 附件本身就是被分享的内容，不该被静默隐藏。是否"只列文章"因此由调用点显式决定。
func (a *API) collectBlogArticles(ctx context.Context, dirID, siteID string) ([]dirFile, error) {
	return a.collectDirFilesOpt(ctx, dirID, siteID, false, true)
}

// nodeTypeOf 从 content_state（JSON 对象）取 node_type；无该键或 JSON 非法时返回空串。
func nodeTypeOf(contentState string) string {
	s := strings.TrimSpace(contentState)
	if s == "" || s[0] != '{' {
		return ""
	}
	var st struct {
		NodeType string `json:"node_type"`
	}
	if json.Unmarshal([]byte(s), &st) != nil {
		return ""
	}
	return st.NodeType
}

// articleVisibleInPublic 判定一个文件是否应作为「文章」出现在博客公开轨（B10）。
//
// 背景：博客目录同时承载文章与附件（图片/音视频/压缩包）。此前公开轨只过滤草稿与定时未到，
// 于是 assets/*.png 这类上传件也进公开列表、热门榜、RSS 与 sitemap —— 对读者是噪声、
// 对搜索引擎是低质 URL（附件既无正文也不该被收录）。
//
// 判定顺序（显式声明优先，mime 推断兜底）：
//  1. content_state.node_type 显式声明为文章型（post/article/page）→ 是文章，不再看 mime；
//     显式声明为附件型（attachment/asset/media/image）→ 非文章，不再看 mime；
//  2. 未声明（或 JSON 非法）：按 mime 推断，复用收录侧 isTextContent ——
//     「能当正文读的才算文章」，与评论收录机制同一判据，避免两套语义漂移；
//  3. 站长开启 blog.list_attachments=true 时一律放行（恢复旧行为）。
func articleVisibleInPublic(mime, contentState string, listAttachments bool) bool {
	if listAttachments {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(nodeTypeOf(contentState))) {
	case "post", "article", "page":
		return true
	case "attachment", "asset", "media", "image":
		return false
	}
	return isTextContent(mime)
}

func (a *API) collectDirFiles(ctx context.Context, dirID, siteID string) ([]dirFile, error) {
	return a.collectDirFilesOpt(ctx, dirID, siteID, false, false)
}

// collectDirFilesAdmin 管理轨收集：与公开轨同源，但保留"定时中"文章
// （站长在管理列表可见并可取消定时，否则定时文章会从管理界面消失）。
// 管理/分享查看场景传 siteID="" 表示不按站点过滤（站长后台可见全站共享内容）。
func (a *API) collectDirFilesAdmin(ctx context.Context, dirID, siteID string) ([]dirFile, error) {
	return a.collectDirFilesOpt(ctx, dirID, siteID, true, false)
}

// collectDirFilesOpt 收集实现；includeScheduled=false 时过滤定时未到文章。
// siteID="" 表示不按站点过滤（管理/分享查看）；非空则仅返回该站点内容（公开轨内容隔离）。
// articlesOnly=true（B10）时追加内容类型过滤：只保留文章，附件不进结果（见 collectBlogArticles）。
func (a *API) collectDirFilesOpt(ctx context.Context, dirID, siteID string, includeScheduled, articlesOnly bool) ([]dirFile, error) {
	// 校验目录存在
	var cnt int
	if err := a.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM files WHERE id=? AND kind='dir' AND deleted_at IS NULL`, dirID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, service.ErrNotFound
	}
	type rowT struct {
		id, parent, name, kind, mime, state, slug string
		authorID                                  string
		size, updatedAt                           int64
		pinScope                                  string
		pinOrder, publishAt                       any
		cover, excerpt, seoT, seoD                string
	}
	byParent := map[string][]rowT{}
	level := []string{dirID}
	for len(level) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(level)), ",")
		args := make([]any, 0, len(level)+1)
		for _, p := range level {
			args = append(args, p)
		}
		// 内容隔离：非空 siteID 时仅返回该站点文件（M1 公开轨按解析站点过滤）。
		siteClause := ""
		if siteID != "" {
			siteClause = " AND site_id=?"
			args = append(args, siteID)
		}
		q := `SELECT id, parent_id, name, kind, COALESCE(mime,''), size, updated_at, COALESCE(content_state,''), COALESCE(slug,''), COALESCE(author_id,''), pin_scope, pin_order, publish_at,
		      COALESCE(cover,''), COALESCE(excerpt,''), COALESCE(seo_title,''), COALESCE(seo_desc,'')
		      FROM files WHERE parent_id IN (` + ph + `)` + siteClause + ` AND deleted_at IS NULL`
		rows, err := a.db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		var next []string
		for rows.Next() {
			var r rowT
			if err := rows.Scan(&r.id, &r.parent, &r.name, &r.kind, &r.mime, &r.size, &r.updatedAt, &r.state, &r.slug, &r.authorID, &r.pinScope, &r.pinOrder, &r.publishAt, &r.cover, &r.excerpt, &r.seoT, &r.seoD); err != nil {
				rows.Close()
				return nil, err
			}
			byParent[r.parent] = append(byParent[r.parent], r)
			next = append(next, r.id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		level = next
	}
	out := []dirFile{}
	var walk func(id, prefix string)
	walk = func(id, prefix string) {
		for _, r := range byParent[id] {
			p := r.name
			if prefix != "" {
				p = prefix + "/" + r.name
			}
			if r.kind == "dir" {
				walk(r.id, p)
				continue
			}
			if r.state != "" {
				var st struct {
					Status string `json:"status"`
				}
				if json.Unmarshal([]byte(r.state), &st) == nil && st.Status == "draft" {
					continue // 草稿不随目录公开
				}
			}
			// B10 内容类型过滤（仅公开轨、仅 collectBlogArticles 场景）：附件不进公开列表，
			// 避免 assets/*.png 这类上传件混进公开列表/热门榜/RSS/sitemap。
			// 显式声明优先：node_type 为 post/article/page 的文件始终视为文章。
			if articlesOnly && !articleVisibleInPublic(r.mime, r.state, a.cfg.GetBool("blog.list_attachments")) {
				continue
			}
			// 2.3 定时发布：publish_at 未到不公开（等同未发布；到点由常驻调度置 NULL）。
			// 管理轨（includeScheduled）不过滤，定时中文章在管理列表可见。
			if !includeScheduled {
				if pa, ok := r.publishAt.(int64); ok && pa > 0 && pa > time.Now().UnixMilli() {
					continue
				}
			}
			var po *int64
			if v, ok := r.pinOrder.(int64); ok {
				po = &v
			}
			f := &service.File{ID: r.id, Name: r.name, Kind: r.kind, Mime: r.mime, Size: r.size, UpdatedAt: r.updatedAt, ContentState: r.state, Slug: r.slug, PinScope: r.pinScope, PinOrder: po}
			out = append(out, dirFile{f: f, path: p, authorID: r.authorID, cover: r.cover, excerpt: r.excerpt, seoTitle: r.seoT, seoDesc: r.seoD})
		}
	}
	walk(dirID, "")
	// 批量补作者展示名 + 标签（各一次 IN 查询，避免逐篇 2 次查询的 N+1）
	a.fillDirFileAuthors(ctx, out)
	a.fillDirFileTags(ctx, out)
	// 批量读摘要缓存作预览（一次 IN 查询，避免逐文件读内容；P1-6）
	ids := make([]any, len(out))
	for i, df := range out {
		ids[i] = df.f.ID
	}
	if len(ids) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		srows, err := a.db.QueryContext(ctx,
			`SELECT file_id, summary FROM file_ai_summaries WHERE status='done' AND file_id IN (`+ph+`)`, ids...)
		if err == nil {
			byFile := map[string]string{}
			for srows.Next() {
				var fid, sum string
				if srows.Scan(&fid, &sum) == nil {
					byFile[fid] = sum
				}
			}
			srows.Close()
			for i := range out {
				if s, ok := byFile[out[i].f.ID]; ok && strings.TrimSpace(s) != "" {
					out[i].preview = clipPreview(s)
				} else {
					out[i].preview = a.readContentPreview(ctx, out[i].f.ID, out[i].f.Mime, out[i].f.Name)
				}
			}
		}
		// M8 延伸修复：付费文章一律只出 paid_preview，绝不回退读正文。
		// 本处是三条下游轨的共用汇聚点——SSR 列表卡、SSR 文章页 desc、公开 JSON 目录分支，
		// 在此收口可一次覆盖（付费文章若走 summaryPreview/readContentPreview，正文开头会
		// 出现在列表卡与页面的 description 里，付费墙就白设了）。
		// M8b 补强（本地增量，非上游）：密码保护文章同属"锁定态"，此前不在本收口范围内，
		// 其 preview 仍回退读正文前 280B → 受保护正文开头公开。改为只认作者填写的 excerpt。
		if prows, perr := a.db.QueryContext(ctx,
			`SELECT id, COALESCE(access_mode,'none'), COALESCE(paid_preview,''),
			        COALESCE(access_pwd,''), COALESCE(excerpt,'')
			   FROM files WHERE id IN (`+ph+`)`, ids...); perr == nil {
			type lockPrev struct{ mode, paidPrev, pwd, excerpt string }
			lockPrevBy := map[string]lockPrev{}
			for prows.Next() {
				var fid string
				var lp lockPrev
				if prows.Scan(&fid, &lp.mode, &lp.paidPrev, &lp.pwd, &lp.excerpt) == nil {
					lockPrevBy[fid] = lp
				}
			}
			prows.Close()
			for i := range out {
				lp, ok := lockPrevBy[out[i].f.ID]
				if !ok {
					continue
				}
				switch {
				case lp.mode == "paid":
					if strings.TrimSpace(lp.paidPrev) == "" {
						out[i].preview = "" // paid_preview=''（none）→ 不出预览
					} else {
						out[i].preview = clipPreview(lp.paidPrev)
					}
				case strings.TrimSpace(lp.pwd) != "":
					// 密码保护：只出作者显式填写的摘要，无则不出（不读正文）。
					out[i].preview = clipPreview(lp.excerpt)
				}
			}
		}
	}
	return out, nil
}

// isDraftFile 判断文件是否为草稿（content_state.status=draft）
func isDraftFile(f *service.File) bool {
	var st struct {
		Status string `json:"status"`
	}
	if f.ContentState == "" {
		return false
	}
	if err := json.Unmarshal([]byte(f.ContentState), &st); err != nil {
		return false
	}
	return st.Status == "draft"
}

// isTextish 判断文件是否可生成文本摘要（MIME 或扩展名）
func isTextish(mime, name string) bool {
	if strings.HasPrefix(mime, "text/") || strings.Contains(mime, "json") || strings.Contains(mime, "markdown") ||
		strings.Contains(mime, "yaml") || strings.Contains(mime, "xml") || strings.Contains(mime, "javascript") ||
		strings.Contains(mime, "html") || strings.Contains(mime, "css") {
		return true
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown", ".txt", ".log", ".json", ".html", ".xml", ".yaml", ".yml", ".csv", ".go", ".py", ".js", ".ts", ".vue", ".sql", ".sh":
		return true
	}
	return false
}

// GET /api/v1/shares/{token} 公开读取分享信息（含文件元数据）
// scope='dir' 时同时返回目录文件列表（公开目录页数据，不含内部 file id）
func (a *API) sharesGet(w http.ResponseWriter, r *http.Request) {
	row, err := a.loadShare(r.Context(), r.PathValue("token"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "SHARE_NOT_FOUND", "share not found or revoked")
		return
	}
	if row.Scope == "dir" {
		files, err := a.collectDirFiles(r.Context(), row.DirID, "")
		if err != nil {
			writeErr(w, http.StatusNotFound, "SHARE_FILE_GONE", "shared directory not found")
			return
		}
		d, _ := a.files.Get(r.Context(), row.DirID)
		fileList := []map[string]any{}
		for _, it := range files {
			fileList = append(fileList, map[string]any{
				"path": it.path, "name": it.f.Name, "kind": it.f.Kind, "mime": it.f.Mime,
				"size": it.f.Size, "updated_at": it.f.UpdatedAt, "preview": it.preview, "slug": it.f.Slug,
				"author": it.author, // 作者展示名（collectDirFiles 已批量补齐；文章页标题下元信息行用）
			})
		}
		var dirMeta any
		if d != nil {
			dirMeta = map[string]any{"id": d.ID, "name": d.Name, "kind": d.Kind}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"share": row,
			"dir":   dirMeta,
			"files": fileList,
		})
		return
	}
	f, err := a.files.Get(r.Context(), row.FileID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "SHARE_FILE_GONE", "shared file not found")
		return
	}
	// 单文件分享也带作者展示名（前端文章页元信息行 page.post.author 读取）
	fileView := struct {
		*service.File
		Author string `json:"author"`
	}{File: f, Author: a.postAuthorName(r.Context(), row.FileID)}
	writeJSON(w, http.StatusOK, map[string]any{
		"share": row,
		"file":  fileView,
	})
}

// GET /api/v1/shares/{token}/content 公开读取内容（文本/图片/音视频/PDF 统一流式）
// scope='dir' 时需 ?path= 指定目录内文件相对路径
func (a *API) sharesContent(w http.ResponseWriter, r *http.Request) {
	row, err := a.loadShare(r.Context(), r.PathValue("token"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "SHARE_NOT_FOUND", "share not found or revoked")
		return
	}
	var fileID string
	if row.Scope == "dir" {
		name := r.URL.Query().Get("path")
		if name == "" {
			writeErr(w, http.StatusBadRequest, "SHARE_PATH_REQUIRED", "directory share requires ?path=")
			return
		}
		files, err := a.collectDirFiles(r.Context(), row.DirID, "")
		if err != nil {
			writeErr(w, http.StatusNotFound, "SHARE_FILE_GONE", "shared directory not found")
			return
		}
		found := false
		for _, it := range files {
			if it.path == name {
				fileID = it.f.ID
				found = true
				break
			}
		}
		if !found {
			writeErr(w, http.StatusNotFound, "SHARE_PATH_NOT_FOUND", "file not found in shared directory")
			return
		}
	} else {
		fileID = row.FileID
	}
	// 2.2 文章密码保护：文件级闸门（未解锁 → 401 PASSWORD_REQUIRED，前端出密码框）
	if err := a.requireArticleUnlock(w, r, fileID); err != nil {
		if errors.Is(err, errPasswordRequired) {
			return
		}
		writeErr(w, http.StatusInternalServerError, "SHARE_CONTENT_FAILED", err.Error())
		return
	}
	// B 项内容付费闸门：密码通过后再查付费（access_mode='paid' 且无有效 grant → 402 PAYMENT_REQUIRED）
	if err := a.requirePaidAccess(w, r, fileID); err != nil {
		if errors.Is(err, errPaidRequired) {
			return
		}
		writeErr(w, http.StatusInternalServerError, "SHARE_CONTENT_FAILED", err.Error())
		return
	}
	rc, f, err := a.files.Content(r.Context(), fileID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "SHARE_FILE_GONE", "shared file not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "SHARE_CONTENT_FAILED", err.Error())
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", contentType(f.Mime, f.Name))
	w.Header().Set("Content-Disposition", "inline")
	// 匿名公开页：html/svg/xml 一律纯文本 + 禁嗅探，防公开分享页 XSS（外部访客可达）
	if strings.EqualFold(path.Ext(f.Name), ".html") || strings.EqualFold(path.Ext(f.Name), ".htm") ||
		strings.EqualFold(path.Ext(f.Name), ".svg") || strings.EqualFold(path.Ext(f.Name), ".xml") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, rc)
}

// GET /api/v1/shares/{token}/thumb 公开分享缩略图（按 token 鉴权，不强制博客子树）。
//
// 与 publicMediaThumb 的区别：publicMediaThumb 强制「必须位于博客子树」（isBlogPostFile），
// 而分享目录不一定在博客子树里，因此分享通道需要专属端点——凭分享 token 可达即视为已授权，
// 仅对 image/*|video/* 生成缩略图（thumbMimeOf 同口径，svg 排除），复用 serveThumb 懒生成。
// 鉴权口径与 sharesContent 一致：file 分享取 row.FileID；dir 分享需 ?path= 经 collectDirFiles 解析。
// 安全边界：token 可达 + 仅 image|video + thumb 能力门控；私密文件不经过此通道（无合法 token 即 404）。
func (a *API) sharesThumb(w http.ResponseWriter, r *http.Request) {
	row, err := a.loadShare(r.Context(), r.PathValue("token"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "SHARE_NOT_FOUND", "share not found or revoked")
		return
	}
	var fileID string
	if row.Scope == "dir" {
		name := r.URL.Query().Get("path")
		if name == "" {
			writeErr(w, http.StatusBadRequest, "SHARE_PATH_REQUIRED", "directory share requires ?path=")
			return
		}
		files, err := a.collectDirFiles(r.Context(), row.DirID, "")
		if err != nil {
			writeErr(w, http.StatusNotFound, "SHARE_FILE_GONE", "shared directory not found")
			return
		}
		found := false
		for _, it := range files {
			if it.path == name {
				fileID = it.f.ID
				found = true
				break
			}
		}
		if !found {
			writeErr(w, http.StatusNotFound, "SHARE_PATH_NOT_FOUND", "file not found in shared directory")
			return
		}
	} else {
		fileID = row.FileID
	}
	if a.media == nil {
		writeErr(w, http.StatusServiceUnavailable, "MEDIA_DISABLED", "媒体模块未启用")
		return
	}
	f, err := a.files.Get(r.Context(), fileID)
	if err != nil || f == nil || f.Kind != "file" {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	if _, ok := thumbMimeOf(f.Mime); !ok {
		writeErr(w, http.StatusNotFound, "THUMB_UNAVAILABLE", "该媒体类型无缩略图")
		return
	}
	// 与 sharesContent 一致的文件级闸门（密码/付费），防缩略图泄漏受保护内容。
	if err := a.requireArticleUnlock(w, r, fileID); err != nil {
		if errors.Is(err, errPasswordRequired) {
			return
		}
		writeErr(w, http.StatusInternalServerError, "SHARE_THUMB_FAILED", err.Error())
		return
	}
	if err := a.requirePaidAccess(w, r, fileID); err != nil {
		if errors.Is(err, errPaidRequired) {
			return
		}
		writeErr(w, http.StatusInternalServerError, "SHARE_THUMB_FAILED", err.Error())
		return
	}
	a.serveThumb(w, r, f, "public, max-age=86400")
}

// DELETE /api/v1/shares/{token} 撤销分享
func (a *API) sharesRevoke(w http.ResponseWriter, r *http.Request) {
	// 博客分享为系统内置（token=blog）：不参与普通撤销，关闭博客请走 blog.open 开关
	// （否则管理员以为"撤销=关博客"，重启后 EnsureBlogSpace 不再自动复活会永久关闭；锁定避免歧义）
	if r.PathValue("token") == service.BlogToken {
		writeErr(w, http.StatusForbidden, "BLOG_SHARE_LOCKED", "博客分享为系统内置，关闭/开启博客请到博客管理页使用博客开关（blog.open）")
		return
	}
	// M6 修复：仅分享所有者（或 admin）可撤销。此前只按 token 撤销——token 一旦出现在
	// 公开页面/分享链接里（本就是给访客看的），任何登录用户都能拿它把站长的分享撤掉。
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "LOGIN_REQUIRED", "请先登录")
		return
	}
	var ownID string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT owner_id FROM shares WHERE token=?`, r.PathValue("token")).Scan(&ownID); err != nil {
		writeErr(w, http.StatusNotFound, "SHARE_NOT_FOUND", "share not found")
		return
	}
	if ownID != uid && !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "SHARE_FORBIDDEN", "无权撤销该分享")
		return
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(),
		`UPDATE shares SET revoked_at = ? WHERE token = ? AND revoked_at IS NULL`,
		now, r.PathValue("token"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SHARE_REVOKE_FAILED", err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeErr(w, http.StatusNotFound, "SHARE_NOT_FOUND", "share not found or already revoked")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// loadShare 按 token 加载未撤销、未过期的分享
func (a *API) loadShare(ctx context.Context, token string) (*shareRow, error) {
	token = strings.TrimSpace(token)
	var row shareRow
	err := a.db.QueryRowContext(ctx,
		`SELECT id, file_id, owner_id, token, permission, scope, COALESCE(dir_id,''), COALESCE(expires_at,0), created_at, COALESCE(revoked_at,0)
		 FROM shares WHERE token = ? AND (revoked_at IS NULL OR revoked_at = 0)`, token).
		Scan(&row.ID, &row.FileID, &row.OwnerID, &row.Token, &row.Permission, &row.Scope, &row.DirID, &row.ExpiresAt, &row.CreatedAt, &row.RevokedAt)
	if err != nil {
		return nil, err
	}
	if row.ExpiresAt > 0 && row.ExpiresAt < time.Now().UnixMilli() {
		return nil, errors.New("share expired")
	}
	return &row, nil
}

// newShareToken 生成 12 字节随机 token（24 hex 字符）。
func newShareToken() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// newID 生成短 uuid（无连字符）。
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b)
}
