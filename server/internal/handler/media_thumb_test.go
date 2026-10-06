// media_thumb_test.go 缩略图端点与批量媒体信息（B15）回归。
//
// 覆盖点：
//  1. mime 分派：svg 排除、音频不生成（有原图不代表有缩略图）；
//  2. 公开端点边界：非法 id 400、不存在 404、越界（非博客子树）404、音频 404；
//  3. 能力门控：capability.thumb=false 时**不生成也不落库**（且改完即时生效，无需重启）；
//  4. 真机链路（有 ffmpeg 时才跑）：出图是合法 JPEG、thumbnail_ref 落库、派生物真实存在、
//     二次请求复用缓存，且**重探不会把已生成的引用清空**（B15 修掉的隐患）；
//  5. 批量端点：上限 100、去重、坏请求体 400、只回缓存（不触发探测）；
//  6. 清理耦合：文件 Purge 后媒体行与派生对象都应消失（PurgeFile 此前零调用点的缺陷）。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/config"
	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
	"github.com/AiKMAP/AiKmap/server/internal/service"
	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

// asOwner 注入登录身份（ctxUserID），模拟 authMiddleware。
// C3 修复后文件写端点（媒体批量/转码/标签/删除/彻底删除等）走 blogAuthorOnly，
// 本包测试是"直调 handler"形态、不过中间件，故需自己补上身份，否则一律 403。
func asOwner(a *API, r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxUserID, a.homeOwnerID()))
}

