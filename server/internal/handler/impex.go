// Package handler 提供 HTTP/WS 入口。
// impex.go 实现导入导出中心 API（R11/R12）：
//
//	POST /api/v1/imports               创建导入（multipart：ZIP/Obsidian；JSON：URL/剪贴板）
//	GET  /api/v1/imports               导入任务历史
//	GET  /api/v1/imports/{id}          任务详情（进度 / 失败清单）
//	POST /api/v1/imports/{id}/retry    重试失败项
//	POST /api/v1/exports               创建导出（folder|obsidian|tag|collection）
//	GET  /api/v1/exports               导出任务历史
//	GET  /api/v1/exports/{id}          任务详情
//	GET  /api/v1/exports/{id}/download 下载导出 ZIP
//	POST /api/v1/exports/{id}/retry    重试失败项
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// impexCreateImport 创建导入任务。
// multipart：source=zip|obsidian，file=上传的 ZIP；JSON：source=url|clipboard。
func (a *API) impexCreateImport(w http.ResponseWriter, r *http.Request) {
	owner := a.homeOwnerID()
	space := a.homeSpaceID()

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(256 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "IMPEX_BAD_FORM", err.Error())
			return
		}
		source := r.FormValue("source")
		if source == "" {
			source = "zip"
		}
		parent := r.FormValue("parent")
		strategy := r.FormValue("on_conflict")
		if strategy == "" {
			strategy = "rename"
		}
		fhs := r.MultipartForm.File["file"]
		if len(fhs) == 0 {
			writeErr(w, http.StatusBadRequest, "IMPEX_NO_FILE", "缺少上传文件（file 字段）")
			return
		}
		f, err := fhs[0].Open()
		if err != nil {
			writeErr(w, http.StatusBadRequest, "IMPEX_OPEN_FAILED", err.Error())
			return
		}
		defer f.Close()
		job, err := a.impex.CreateZipImport(r.Context(), owner, space, source, parent, strategy, f, fhs[0].Size)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "IMPEX_CREATE_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}

	var req struct {
		Source string `json:"source"`
		Parent string `json:"parent"`
		URL    string `json:"url"`
		UA     string `json:"ua"`
		Name   string `json:"name"`
		Text   string `json:"text"`
		Image  string `json:"image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	var (
		job *service.ImpexJob
		err error
	)
	switch req.Source {
	case "url":
		job, err = a.impex.CreateURLImport(r.Context(), owner, space, req.Parent, req.URL, req.UA)
	case "clipboard":
		job, err = a.impex.CreateClipboardImport(r.Context(), owner, space, req.Parent, req.Name, req.Text, req.Image)
	default:
		writeErr(w, http.StatusBadRequest, "IMPEX_BAD_SOURCE", "ZIP/Obsidian 导入请使用 multipart 上传")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "IMPEX_CREATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// impexCreateExport 创建导出任务：{source, target_id, name}。
func (a *API) impexCreateExport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source   string `json:"source"`
		TargetID string `json:"target_id"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error())
		return
	}
	job, err := a.impex.CreateExport(r.Context(), a.homeOwnerID(), a.homeSpaceID(), req.Source, req.TargetID, req.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "IMPEX_CREATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (a *API) impexListImports(w http.ResponseWriter, r *http.Request) {
	a.impexList(w, r, service.ImpexImport)
}

func (a *API) impexListExports(w http.ResponseWriter, r *http.Request) {
	a.impexList(w, r, service.ImpexExport)
}

func (a *API) impexList(w http.ResponseWriter, r *http.Request, kind string) {
	// M11 修复：普通用户仅能列出自己的任务；admin 列出全部
	ownerID := ""
	if !a.isAdmin(r) {
		ownerID = a.curUserID(r)
		if ownerID == "" {
			writeErr(w, http.StatusUnauthorized, "LOGIN_REQUIRED", "请先登录")
			return
		}
	}
	items, err := a.impex.List(r.Context(), kind, ownerID, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "IMPEX_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// impexOwned M11 归属校验：仅任务 owner（系统任务由 admin 代管）可访问/操作。
// 通过返回 true；失败已写响应并返回 false，调用方应立即 return。
func (a *API) impexOwned(w http.ResponseWriter, r *http.Request, id string) bool {
	uid := a.curUserID(r)
	if uid == "" {
		writeErr(w, http.StatusUnauthorized, "LOGIN_REQUIRED", "请先登录")
		return false
	}
	owner, err := a.impex.OwnerOf(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "IMPEX_NOT_FOUND", "job not found")
		return false
	}
	if owner != uid && owner != service.SystemOwnerID && !a.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "IMPEX_FORBIDDEN", "无权访问该任务")
		return false
	}
	return true
}

// impexGet 任务详情：GET /api/v1/{imports|exports}/{id}
func (a *API) impexGet(w http.ResponseWriter, r *http.Request) {
	if !a.impexOwned(w, r, r.PathValue("id")) {
		return
	}
	j, err := a.impex.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "IMPEX_NOT_FOUND", "job not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "IMPEX_GET_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, j)
}

// impexRetry 重试失败项：POST /api/v1/{imports|exports}/{id}/retry
func (a *API) impexRetry(w http.ResponseWriter, r *http.Request) {
	if !a.impexOwned(w, r, r.PathValue("id")) {
		return
	}
	j, err := a.impex.Retry(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "IMPEX_RETRY_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, j)
}

// impexDownload 下载导出产物：GET /api/v1/exports/{id}/download
func (a *API) impexDownload(w http.ResponseWriter, r *http.Request) {
	if !a.impexOwned(w, r, r.PathValue("id")) {
		return
	}
	rc, name, size, err := a.impex.ArtifactStream(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "IMPEX_DOWNLOAD_FAILED", err.Error())
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	_, _ = io.Copy(w, rc)
}
