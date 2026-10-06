// Package handler 提供 HTTP/WS 入口。
// files.go 实现文件 API（实施文档 §8.2 文件端点）：列表/上传/下载/目录/移动/删除/树。
// 阶段 1：单用户优先，owner 取自配置的默认 owner；多用户鉴权随阶段 3 接入。
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// ---- 文件 API ----

func (a *API) filesList(w http.ResponseWriter, r *http.Request) {
	// names=1：全空间文件名清单（[[ 联想 / 双链解析数据源）
	if r.URL.Query().Get("names") == "1" {
		items, err := a.files.ListWikiNames(r.Context(), a.homeSpaceID())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "FILES_LIST_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	// scope=drafts：全空间草稿箱视图；scope=trash：回收站视图；否则按 parent 列目录
	switch r.URL.Query().Get("scope") {
	case "drafts":
		items, err := a.files.ListDrafts(r.Context(), a.homeSpaceID())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "FILES_LIST_FAILED", err.Error())
			return
		}
		a.listFilesWithTags(w, r, items)
		return
	case "trash":
		items, err := a.files.ListTrash(r.Context(), a.homeSpaceID())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "FILES_LIST_FAILED", err.Error())
			return
		}
		a.listFilesWithTags(w, r, items)
		return
	}
	// tag={tagId}：按标签跨目录筛选（文件库/标签筛选共用）
	if tagID := r.URL.Query().Get("tag"); tagID != "" {
		items, err := a.files.ListByTag(r.Context(), a.homeSpaceID(), tagID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "FILES_LIST_FAILED", err.Error())
			return
		}
		a.listFilesWithTags(w, r, items)
		return
	}
	parent := r.URL.Query().Get("parent")
	// 爱库录精简发行：博客目录对站长可见（文件管理可直接拖 md 发布）；根目录/博客根仍禁止删除
	items, err := a.files.ListDir(r.Context(), a.homeSpaceID(), parent, a.operatingSiteID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FILES_LIST_FAILED", err.Error())
		return
	}
	a.listFilesWithTags(w, r, items)
}

// filesResolveWiki 按双链标题解析目标文件：GET /api/v1/files/resolve?name=标题
// （阅读页正文 [[标题]] 点击跳转用；返回 404 表示未匹配 → 前端灰显不可点）
func (a *API) filesResolveWiki(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeErr(w, http.StatusBadRequest, "RESOLVE_NO_NAME", "缺少文件名")
		return
	}
	id := a.files.ResolveWikiTitle(r.Context(), a.homeSpaceID(), name)
	if id == "" {
		writeErr(w, http.StatusNotFound, "RESOLVE_NOT_FOUND", "未找到该标题对应文件")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "name": name})
}

