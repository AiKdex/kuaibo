package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// publicMediaGet GET /api/v1/public/media/{id}
// 公开服务博客子树内的图片文件（封面图 / 正文粘贴上传图片）。
// 安全边界：
//   - 仅 image/* mime（svg 除外——活性文档，防存储型 XSS）；
//   - 文件必须位于博客目录（BlogDirID）子树内（递归上溯校验，含任意层级子目录）；
//   - nosniff + inline，长缓存（图片内容不可变，按 id 寻址）。
func (a *API) publicMediaGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > 64 || !isSafeID(id) {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "非法文件 ID")
		return
	}
	rc, f, err := a.files.Content(r.Context(), id)
	if err != nil {
		// 公开端点：不区分不存在/目录/存储错误，一律 404（不泄漏内部状态）
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	defer rc.Close()

	if !strings.HasPrefix(f.Mime, "image/") || strings.EqualFold(f.Mime, "image/svg+xml") {
		writeErr(w, http.StatusNotFound, "NOT_PUBLIC_MEDIA", "仅公开博客子树内的图片")
		return
	}
	if !a.isBlogPostFile(a.db, id, service.BlogDirID) {
		writeErr(w, http.StatusNotFound, "NOT_PUBLIC_MEDIA", "仅公开博客子树内的图片")
		return
	}

	w.Header().Set("Content-Type", f.Mime)
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, rc)
}

// isSafeID 限定为 newID() 形态的 32 位十六进制（或 36 位带连字符 UUID），防路径注入。
func isSafeID(id string) bool {
	for _, c := range id {
		ok := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '-'
		if !ok {
			return false
		}
	}
	return len(id) >= 32 && len(id) <= 36
}
