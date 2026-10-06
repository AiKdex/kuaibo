// media_test.go 媒体元信息端点（B8 / B13）回归。
//
// 覆盖点：
//  1. 纯函数边界：公开媒体 mime 放行规则（含 svg 排除、带 charset 参数）与 kind 归一；
//  2. HTTP 边界：非法 id 400、非媒体 mime 404、越界（非博客子树）404；
//  3. 真机链路（本地 FileStore + 临时库）：博客子树内图片 → 匿名公开端点可读，
//     且探测结果（宽高）真实落库；音视频在无 ffprobe 环境下**不报 500**而是标 unsupported。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
	"github.com/AiKMAP/AiKmap/server/internal/service"
	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

// newMediaAPI 建一个「能真读文件、真落库」的测试环境（本地存储 + 临时库）。
func newMediaAPI(t *testing.T) *API {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	b := bus.New()
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
		db: db, b: b, aud: aud,
		files:  service.NewFileStore(db, st, b, aud),
		notify: service.NewNotifyStore(db),
		media:  service.NewMediaStore(db),
	}
}

// pngBytes 生成一张 w×h 的真 PNG（用于真实 header 探测）。
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func uploadAt(t *testing.T, a *API, parent, name, mime string, body []byte) string {
	t.Helper()
	f, err := a.files.Upload(context.Background(), service.SystemOwnerID, service.SystemHomeSpaceID,
		parent, name, mime, bytes.NewReader(body), int64(len(body)), service.DefaultSiteID)
	if err != nil {
		t.Fatalf("upload %s: %v", name, err)
	}
	return f.ID
}

func callPath(t *testing.T, h http.HandlerFunc, id string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/public/media/"+id+"/info", nil)
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

// TestPublicMediaMimeRules 放行规则（svg 必须排除；带 charset 参数仍要认得）。
func TestPublicMediaMimeRules(t *testing.T) {
	cases := []struct {
		mime string
		want bool
	}{
		{"image/png", true},
		{"image/jpeg; charset=binary", true},
		{"video/mp4", true},
		{"audio/mpeg", true},
		{"image/svg+xml", false}, // 活性文档 → 公开直出等于存储型 XSS
		{"text/markdown", false},
		{"application/pdf", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isPublicMediaMime(c.mime); got != c.want {
			t.Errorf("isPublicMediaMime(%q) = %v, want %v", c.mime, got, c.want)
		}
	}
	kinds := map[string]string{"image/png": "image", "video/mp4": "video", "audio/mpeg": "audio",
		"image/webp; charset=binary": "image", "text/plain": ""}
	for m, want := range kinds {
		if got := mediaKindOf(m); got != want {
			t.Errorf("mediaKindOf(%q) = %q, want %q", m, got, want)
		}
	}
}

// TestPublicMediaInfoBadID 非法 id 一律 400（防路径注入的第一道闸）。
func TestPublicMediaInfoBadID(t *testing.T) {
	a := newMediaAPI(t)
	for _, bad := range []string{"", "short", "../../etc/passwd", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/public/media/x/info", nil)
		r.SetPathValue("id", bad)
		w := httptest.NewRecorder()
		a.publicMediaInfoGet(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("id=%q code=%d, want 400", bad, w.Code)
		}
	}
}

// TestPublicMediaInfoBlogImage 博客子树内的图片：匿名可读 + 宽高真实探测。
func TestPublicMediaInfoBlogImage(t *testing.T) {
	a := newMediaAPI(t)
	id := uploadAt(t, a, service.BlogDirID, "b13-cover.png", "image/png", pngBytes(t, 7, 5))

	w := callPath(t, a.publicMediaInfoGet, id)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s, want 200", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got["probe_status"] != "ok" || got["kind"] != "image" {
		t.Fatalf("probe_status=%v kind=%v, want ok/image", got["probe_status"], got["kind"])
	}
	if got["width"] != float64(7) || got["height"] != float64(5) {
		t.Fatalf("宽高 = %vx%v, want 7x5", got["width"], got["height"])
	}
	// 公开响应不得泄漏内部引用字段
	if _, bad := got["thumbnail_ref"]; bad {
		t.Fatal("公开响应不应包含 thumbnail_ref")
	}
}

// TestPublicMediaInfoExternalFile404 非博客子树（空间根）的文件：即便 mime 合法也 404。
func TestPublicMediaInfoExternalFile404(t *testing.T) {
	a := newMediaAPI(t)
	id := uploadAt(t, a, "", "b13-outside.png", "image/png", pngBytes(t, 3, 3))
	if w := callPath(t, a.publicMediaInfoGet, id); w.Code != http.StatusNotFound {
		t.Fatalf("越界文件 code=%d, want 404", w.Code)
	}
}

// TestPublicMediaInfoSVGRejected 博客子树内的 svg 也必须 404（活性文档边界）。
func TestPublicMediaInfoSVGRejected(t *testing.T) {
	a := newMediaAPI(t)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"></svg>`)
	id := uploadAt(t, a, service.BlogDirID, "b13-x.svg", "image/svg+xml", svg)
	if w := callPath(t, a.publicMediaInfoGet, id); w.Code != http.StatusNotFound {
		t.Fatalf("svg code=%d, want 404", w.Code)
	}
}

// TestFileMediaVideoNever500 音视频探测无论环境如何都不得 500：
// 有 ffprobe → ok；没有 → unsupported（安静降级，绝不把 500 抛给用户）。
func TestFileMediaVideoNever500(t *testing.T) {
	a := newMediaAPI(t)
	// 内容不是真 mp4（只验「不炸」，真样本由 service 侧真机测试覆盖）
	id := uploadAt(t, a, service.BlogDirID, "b13-clip.mp4", "video/mp4", []byte("not-a-real-mp4"))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+id+"/media", nil)
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	a.fileMediaGet(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s, want 200（探测失败只降级、不报错）", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	switch got["probe_status"] {
	case "ok", "unsupported":
	default:
		t.Fatalf("probe_status=%v, want ok|unsupported", got["probe_status"])
	}
}
