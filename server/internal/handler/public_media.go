package handler

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// publicMediaGet GET /api/v1/public/media/{id}
// 公开服务博客子树内的媒体文件（封面图 / 正文粘贴上传的图片 / 文章内嵌的音视频）。
// 安全边界：
//   - 仅 image|video|audio（svg 除外——活性文档，防存储型 XSS）；
//   - 文件必须位于博客目录（BlogDirID）子树内（递归上溯校验，含任意层级子目录）；
//   - nosniff + inline。
//
// 音视频额外支持 Range：交由 http.ServeContent 处理，视频才能拖动进度、边下边播；
// 否则只能从头线性播放（大文件基本等于不可用）。
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

	if !isPublicMediaMime(f.Mime) {
		writeErr(w, http.StatusNotFound, "NOT_PUBLIC_MEDIA", "仅公开博客子树内的媒体")
		return
	}
	if !a.isBlogPostFile(a.db, id, service.BlogDirID) {
		writeErr(w, http.StatusNotFound, "NOT_PUBLIC_MEDIA", "仅公开博客子树内的媒体")
		return
	}

	w.Header().Set("Content-Type", f.Mime)
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	// 本地磁盘后端返回 *os.File（天然 ReadSeeker）→ 交给 ServeContent 处理 Range/If-Modified-Since；
	// 流式后端（S3/WebDAV）不满足 ReadSeeker，退化为整段输出。
	if rs, ok := rc.(io.ReadSeeker); ok {
		http.ServeContent(w, r, f.Name, time.UnixMilli(f.UpdatedAt), rs)
		return
	}
	_, _ = io.Copy(w, rc)
}

// publicMediaInfoGet GET /api/v1/public/media/{id}/info
// 公开（匿名）媒体元信息：图片宽高 / 音视频时长·分辨率·编码·码率。
//
// 安全边界与 publicMediaGet 完全一致：仅博客子树内、仅 image|video|audio（svg 排除）；
// 不存在或越界一律 404，不区分原因、不泄漏内部状态。
//
// 为什么不复用 /files/{id}/media：那条在鉴权中间件内，访客读不到；
// 而主题/第三方在公开页展示「视频时长」这类信息必须匿名可用。
// 响应有意只回展示所需字段，不暴露 thumbnail_ref / transcoded_ref 等内部引用。
func (a *API) publicMediaInfoGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > 64 || !isSafeID(id) {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "非法文件 ID")
		return
	}
	if a.media == nil {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	f, err := a.files.Get(r.Context(), id)
	if err != nil || f == nil || f.Kind != "file" {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	if !isPublicMediaMime(f.Mime) || !a.isBlogPostFile(a.db, id, service.BlogDirID) {
		writeErr(w, http.StatusNotFound, "NOT_PUBLIC_MEDIA", "仅公开博客子树内的媒体")
		return
	}
	info, err := a.ensureMediaInfo(r.Context(), id, f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "MEDIA_SAVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"file_id":      info.FileID,
		"kind":         mediaKindOf(f.Mime),
		"width":        info.Width,
		"height":       info.Height,
		"duration_ms":  info.DurationMS,
		"codec":        info.Codec,
		"bitrate":      info.Bitrate,
		"probe_status": info.ProbeStatus,
	})
}

// isPublicMediaMime 公开媒体端点放行的 mime：图片 / 视频 / 音频。
// 明确排除 image/svg+xml —— svg 是活性文档（可内嵌脚本），公开直出等于存储型 XSS 通道。
func isPublicMediaMime(mime string) bool {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	if m == "image/svg+xml" {
		return false
	}
	return strings.HasPrefix(m, "image/") || strings.HasPrefix(m, "video/") || strings.HasPrefix(m, "audio/")
}