// filesRestore 从回收站恢复：POST /api/v1/files/{id}/restore
func (a *API) filesRestore(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单（此前无守卫，任意登录用户可删改全站文件）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	f, err := a.files.Restore(r.Context(), a.writeActor(r), id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "RESTORE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// filesPurge 彻底删除：DELETE /api/v1/files/{id}/purge
func (a *API) filesPurge(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单（此前无守卫，任意登录用户可删改全站文件）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	// 博客目录保护：站点公开根，禁止任何形式的删除
	if id == service.BlogDirID {
		writeErr(w, http.StatusForbidden, "BLOG_DIR_PROTECTED", "博客目录是站点根，禁止删除（子目录和文件不受限）")
		return
	}
	if err := a.files.Purge(r.Context(), a.writeActor(r), id); err != nil {
		writeErr(w, http.StatusBadRequest, "PURGE_FAILED", err.Error())
		return
	}
	// B15：清掉已失去源文件的媒体记录与缩略图派生对象（对账式，覆盖子树）
	a.cleanupMediaOrphans(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// filesEmptyTrash 清空回收站：POST /api/v1/files/trash/empty
func (a *API) filesEmptyTrash(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单（此前无守卫，任意登录用户可删改全站文件）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	n, err := a.files.EmptyTrash(r.Context(), a.writeActor(r), a.homeSpaceID())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "EMPTY_TRASH_FAILED", err.Error())
		return
	}
	// B15：清空回收站一次可能清掉整棵子树，同样走孤儿对账
	a.cleanupMediaOrphans(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "purged": n})
}

// maybeAutoPublishBlogFile 上传后自动发布（站长可配 blog.auto_publish_on_upload）。
// 仅当：开关开启 + 文件在博客子树内 + 是 markdown。
func (a *API) maybeAutoPublishBlogFile(ctx context.Context, f *service.File) {
	if f == nil || f.ID == "" {
		return
	}
	lower := strings.ToLower(f.Name)
	if !strings.HasSuffix(lower, ".md") && !strings.HasSuffix(lower, ".markdown") {
		return
	}
	if v, ok := a.cfg.Get("blog.auto_publish_on_upload"); ok {
		if s, _ := v.(string); s != "true" {
			return
		}
	} else {
		return
	}
	// 判断是否在博客子树
	id := f.ParentID
	for i := 0; i < 32 && id != ""; i++ {
		if id == service.BlogDirID {
			_, _ = a.db.ExecContext(ctx,
				`UPDATE files SET content_state=json_set(COALESCE(content_state,'{}'), '$.status', 'published'), updated_at=? WHERE id=?`,
				time.Now().UnixMilli(), f.ID)
			return
		}
		var pid string
		if err := a.db.QueryRowContext(ctx,
			`SELECT COALESCE(parent_id,'') FROM files WHERE id=?`, id).Scan(&pid); err != nil {
			return
		}
		id = pid
	}
}

// filesSetStatus 更新内容状态：PUT /api/v1/files/{id}/status {status:"draft"|"published"}
func (a *API) filesSetStatus(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单（此前无守卫，任意登录用户可删改全站文件）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	f, err := a.files.SetStatus(r.Context(), a.writeActor(r), id, req.Status)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "STATUS_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (a *API) filesTree(w http.ResponseWriter, r *http.Request) {
	items, err := a.files.Tree(r.Context(), a.homeSpaceID())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "FILES_TREE_FAILED", err.Error())
		return
	}
	// 精简发行：博客树在文件管理可见（站长可直接整理/拖拽发布）
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// blogDirHiddenIDs 返回博客目录及其全部子孙 id（存储层不可见集合）。
// "目录即博客"约定：博客目录只在博客展示层（/blog 管理）操作，文件存储层不挂载。
// 爱库录精简发行：列表/树已放开可见；此函数保留给搜索 scope 等逻辑复用。
func (a *API) blogDirHiddenIDs(ctx context.Context) map[string]bool {
	ids := map[string]bool{service.BlogDirID: true}
	level := []string{service.BlogDirID}
	for len(level) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(level)), ",")
		args := make([]any, len(level))
		for i, p := range level {
			args[i] = p
		}
		rows, err := a.db.QueryContext(ctx,
			`SELECT id FROM files WHERE parent_id IN (`+ph+`) AND deleted_at IS NULL`, args...)
		if err != nil {
			return ids
		}
		var next []string
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids[id] = true
				next = append(next, id)
			}
		}
		rows.Close()
		level = next
	}
	return ids
}

// blogScopeIDs 返回博客子树 id 列表（搜索范围 scope=blog/file 限定用）。
// SQLite 变量数上限 999，列表超 900 或查询失败返回 ok=false（调用方回退全量搜索）。
func (a *API) blogScopeIDs(ctx context.Context) ([]string, bool) {
	ids := []string{service.BlogDirID}
	level := []string{service.BlogDirID}
	const max = 900
	for len(level) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(level)), ",")
		args := make([]any, len(level))
		for i, p := range level {
			args[i] = p
		}
		rows, err := a.db.QueryContext(ctx,
			`SELECT id FROM files WHERE parent_id IN (`+ph+`) AND deleted_at IS NULL`, args...)
		if err != nil {
			return nil, false
		}
		var next []string
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
				next = append(next, id)
				if len(ids) > max {
					rows.Close()
					return nil, false
				}
			}
		}
		rows.Close()
		level = next
	}
	return ids, true
}

func (a *API) filesGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	f, err := a.files.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "FILE_GET_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// filesSummary 查询文件 AI 入库解读（摘要+标签+元数据 K19）。无记录/未完成返回空对象（前端自行降级）。
// GET /api/v1/files/{id}/summary
func (a *API) filesSummary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if a.summarizer == nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	row, err := a.summarizer.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SUMMARY_GET_FAILED", err.Error())
		return
	}
	if row == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "none"})
		return
	}
	writeJSON(w, http.StatusOK, row)
}

