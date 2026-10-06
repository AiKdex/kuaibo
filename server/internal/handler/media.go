// media.go 媒体元信息端点（B8）。
//
//	GET /api/v1/files/{id}/media   媒体元信息（登录态；图片宽高 / 音视频时长·分辨率·编码）
//
// 懒探测：缓存命中直接返回；否则就地探测并落库（见 ensureMediaInfo）。
//
// 音视频为什么必须 ffprobe：容器（mp4/mkv/mov）的元信息没有「解 header 就够」的通用做法，
// moov 原子可能位于文件尾，纯流式读取不可靠。因此音视频探测**需要宿主机路径**，
// 只对本地磁盘后端可用；远程后端（S3/WebDAV）与未装 ffprobe 的环境一律安静降级为
// probe_status='unsupported'，表结构无需二次迁移，装好 ffmpeg 后自动生效。
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// fileMediaGet GET /api/v1/files/{id}/media
func (a *API) fileMediaGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || len(id) > 64 {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "非法文件 ID")
		return
	}
	if a.media == nil {
		writeErr(w, http.StatusServiceUnavailable, "MEDIA_DISABLED", "媒体模块未启用")
		return
	}
	f, err := a.files.Get(r.Context(), id)
	if err != nil || f == nil || f.Kind != "file" {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	info, err := a.ensureMediaInfo(r.Context(), id, f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "MEDIA_SAVE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// ensureMediaInfo 取媒体元信息：缓存命中直接返回，否则懒探测并落库。
//
// 缓存条件**有意排除** pending 与 unsupported 两种状态：
//   - pending：从未探测过；
//   - unsupported：上次探测时环境不具备（未装 ffprobe，或存储后端非本地盘）。
//     环境会变 —— 若不重探，装好 ffmpeg 后旧记录会永远停在 unsupported，
//     这对「能力可降级」的承诺是致命的。重探成本极低：无 ffprobe 时只做一次
//     exec.LookPath（微秒级，完全不碰文件 IO）。
//
// 探测按 mime 分派：图片只解 header（不解全图，省内存），音视频交 ffprobe（需本地路径）。
func (a *API) ensureMediaInfo(ctx context.Context, id string, f *service.File) (*service.MediaInfo, error) {
	prev, hasPrev := a.media.Get(ctx, id)
	if hasPrev &&
		prev.ProbeStatus != service.MediaPending && prev.ProbeStatus != service.MediaUnsupported {
		return prev, nil
	}
	info := &service.MediaInfo{FileID: id, ProbeStatus: service.MediaUnsupported}
	// 🔴 Upsert 是全列覆盖：重探时必须把已有派生引用带过来，否则会把已生成的缩略图引用清空。
	// （unsupported 不是终态 → 会反复重探；若不带过来，表现为「缩略图生成过一次后又没了」。）
	if hasPrev {
		// B17：Upsert 全列覆盖，重探必须带过全部派生字段（含转码状态），否则会清掉已生成的转码产物引用。
		info.CarryDerived(prev)
	}
	mime := strings.ToLower(f.Mime)
	switch {
	case strings.HasPrefix(mime, "image/") && mime != "image/svg+xml":
		// svg 是活性文档，不解码；其余 image/* 只读 header 取宽高
		if rc, _, err := a.files.Content(ctx, id); err == nil {
			defer rc.Close()
			if iw, ih, format, ok := service.ProbeImage(rc); ok {
				info.Width, info.Height, info.Codec = iw, ih, format
				info.ProbeStatus = service.MediaOK
			}
		}
	case strings.HasPrefix(mime, "video/") || strings.HasPrefix(mime, "audio/"):
		// 音视频要宿主机路径（随机访问）；非本地后端 LocalPath 直接返回 false
		if p, ok := a.files.LocalPath(ctx, id); ok {
			if av, ok := service.ProbeAV(ctx, p); ok {
				info.DurationMS, info.Width, info.Height = av.DurationMS, av.Width, av.Height
				info.Codec, info.Bitrate = av.Codec, av.Bitrate
				info.ProbeStatus = service.MediaOK
			}
		}
	}
	if err := a.media.Upsert(ctx, info); err != nil {
		return nil, err
	}
	return info, nil
}

// ---- 媒体缩略图（B15）：懒生成 + 派生对象直出 ----
//
// 为什么懒生成：上传链路零侵入（与 B13 探测同一策略）；缩略图只有被消费时才值得花 CPU。
// 与探测的三处刻意差异：
//  1. 门控关闭 / ffmpeg 缺失 / 生成失败 → **不写任何状态**。「没有缩略图」不是元信息，
//     写进 thumbnail_ref 会让人无法区分「没生成过」与「生成失败」，也会让环境补齐后
//     永远不重试；
//  2. 复用判定要 Stat 派生对象是否真的还在（存储可能被人工清理/迁移过）；
//  3. 缩略图缺失是 404 + 语义化错误码，不是 500 —— 前端据此回退到图标。

// thumbEnabled 缩略图能力门控（运行期判定，改完即时生效，见 service.CapabilityApply）。
func (a *API) thumbEnabled() bool {
	on, _ := service.CapabilityEnabled(a.db, a.cfg, service.CapThumb)
	return on
}

// thumbMimeOf 判断该 mime 能否做缩略图，并回答「是否按视频抽帧处理」。
// 与公开媒体端点同一口径：仅 image/* 与 video/*；svg 是活性文档（可内嵌脚本），不解码。
func thumbMimeOf(mime string) (isVideo, ok bool) {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	if m == "image/svg+xml" {
		return false, false
	}
	switch {
	case strings.HasPrefix(m, "video/"):
		return true, true
	case strings.HasPrefix(m, "image/"):
		return false, true
	}
	return false, false
}

// ensureThumb 取缩略图派生对象 key：命中即复用，否则就地生成并落库。
func (a *API) ensureThumb(ctx context.Context, f *service.File) (string, bool) {
	if a.media == nil || a.files == nil {
		return "", false
	}
	isVideo, ok := thumbMimeOf(f.Mime)
	if !ok {
		return "", false
	}
	// 门控必须**先于**缓存命中：否则关闭后已缓存的缩略图仍能被取到，
	// 面板显示「已关闭」而端点照常 200 —— 判定与实现漂移（B14 同类教训）。
	if !a.thumbEnabled() {
		return "", false
	}
	info, err := a.ensureMediaInfo(ctx, f.ID, f)
	if err != nil || info == nil {
		return "", false
	}
	if info.ThumbnailRef != "" {
		if _, exists := a.files.DerivedStat(ctx, info.ThumbnailRef); exists {
			return info.ThumbnailRef, true
		}
	}
	if !service.FFmpegAvailable() {
		return "", false
	}
	// 抽帧需要**随机访问**源文件 → 只有本地（或本地挂载）后端可用；远程后端安静降级
	src, ok := a.files.LocalPath(ctx, f.ID)
	if !ok {
		return "", false
	}
	tmp, err := os.CreateTemp("", "aiklog-thumb-*.jpg")
	if err != nil {
		return "", false
	}
	tmpName := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpName)

	// B16：尺寸/抽帧位置/JPEG 质量改为运行期读 settings（media.*），改完即时生效；
	// 配置缺失或越界时回退内置默认，不因站长配错而完全不生成缩略图。
	opt := service.ThumbOptionsFrom(a.cfg)
	if err := service.MakeThumb(ctx, src, tmpName, isVideo, service.ThumbSeekMS(info.DurationMS, opt.SeekMS), opt); err != nil {
		return "", false // 失败不落库：下次请求自然重试（坏文件每次快速失败，代价可接受）
	}
	out, err := os.Open(tmpName)
	if err != nil {
		return "", false
	}
	defer out.Close()
	st, err := out.Stat()
	if err != nil || st.Size() <= 0 {
		return "", false
	}
	key := service.DerivedKey(f.SpaceID, f.ID, "thumb.jpg")
	if err := a.files.PutDerived(ctx, key, out, st.Size()); err != nil {
		return "", false
	}
	info.ThumbnailRef = key
	if err := a.media.Upsert(ctx, info); err != nil {
		return "", false
	}
	return key, true
}

// serveThumb 直出缩略图（调用方须已完成鉴权与边界校验）。
//
// 门控口径（与 B14 契约一致）：能力关闭 = 端点下线（404）。这里先判门控再取图，
// 并给出可区分的原因码 —— 否则运维会把「能力关了」误读成「这个文件坏了」。
// 代价是关闭期间连已缓存的缩略图也不再返回；这是有意为之：面板说关就得真关，
// 否则「关闭态逐端点断言」这种验证手段就失效了。面板文案已同步写明该后果。
func (a *API) serveThumb(w http.ResponseWriter, r *http.Request, f *service.File, cache string) {
	if !a.thumbEnabled() {
		writeErr(w, http.StatusNotFound, "CAPABILITY_DISABLED", "媒体缩略图能力未启用")
		return
	}
	key, ok := a.ensureThumb(r.Context(), f)
	if !ok {
		writeErr(w, http.StatusNotFound, "THUMB_UNAVAILABLE", "该媒体暂无缩略图")
		return
	}
	rc, err := a.files.DerivedReader(r.Context(), key)
	if err != nil {
		writeErr(w, http.StatusNotFound, "THUMB_UNAVAILABLE", "该媒体暂无缩略图")
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", cache)
	_, _ = io.Copy(w, rc)
}

// fileMediaThumb GET /api/v1/files/{id}/thumb
// 登录态缩略图：不限博客子树（站长的文件管理器里任何图片/视频都应能预览）。
func (a *API) fileMediaThumb(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || len(id) > 64 {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "非法文件 ID")
		return
	}
	if a.media == nil {
		writeErr(w, http.StatusServiceUnavailable, "MEDIA_DISABLED", "媒体模块未启用")
		return
	}
	f, err := a.files.Get(r.Context(), id)
	if err != nil || f == nil || f.Kind != "file" {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	a.serveThumb(w, r, f, "private, max-age=86400")
}

// filesMediaBatch POST /api/v1/files/media/batch
// 请求 {"ids":[...]}（上限 100），只回**已缓存**的元信息，不触发探测/生成。
//
// 为什么不批量探测：一次列表请求若逐个 ffprobe，会在共享生产机上拉起上百个子进程。
// 未命中者原样出现在 missing 里，由前端按需（逐个、按可见性）再取 —— 这是有意的取舍，
// 与 B13「懒探测」同一立场。
func (a *API) filesMediaBatch(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单
	if !a.blogAuthorOnly(w, r) {
		return
	}
	if a.media == nil {
		writeErr(w, http.StatusServiceUnavailable, "MEDIA_DISABLED", "媒体模块未启用")
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_BODY", "请求体不是合法 JSON")
		return
	}
	const maxIDs = 100
	seen := make(map[string]bool, len(req.IDs))
	ids := make([]string, 0, len(req.IDs))
	for _, v := range req.IDs {
		v = strings.TrimSpace(v)
		if v == "" || len(v) > 64 || seen[v] {
			continue
		}
		seen[v] = true
		ids = append(ids, v)
	}
	if len(ids) > maxIDs {
		writeErr(w, http.StatusBadRequest, "TOO_MANY_IDS", "单次最多查询 100 个文件")
		return
	}
	got, err := a.media.ListByIDs(r.Context(), ids)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "MEDIA_QUERY_FAILED", err.Error())
		return
	}
	items := make(map[string]any, len(got))
	missing := make([]string, 0, len(ids))
	for _, id := range ids {
		m, ok := got[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		items[id] = map[string]any{
			"probe_status":  m.ProbeStatus,
			"width":         m.Width,
			"height":        m.Height,
			"duration_ms":   m.DurationMS,
			"codec":         m.Codec,
			"bitrate":       m.Bitrate,
			"has_thumbnail": m.ThumbnailRef != "",
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "missing": missing})
}

// cleanupMediaOrphans 清理「源文件已不在 files 表」的媒体记录，并顺手清掉其派生对象。
//
// 定位：**兜底对账**，不是 Purge 的主路径。Purge 自身已能清干净 —— 它在删行前收集
// (space, fileID) 并按前缀删派生对象（见 FileStore.Purge 的 B15 段），子表清扫里也含
// file_media。本函数覆盖的是「files 行以别的途径消失、file_media 行却留下」的情形
// （组织/空间级联删除、导入器回滚、手工改库等），代价只有一次 SQL。
//
// ⚠️ 纠正 B15 中途的一个错误判断：曾以为「MediaStore.PurgeFile 零调用点 → file_media 行
// 会永久残留」。实测不成立 —— file_media 自 B8 起就在 Purge 的子表清扫名单里。真正会残留的是
// **磁盘上的派生对象**，而它没法靠事后对账找回（行已随清扫消失），故改为在 Purge 内、删行前
// 收集。本函数保留为网状兜底。
//
// 口径：files 表中软删（回收站）的行仍然存在，故可恢复文件的媒体记录不会被误清。
//
// 失败一律安静：派生对象清不掉只影响磁盘占用，绝不能让「删除文件」这个主操作失败。
func (a *API) cleanupMediaOrphans(ctx context.Context) {
	if a.media == nil || a.files == nil {
		return
	}
	orphans, err := a.media.DeleteOrphans(ctx)
	if err != nil {
		return
	}
	for _, o := range orphans {
		prefix := service.DerivedPrefixOfKey(o.ThumbnailRef)
		if prefix == "" {
			continue
		}
		a.files.DeleteDerivedPrefix(ctx, prefix)
	}
}

// ---- 视频转码（B17）：触发 / 状态 / 取消 ----

// transcodeEnabled 视频转码能力门控（运行期判定，改完即时生效，见 service.CapabilityApply）。
func (a *API) transcodeEnabled() bool {
	on, _ := service.CapabilityEnabled(a.db, a.cfg, service.CapTranscode)
	return on
}

// fileTranscodeTrigger POST /api/v1/files/{id}/transcode
// 发起/重试视频转码（异步）。幂等：已在跑/已排队/已完成且参数未变 → 直接返回当前状态。
func (a *API) fileTranscodeTrigger(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单
	if !a.blogAuthorOnly(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || len(id) > 64 {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "非法文件 ID")
		return
	}
	if a.transcode == nil {
		writeErr(w, http.StatusServiceUnavailable, "TRANSCODE_DISABLED", "视频转码模块未启用")
		return
	}
	if !a.transcodeEnabled() {
		writeErr(w, http.StatusNotFound, "CAPABILITY_DISABLED", "视频转码能力未启用")
		return
	}
	f, err := a.files.Get(r.Context(), id)
	if err != nil || f == nil || f.Kind != "file" {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	job, err := a.transcode.Trigger(r.Context(), f)
	if err != nil {
		code := "TRANSCODE_FAILED"
		switch {
		case errors.Is(err, service.ErrTranscodeNotVideo):
			code = "NOT_VIDEO"
		case errors.Is(err, service.ErrTranscodeTooLong):
			code = "TOO_LONG"
		case errors.Is(err, service.ErrTranscodeNoLocal):
			code = "NO_LOCAL_FILE"
		case errors.Is(err, service.ErrTranscodeNoFFmpeg):
			code = "NO_FFMPEG"
		case errors.Is(err, service.ErrTranscodeNoManager):
			code = "DISABLED"
		}
		writeErr(w, http.StatusBadRequest, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// fileTranscodeStatus GET /api/v1/files/{id}/transcode
// 查询转码进度/状态：优先内存实时进度，否则回落落库态。
func (a *API) fileTranscodeStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || len(id) > 64 {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "非法文件 ID")
		return
	}
	if a.transcode == nil {
		writeErr(w, http.StatusServiceUnavailable, "TRANSCODE_DISABLED", "视频转码模块未启用")
		return
	}
	f, err := a.files.Get(r.Context(), id)
	if err != nil || f == nil || f.Kind != "file" {
		writeErr(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}
	job := a.transcode.Status(r.Context(), id)
	writeJSON(w, http.StatusOK, job)
}

// fileTranscodeCancel POST /api/v1/files/{id}/transcode/cancel
// 取消排队中或运行中的转码任务（运行中靠取消 context 终止 ffmpeg 子进程）。
func (a *API) fileTranscodeCancel(w http.ResponseWriter, r *http.Request) {
	// C3 修复：文件写操作仅 owner/admin/作者白名单
	if !a.blogAuthorOnly(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || len(id) > 64 {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "非法文件 ID")
		return
	}
	if a.transcode == nil {
		writeErr(w, http.StatusServiceUnavailable, "TRANSCODE_DISABLED", "视频转码模块未启用")
		return
	}
	ok := a.transcode.Cancel(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"canceled": ok})
}