// mediaKindOf 把 mime 归一为 image|video|audio（供主题/前端做渲染分派；其他为空串）。
func mediaKindOf(mime string) string {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch {
	case strings.HasPrefix(m, "image/"):
		return "image"
	case strings.HasPrefix(m, "video/"):
		return "video"
	case strings.HasPrefix(m, "audio/"):
		return "audio"
	}
	return ""
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

// publicMediaThumb GET /api/v1/public/media/{id}/thumb
// 公开缩略图（B15）：边界与 publicMediaGet 完全一致 —— 先校验「博客子树内的公开媒体」，
// 再判断「该媒体类型是否有缩略图概念」。
//
// 用途：公开页的列表/卡片显示封面。原图常是 MB 级，直接当缩略图用会白白吃掉移动端流量，
// 而缩略图是 480px JPEG（通常几十 KB）。
//
// 顺序上先查子树后查类型：既有的 publicMediaGet/publicMediaInfoGet 已公开「该 id 是否为
// 博客子树媒体」，因此这里的 404 不额外泄漏任何信息。
func (a *API) publicMediaThumb(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > 64 || !isSafeID(id) {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "非法文件 ID")
		return
	}
	if a.media == nil {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	f, err := a.files.Get(r.Context(), id)
	if err != nil || f == nil || f.Kind != "file" {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	if !isPublicMediaMime(f.Mime) || !a.isBlogPostFile(a.db, id, service.BlogDirID) {
		writeErr(w, http.StatusNotFound, "NOT_PUBLIC_MEDIA", "仅公开博客子树内的媒体")
		return
	}
	if _, ok := thumbMimeOf(f.Mime); !ok {
		// 音频等没有可展示的视觉缩略图（封面提取是另一件事，不在本批范围）
		writeErr(w, http.StatusNotFound, "THUMB_UNAVAILABLE", "该媒体类型无缩略图")
		return
	}
	a.serveThumb(w, r, f, "public, max-age=86400")
}

// servePublicOriginal 直出公开媒体的原文件（与 publicMediaGet 同款边界，但假定调用方已校验 id/f）。
// 供 publicMediaPlay 在「门控关闭 / 暂无产物 / 产物丢失」时回落，避免让既有博客视频集体黑屏。
func (a *API) servePublicOriginal(w http.ResponseWriter, r *http.Request, id string, f *service.File) {
	rc, _, err := a.files.Content(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", f.Mime)
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	// 本地磁盘后端返回 *os.File（天然 ReadSeeker）→ 交给 ServeContent 处理 Range/If-Modified-Since；
	// 流式后端（S3/WebDAV）不满足 ReadSeeker，退化为整段输出。
	if rs, ok := rc.(io.ReadSeeker); ok {
		http.ServeContent(w, r, f.Name, time.UnixMilli(f.UpdatedAt), rs)
		return
	}
	_, _ = io.Copy(w, rc)
}

// publicMediaPlay GET /api/v1/public/media/{id}/play
// 公开播放转码产物（B17）：门控开启且已有可用转码产物时直出 play-<sig>.mp4（Web 友好 H.264/AAC），
// 否则回落到原文件。
//
// 为什么要「回落原文件」：① 门控关闭时，关停的必须是「转码」而非「播放」—— 原文件一直能播，
// 关掉转码只是不再提供二次编码版本；② 视频还没转码完/转码失败，也应能播原文件而非 404。
// 这样「关闭转码」不会让公开博客里的视频集体黑屏。
func (a *API) publicMediaPlay(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > 64 || !isSafeID(id) {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "非法文件 ID")
		return
	}
	if a.media == nil {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	f, err := a.files.Get(r.Context(), id)
	if err != nil || f == nil || f.Kind != "file" {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	if !isPublicMediaMime(f.Mime) || !a.isBlogPostFile(a.db, id, service.BlogDirID) {
		writeErr(w, http.StatusNotFound, "NOT_PUBLIC_MEDIA", "仅公开博客子树内的媒体")
		return
	}
	// 门控关闭 → 直接回落原文件（不读转码状态）。
	if !a.transcodeEnabled() {
		a.servePublicOriginal(w, r, id, f)
		return
	}
	info, ok := a.media.Get(r.Context(), id)
	if !ok || info == nil || info.TranscodeStatus != service.TranscodeDone || info.TranscodedRef == "" {
		a.servePublicOriginal(w, r, id, f)
		return
	}
	if _, exists := a.files.DerivedStat(r.Context(), info.TranscodedRef); !exists {
		a.servePublicOriginal(w, r, id, f)
		return
	}
	rc, err := a.files.DerivedReader(r.Context(), info.TranscodedRef)
	if err != nil {
		a.servePublicOriginal(w, r, id, f)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if rs, ok := rc.(io.ReadSeeker); ok {
		http.ServeContent(w, r, f.Name, time.UnixMilli(info.TranscodeUpdatedAt), rs)
		return
	}
	_, _ = io.Copy(w, rc)
}