// filesUpload 处理 multipart 拖拽上传：POST /api/v1/files/upload?parent=<dirId>
// 字段名 file（可多个，支持文件夹拖拽：每个文件可带同名 path 字段记录相对路径，如 "sub/a.md"，后端自动建目录链）；
// 也支持 FormValue("path") 上传到指定路径（供 WebDAV 类接入）。
func (a *API) filesUpload(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单（此前无守卫，任意登录用户可删改全站文件）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	parent := r.URL.Query().Get("parent")
	spaceID := a.homeSpaceID()
	ownerID := a.writeActor(r)

	// 单文件直传（流式，兼容 curl -F file=@x）与 multipart 多文件
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "UPLOAD_BAD_FORM", err.Error())
		return
	}
	type result struct {
		File *service.File `json:"file"`
		Err  string        `json:"error,omitempty"`
	}
	results := []result{}
	if fhs := r.MultipartForm.File["file"]; len(fhs) > 0 {
		paths := r.MultipartForm.Value["path"]
		for i, fh := range fhs {
			rel := ""
			if i < len(paths) {
				rel = paths[i]
			}
			rel = strings.ReplaceAll(rel, "\\", "/")
			name := path.Base(rel)
			if name == "." || name == "" || name == "/" {
				name = fh.Filename
			}
			targetParent := parent
			dir := path.Dir(rel)
			if dir != "." && dir != "/" && dir != "" {
				pid, err := a.files.EnsurePath(r.Context(), ownerID, spaceID, parent, dir)
				if err != nil {
					results = append(results, result{Err: err.Error()})
					continue
				}
				targetParent = pid
			}
			f, err := fh.Open()
			if err != nil {
				results = append(results, result{Err: err.Error()})
				continue
			}
			file, err := a.files.Upload(r.Context(), ownerID, spaceID, targetParent, name, fh.Header.Get("Content-Type"), f, fh.Size, a.operatingSiteID(r))
			f.Close()
			if err == nil {
				a.maybeAutoPublishBlogFile(r.Context(), file)
			}
			results = append(results, result{File: file, Err: errString(err)})
		}
	} else if p := r.FormValue("path"); p != "" {
		// 按路径上传（父目录由路径推导，自动建目录）
		p = strings.ReplaceAll(p, "\\", "/")
		name := path.Base(p)
		dir := path.Dir(p)
		targetParent := parent
		if dir != "." && dir != "/" && dir != "" {
			pid, err := a.files.EnsurePath(r.Context(), ownerID, spaceID, parent, dir)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "UPLOAD_MKDIR_FAILED", err.Error())
				return
			}
			targetParent = pid
		}
		file, err := a.files.Upload(r.Context(), ownerID, spaceID, targetParent, name, r.Header.Get("Content-Type"), r.Body, r.ContentLength, a.operatingSiteID(r))
		if err == nil {
			a.maybeAutoPublishBlogFile(r.Context(), file)
		}
		results = append(results, result{File: file, Err: errString(err)})
	} else {
		writeErr(w, http.StatusBadRequest, "UPLOAD_NO_FILE", "no file field")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (a *API) filesMkdir(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单（此前无守卫，任意登录用户可删改全站文件）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	var req struct {
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	f, err := a.files.CreateDir(r.Context(), a.writeActor(r), a.homeSpaceID(), req.ParentID, req.Name, a.operatingSiteID(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "MKDIR_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// filesCreateDoc 新建文档：POST /api/v1/files/doc {parent_id, name, content}
func (a *API) filesCreateDoc(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单（此前无守卫，任意登录用户可删改全站文件）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	var req struct {
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
		Content  string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "DOC_NO_NAME", "name required")
		return
	}
	f, err := a.files.CreateDoc(r.Context(), a.writeActor(r), a.homeSpaceID(), req.ParentID, req.Name, req.Content, a.operatingSiteID(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "DOC_CREATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// filesUpdateContent 更新文本文件内容：PUT /api/v1/files/{id}/content {content}
func (a *API) filesUpdateContent(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单（此前无守卫，任意登录用户可删改全站文件）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	f, err := a.files.UpdateContent(r.Context(), a.writeActor(r), id, req.Content)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
			return
		}
		writeErr(w, http.StatusBadRequest, "UPDATE_CONTENT_FAILED", err.Error())
		return
	}
	// B6：保存成功后投订阅通知 + 落 @提及（增值动作，失败不影响保存结果）
	a.afterContentChange(r, id, a.curUserID(r), req.Content, f)
	writeJSON(w, http.StatusOK, f)
}

func (a *API) filesMove(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单（此前无守卫，任意登录用户可删改全站文件）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	// 博客目录保护：站点公开根，禁止移动/重命名（改名会破坏"目录即博客"心智）
	if id == service.BlogDirID {
		writeErr(w, http.StatusForbidden, "BLOG_DIR_PROTECTED", "博客目录是站点根，禁止移动/重命名（子目录和文件不受限）")
		return
	}
	var req struct {
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	f, err := a.files.Move(r.Context(), a.writeActor(r), id, req.ParentID, req.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "MOVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (a *API) filesCopy(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单（此前无守卫，任意登录用户可删改全站文件）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	f, err := a.files.Copy(r.Context(), a.writeActor(r), id, req.ParentID, req.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "COPY_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (a *API) filesDelete(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单（此前无守卫，任意登录用户可删改全站文件）
	if !a.blogAuthorOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	// 博客目录保护：站点公开根，禁止软删（回收站也不允许——整站下线风险）
	if id == service.BlogDirID {
		writeErr(w, http.StatusForbidden, "BLOG_DIR_PROTECTED", "博客目录是站点根，禁止删除（子目录和文件不受限）")
		return
	}
	// 采集产物删除记忆：软删前提取 source_url 记墓碑，防"清理→重采→又回来"死循环
	a.tombstoneCollectArtifact(r.Context(), id)
	if err := a.files.Delete(r.Context(), a.writeActor(r), id); err != nil {
		writeErr(w, http.StatusBadRequest, "DELETE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// reFrontSourceURL 提取采集 markdown 的 source_url（front matter 溯源字段）。
var reFrontSourceURL = regexp.MustCompile(`(?m)^source_url:\s*(\S+)`)

// tombstoneCollectArtifact 删除采集产物前记墓碑：仅处理采集目录下、且内容含 source_url 的
// markdown 文件（采集器入库产物）；非采集文件无操作。
func (a *API) tombstoneCollectArtifact(ctx context.Context, id string) {
	f, err := a.files.Get(ctx, id)
	if err != nil || f == nil || !strings.HasPrefix(f.StorageRef, "采集/") {
		return
	}
	rc, _, err := a.files.Content(ctx, id)
	if err != nil {
		return
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	m := reFrontSourceURL.FindSubmatch(data)
	if m == nil {
		return
	}
	// 爱库录精简：已裁剪采集墓碑（collector.TombstoneSource）
}

// filesContent 下载/预览：GET /api/v1/files/{id}/content?download=1
func (a *API) filesContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rc, f, err := a.files.Content(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "CONTENT_FAILED", err.Error())
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", contentType(f.Mime, f.Name))
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+f.Name+"\"")
	} else {
		w.Header().Set("Content-Disposition", "inline")
	}
	// 防同源存储型 XSS：html/svg/xml 等活性文档一律降级为纯文本预览 + 禁 MIME 嗅探
	if strings.EqualFold(filepath.Ext(f.Name), ".html") || strings.EqualFold(filepath.Ext(f.Name), ".htm") ||
		strings.EqualFold(filepath.Ext(f.Name), ".svg") || strings.EqualFold(filepath.Ext(f.Name), ".xml") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-AiKmap-File-Id", f.ID)

	// 文本类文件做 UTF-8 规范化：Windows 老文本常为 GBK/CP936 编码，
	// 统一转为 UTF-8 输出，阅读/分享/API 一处修复全局受益。
	if strings.HasPrefix(f.Mime, "text/") {
		data, err := io.ReadAll(rc)
		if err == nil {
			if out, ok := utf8Normalize(data); ok {
				_, _ = w.Write(out)
				return
			}
		}
	}
	_, _ = io.Copy(w, rc)
}

// utf8Normalize 文本字节 → UTF-8：已是合法 UTF-8 原样返回；否则尝试按 GBK 解码转 UTF-8。
func utf8Normalize(b []byte) ([]byte, bool) {
	if utf8.Valid(b) {
		return b, true
	}
	if dec, err := simplifiedchinese.GBK.NewDecoder().Bytes(b); err == nil && utf8.Valid(dec) {
		return dec, true
	}
	return b, false
}

// homeSpaceID 阶段 1 返回 home 空间（多用户随阶段 3 接入）。
func (a *API) homeSpaceID() string {
	if a.cfg != nil {
		if v := a.cfg.GetString("system.home_space_id"); v != "" {
			return v
		}
	}
	return "00000000-0000-0000-0000-000000000002"
}

func (a *API) homeOwnerID() string {
	if a.cfg != nil {
		if v := a.cfg.GetString("system.owner_id"); v != "" {
			return v
		}
	}
	return "00000000-0000-0000-0000-000000000001"
}

// contentType 由 MIME 与扩展名推断（前端预览用）。
func contentType(mime, name string) string {
	if mime != "" && !strings.HasPrefix(mime, "application/octet-stream") {
		return mime
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".txt", ".log":
		return "text/plain; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
