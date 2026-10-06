package handler

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"time"
)

// ---- B5 写作增强：版本历史 列表 / 下载 / 恢复 ----
//
// 权限口径与同级文件接口（filesUpdateContent / filesReplaceContentBinary）保持一致：
// 已登录即可读，恢复动作靠「可逆性」兜底 —— RestoreVersion 会把恢复前的内容也存一份快照，
// 所以「恢复」本身可以被再次恢复，不存在不可撤销的破坏。

// fileVersions GET /api/v1/files/{id}/versions
func (a *API) fileVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	vs, err := a.files.Versions(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "VERSIONS_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": vs})
}

// fileVersionContent GET /api/v1/files/{id}/versions/{v}/content
func (a *API) fileVersionContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	v, _ := strconv.Atoi(r.PathValue("v"))
	rc, f, err := a.files.VersionContent(r.Context(), id, v)
	if err != nil {
		writeErr(w, http.StatusNotFound, "VERSION_NOT_FOUND", err.Error())
		return
	}
	defer rc.Close()
	body, rerr := io.ReadAll(io.LimitReader(rc, 128<<20))
	if rerr != nil {
		writeErr(w, http.StatusInternalServerError, "VERSION_READ_FAILED", rerr.Error())
		return
	}
	w.Header().Set("Content-Type", f.Mime)
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(f.Name+"-v"+strconv.Itoa(v)))
	http.ServeContent(w, r, f.Name, time.UnixMilli(f.UpdatedAt), bytes.NewReader(body))
}

// fileVersionRestore POST /api/v1/files/{id}/versions/{v}/restore
func (a *API) fileVersionRestore(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单
	if !a.blogAuthorOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	v, _ := strconv.Atoi(r.PathValue("v"))
	nf, err := a.files.RestoreVersion(r.Context(), a.curUserID(r), id, v)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VERSION_RESTORE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nf)
}