// newThumbAPI 在 newMediaAPI 的基础上注入 cfg：门控判定需要它（capability.thumb）。
func newThumbAPI(t *testing.T) *API {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "thumb.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	b := bus.New()
	cfg := config.New(db, b)
	aud := service.New(db)
	ctx := context.Background()
	if err := service.EnsureSystem(ctx, db, aud); err != nil {
		t.Fatalf("ensure system: %v", err)
	}
	if err := service.EnsureBlogSpace(ctx, db); err != nil {
		t.Fatalf("ensure blog space: %v", err)
	}
	st, err := storage.NewLocal(filepath.Join(dir, "files"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	return &API{
		db: db, cfg: cfg, b: b, aud: aud,
		files:  service.NewFileStore(db, st, b, aud),
		notify: service.NewNotifyStore(db),
		media:  service.NewMediaStore(db),
	}
}

// callThumb 直接调缩略图 handler（httptest）。
func callThumb(t *testing.T, h http.HandlerFunc, id string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/public/media/"+id+"/thumb", nil)
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

// 1) 缩略图 mime 分派：只认 image/*（除 svg）与 video/*；音频不生成。
func TestThumbMimeDispatch(t *testing.T) {
	cases := []struct {
		mime    string
		isVideo bool
		ok      bool
	}{
		{"image/png", false, true},
		{"image/jpeg; charset=binary", false, true},
		{"video/mp4", true, true},
		{"video/webm; charset=binary", true, true},
		{"image/svg+xml", false, false}, // 活性文档，不解码
		{"audio/mpeg", false, false},    // 音频无视觉缩略图
		{"text/markdown", false, false},
		{"application/pdf", false, false},
		{"", false, false},
	}
	for _, c := range cases {
		isVideo, ok := thumbMimeOf(c.mime)
		if ok != c.ok || (ok && isVideo != c.isVideo) {
			t.Errorf("thumbMimeOf(%q) = (%v,%v), want (%v,%v)", c.mime, isVideo, ok, c.isVideo, c.ok)
		}
	}
}

// 2) 公开缩略图边界：不泄漏、不越界、不把「没缩略图」当 500。
func TestPublicMediaThumbBoundaries(t *testing.T) {
	a := newThumbAPI(t)
	ctx := context.Background()

	// 非法 id → 400
	for _, bad := range []string{"", "short", "../../etc/passwd", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"} {
		if w := callThumb(t, a.publicMediaThumb, bad); w.Code != http.StatusBadRequest {
			t.Errorf("id=%q code=%d, want 400", bad, w.Code)
		}
	}

	// 合法形态但不存在的 id → 404
	missing := strings.Repeat("a", 32)
	if w := callThumb(t, a.publicMediaThumb, missing); w.Code != http.StatusNotFound {
		t.Errorf("不存在的 id code=%d, want 404", w.Code)
	}

	// 博客子树外（空间根）→ 404 NOT_PUBLIC_MEDIA（与 publicMediaGet 同边界）
	outside := uploadAt(t, a, "", "b15-outside.png", "image/png", pngBytes(t, 4, 4))
	if w := callThumb(t, a.publicMediaThumb, outside); w.Code != http.StatusNotFound ||
		!strings.Contains(w.Body.String(), "NOT_PUBLIC_MEDIA") {
		t.Errorf("越界文件 code=%d body=%s, want 404 NOT_PUBLIC_MEDIA", w.Code, w.Body.String())
	}

	// 子树内 svg → 404（公开媒体口径本身就不放行 svg）
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"></svg>`)
	svgID := uploadAt(t, a, service.BlogDirID, "b15-x.svg", "image/svg+xml", svg)
	if w := callThumb(t, a.publicMediaThumb, svgID); w.Code != http.StatusNotFound {
		t.Errorf("svg code=%d, want 404", w.Code)
	}

	// 子树内音频 → 404 THUMB_UNAVAILABLE（有原文件但无缩略图概念）
	audID := uploadAt(t, a, service.BlogDirID, "b15-a.mp3", "audio/mpeg", []byte("not-a-real-mp3"))
	w := callThumb(t, a.publicMediaThumb, audID)
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "THUMB_UNAVAILABLE") {
		t.Errorf("音频 code=%d body=%s, want 404 THUMB_UNAVAILABLE", w.Code, w.Body.String())
	}
	// 音频也不应因此留下媒体行
	if _, ok := a.media.Get(ctx, audID); ok {
		t.Error("音频不该产生 file_media 行")
	}
}

// 3) 能力门控：关闭后**端点整体下线**（404 CAPABILITY_DISABLED），改完立即生效（运行期开关）。
//
// 🔴 关键回归点：关闭态必须连**已缓存的**缩略图也挡住。曾经的实现把「缓存命中」放在门控
// 之前，于是面板写着「已关闭」而端点照常 200 —— 判定与实现漂移，正是 B14 那类教训。
// 所以这里先在开启态生成一份缓存，再关闭，断言仍是 404。
func TestThumbGateOffBlocksGeneration(t *testing.T) {
	a := newThumbAPI(t)
	ctx := context.Background()
	id := uploadAt(t, a, service.BlogDirID, "b15-gated.png", "image/png", pngBytes(t, 40, 30))

	// 造出缓存（无 ffmpeg 的机器上跳过这一步，后面的 404 断言仍然有效）
	if service.FFmpegAvailable() {
		if w := callThumb(t, a.publicMediaThumb, id); w.Code == http.StatusOK {
			if info, ok := a.media.Get(ctx, id); !ok || info.ThumbnailRef == "" {
				t.Fatal("开启态应已生成并落库缩略图引用")
			}
		}
	}

	if code, body := putSetting(t, a, "capability.thumb", "false"); code != http.StatusOK {
		t.Fatalf("关闭 capability.thumb 失败 code=%d body=%s", code, body)
	}
	if a.thumbEnabled() {
		t.Fatal("写库后门控应立刻为关闭（运行期判定）")
	}
	// 两个端点都要下线；原因码必须能与「这个文件没有缩略图」区分（否则运维会误判为文件坏了）
	for name, h := range map[string]http.HandlerFunc{"public": a.publicMediaThumb, "authed": a.fileMediaThumb} {
		w := callThumb(t, h, id)
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "CAPABILITY_DISABLED") {
			t.Fatalf("%s 端点门控关闭时 code=%d body=%s, want 404 CAPABILITY_DISABLED", name, w.Code, w.Body.String())
		}
	}

	// 重新打开 → 恢复生成能力（同一进程内，证明是运行期闸门而非装配期）
	if code, body := putSetting(t, a, "capability.thumb", "true"); code != http.StatusOK {
		t.Fatalf("打开 capability.thumb 失败 code=%d body=%s", code, body)
	}
	if !a.thumbEnabled() {
		t.Fatal("打开后门控应立刻为开启")
	}
	if w := callThumb(t, a.publicMediaThumb, id); w.Code != http.StatusOK && service.FFmpegAvailable() {
		t.Fatalf("重新打开后 code=%d body=%s, want 200", w.Code, w.Body.String())
	}
}

// 4) 真机链路：需要宿主机有 ffmpeg，没有就跳过（不把环境依赖伪装成失败）。
func TestThumbGeneratedForBlogImage(t *testing.T) {
	if !service.FFmpegAvailable() {
		t.Skip("宿主机无 ffmpeg，跳过真实生成断言")
	}
	a := newThumbAPI(t)
	ctx := context.Background()
	id := uploadAt(t, a, service.BlogDirID, "b15-gen.png", "image/png", pngBytes(t, 40, 30))

	w := callThumb(t, a.publicMediaThumb, id)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s, want 200", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("Content-Type=%q, want image/jpeg", ct)
	}
	if w.Header().Get("Cache-Control") == "" {
		t.Error("公开缩略图应带 Cache-Control（避免列表页反复请求）")
	}
	body := w.Body.Bytes()
	if len(body) < 4 || body[0] != 0xFF || body[1] != 0xD8 {
		t.Fatalf("产出不是 JPEG（前 4 字节 % x）", body[:min(4, len(body))])
	}

	info, ok := a.media.Get(ctx, id)
	if !ok || info.ThumbnailRef == "" {
		t.Fatalf("缩略图引用应落库，实得 ok=%v info=%+v", ok, info)
	}
	if _, exists := a.files.DerivedStat(ctx, info.ThumbnailRef); !exists {
		t.Fatalf("派生对象不存在：%s", info.ThumbnailRef)
	}
	if !strings.HasPrefix(info.ThumbnailRef, service.DerivedSpacePrefix(mustFile(t, a, id).SpaceID)) {
		t.Fatalf("缩略图引用应位于本空间的派生前缀下，实得 %s", info.ThumbnailRef)
	}

	// 二次请求：命中缓存（引用不变），不重复生成
	first := info.ThumbnailRef
	if w2 := callThumb(t, a.publicMediaThumb, id); w2.Code != http.StatusOK {
		t.Fatalf("二次请求 code=%d", w2.Code)
	}
	again, _ := a.media.Get(ctx, id)
	if again == nil || again.ThumbnailRef != first {
		t.Fatalf("二次请求不应改变引用：%v -> %v", first, again)
	}

	// 🔴 重探不得清空已生成的缩略图引用：把探测状态改成 unsupported 模拟「环境变化后重探」，
	// 再请求 /files/{id}/media，引用必须仍在（这正是 B15 修掉的隐患）。
	if _, err := a.db.ExecContext(ctx, `UPDATE file_media SET probe_status='unsupported' WHERE file_id=?`, id); err != nil {
		t.Fatalf("准备重探场景失败：%v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+id+"/media", nil)
	r.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	a.fileMediaGet(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("fileMedia code=%d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got["thumbnail_ref"] != first {
		t.Fatalf("重探后 thumbnail_ref 被清空/改写：%v, want %s", got["thumbnail_ref"], first)
	}
}

// 5) 批量媒体信息：上限、去重、坏体 400、只回缓存。
func TestFilesMediaBatchShape(t *testing.T) {
	a := newThumbAPI(t)
	ctx := context.Background()

	// 坏请求体 → 400
	r := httptest.NewRequest(http.MethodPost, "/api/v1/files/media/batch", strings.NewReader("{not-json"))
	w := httptest.NewRecorder()
	a.filesMediaBatch(w, asOwner(a, r))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("坏体 code=%d, want 400", w.Code)
	}

	// 超过上限 → 400（注意：id 必须**互不相同**，否则会被去重压到上限之下，
	// 这条断言就变成「测了去重、没测上限」—— 初版即踩此坑）
	ids := make([]string, 0, 101)
	for i := 0; i < 101; i++ {
		ids = append(ids, fmt.Sprintf("%032x", i))
	}
	payload, _ := json.Marshal(map[string]any{"ids": ids})
	r = httptest.NewRequest(http.MethodPost, "/api/v1/files/media/batch", bytes.NewReader(payload))
	w = httptest.NewRecorder()
	a.filesMediaBatch(w, asOwner(a, r))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "TOO_MANY_IDS") {
		t.Fatalf("超限 code=%d body=%s, want 400 TOO_MANY_IDS", w.Code, w.Body.String())
	}

	// 正常：一个已缓存（先探测过）+ 一个未缓存 + 一个重复
	cached := uploadAt(t, a, service.BlogDirID, "b15-batch.png", "image/png", pngBytes(t, 6, 4))
	if _, err := a.ensureMediaInfo(ctx, cached, mustFile(t, a, cached)); err != nil {
		t.Fatalf("ensureMediaInfo: %v", err)
	}
	unknown := strings.Repeat("b", 32)
	payload, _ = json.Marshal(map[string]any{"ids": []string{cached, unknown, cached, "  "}})
	r = httptest.NewRequest(http.MethodPost, "/api/v1/files/media/batch", bytes.NewReader(payload))
	w = httptest.NewRecorder()
	a.filesMediaBatch(w, asOwner(a, r))
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		Items   map[string]map[string]any `json:"items"`
		Missing []string                  `json:"missing"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	it, ok := out.Items[cached]
	if !ok {
		t.Fatalf("已缓存项应出现在 items：%s", w.Body.String())
	}
	if it["width"] != float64(6) || it["height"] != float64(4) {
		t.Fatalf("宽高 = %vx%v, want 6x4", it["width"], it["height"])
	}
	if it["has_thumbnail"] != false {
		t.Error("仅探测过（未请求缩略图）时 has_thumbnail 应为 false")
	}
	if len(out.Missing) != 1 || out.Missing[0] != unknown {
		t.Fatalf("missing=%v, want 仅 %s（去重后）", out.Missing, unknown)
	}
	// 批量端点是「只读缓存」：未命中者不得因批量查询被写入 media 行
	if _, ok := a.media.Get(ctx, unknown); ok {
		t.Error("批量查询不应触发探测并写库")
	}
}

// 6) 清理耦合：Purge 后 file_media 行与派生对象都必须消失。
// ⚠️ 这条用例的价值在**派生对象**那一半：file_media 行自 B8 起就被 Purge 的子表清扫删掉，
// 而派生对象与 files 行没有外键关系、清扫覆不到 —— 一度真的会残留（B15 实测踩到，于是把
// 收集点挪进 Purge、删行之前）。故两半断言都要留：只查行会放过真正的洞，只查派生对象
// 又会漏掉「行没清」的回归。
func TestPurgeCleansMediaRowAndDerived(t *testing.T) {
	a := newThumbAPI(t)
	ctx := context.Background()
	id := uploadAt(t, a, service.BlogDirID, "b15-purge.png", "image/png", pngBytes(t, 20, 20))

	// 先确保有媒体行（无 ffmpeg 时也能有：图片走 header 探测）
	if _, err := a.ensureMediaInfo(ctx, id, mustFile(t, a, id)); err != nil {
		t.Fatalf("ensureMediaInfo: %v", err)
	}
	var thumbRef string
	if info, ok := a.media.Get(ctx, id); ok {
		thumbRef = info.ThumbnailRef
	}
	// 有 ffmpeg 时顺手生成一张，覆盖「派生物被一并清掉」这一路径
	if service.FFmpegAvailable() && a.thumbEnabled() {
		_ = callThumb(t, a.publicMediaThumb, id)
		if info, ok := a.media.Get(ctx, id); ok {
			thumbRef = info.ThumbnailRef
		}
	}

	// 走真实删除链路：软删 → 彻底删除（handler 内部会做孤儿对账）
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+id, nil)
	r.SetPathValue("id", id)
	a.filesDelete(httptest.NewRecorder(), asOwner(a, r))
	r = httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+id+"/purge", nil)
	r.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	a.filesPurge(rec, asOwner(a, r))
	if rec.Code != http.StatusOK {
		t.Fatalf("purge code=%d body=%s", rec.Code, rec.Body.String())
	}

	if _, ok := a.media.Get(ctx, id); ok {
		t.Fatal("Purge 后 file_media 行应被清理（此前是零调用点的缺陷）")
	}
	if thumbRef != "" {
		if _, exists := a.files.DerivedStat(ctx, thumbRef); exists {
			t.Fatalf("Purge 后派生对象应被清理：%s", thumbRef)
		}
	}
}

// mustFile 取文件行（测试辅助）。
func mustFile(t *testing.T, a *API, id string) *service.File {
	t.Helper()
	f, err := a.files.Get(context.Background(), id)
	if err != nil || f == nil {
		t.Fatalf("取文件失败 id=%s: %v", id, err)
	}
	return f
}
