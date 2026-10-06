// shares_thumb_test.go B19：分享通道专属公开缩略图端点回归。
//
// 覆盖点：
//  1. file 分享：token 解析出 row.FileID，image/video → 200 JPEG（有 ffmpeg），非缩略图类型 → 404；
//  2. 不强制博客子树：空间根（非博客子树）的文件分享同样可达缩略图（与 publicMediaThumb 的关键差异）；
//  3. dir 分享：需 ?path= 经 collectDirFiles 解析；缺 path → 400、path 不存在 → 404；
//  4. mime 分派：svg / 音频 → 404 THUMB_UNAVAILABLE（与后端 thumbMimeOf 同口径）；
//  5. 鉴权：token 不存在/已撤销 → 404；无合法 token 即不可达（私密文件不经过此通道）。
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// callShareThumb 直接调分享缩略图 handler（httptest）。
func callShareThumb(t *testing.T, a *API, token, pathQ string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/v1/shares/" + token + "/thumb"
	if pathQ != "" {
		url += "?" + pathQ
	}
	r := httptest.NewRequest(http.MethodGet, url, nil)
	r.SetPathValue("token", token)
	w := httptest.NewRecorder()
	a.sharesThumb(w, r)
	return w
}

// insertShare 直插一条未撤销、未过期的分享（测试辅助）。
func insertShare(t *testing.T, a *API, scope, fileID, dirID, token string) {
	t.Helper()
	if _, err := a.db.ExecContext(context.Background(),
		`INSERT INTO shares (id, file_id, owner_id, token, permission, scope, dir_id, expires_at, created_at, revoked_at)
		 VALUES (?,?,?,?,?,?,?,0,?,0)`,
		newID(), fileID, service.SystemOwnerID, token, "view", scope, dirID, time.Now().UnixMilli()); err != nil {
		t.Fatalf("insert share: %v", err)
	}
}

// 1) file 分享：博客子树内的图片 → 200 JPEG（有 ffmpeg）。
func TestShareThumbFileImage(t *testing.T) {
	a := newThumbAPI(t)
	id := uploadAt(t, a, service.BlogDirID, "b19-file.png", "image/png", pngBytes(t, 40, 30))
	const token = "b19tokfileimg"
	insertShare(t, a, "file", id, "", token)

	if !service.FFmpegAvailable() {
		w := callShareThumb(t, a, token, "")
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "THUMB_UNAVAILABLE") {
			t.Fatalf("无 ffmpeg 时 code=%d body=%s, want 404 THUMB_UNAVAILABLE", w.Code, w.Body.String())
		}
		return
	}
	w := callShareThumb(t, a, token, "")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s, want 200", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("Content-Type=%q, want image/jpeg", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc == "" {
		t.Error("分享缩略图应带 Cache-Control")
	}
	body := w.Body.Bytes()
	if len(body) < 4 || body[0] != 0xFF || body[1] != 0xD8 {
		t.Fatalf("产出不是 JPEG（前 4 字节 % x）", body[:min(4, len(body))])
	}
}

// 2) 不强制博客子树：空间根（非博客子树）的图片 file 分享同样可达。
func TestShareThumbNonBlogSubtree(t *testing.T) {
	a := newThumbAPI(t)
	id := uploadAt(t, a, "", "b19-root.png", "image/png", pngBytes(t, 20, 20))
	const token = "b19tokroot"
	insertShare(t, a, "file", id, "", token)

	// 即便在博客子树外，凭合法 token 仍可达缩略图（与 publicMediaThumb 强制子树的关键差异）
	if !service.FFmpegAvailable() {
		w := callShareThumb(t, a, token, "")
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "THUMB_UNAVAILABLE") {
			t.Fatalf("无 ffmpeg 时 code=%d body=%s, want 404 THUMB_UNAVAILABLE", w.Code, w.Body.String())
		}
		return
	}
	if w := callShareThumb(t, a, token, ""); w.Code != http.StatusOK {
		t.Fatalf("非博客子树文件分享 code=%d body=%s, want 200", w.Code, w.Body.String())
	}
}

// 3) mime 分派：svg / 音频 file 分享 → 404 THUMB_UNAVAILABLE（有原文件但无缩略图概念）。
func TestShareThumbFileNonThumbable(t *testing.T) {
	a := newThumbAPI(t)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"></svg>`)
	svgID := uploadAt(t, a, service.BlogDirID, "b19-x.svg", "image/svg+xml", svg)
	audID := uploadAt(t, a, service.BlogDirID, "b19-a.mp3", "audio/mpeg", []byte("not-a-real-mp3"))
	insertShare(t, a, "file", svgID, "", "b19toksvg")
	insertShare(t, a, "file", audID, "", "b19tokaud")

	for token, wantCode := range map[string]int{"b19toksvg": http.StatusNotFound, "b19tokaud": http.StatusNotFound} {
		w := callShareThumb(t, a, token, "")
		if w.Code != wantCode || !strings.Contains(w.Body.String(), "THUMB_UNAVAILABLE") {
			t.Fatalf("token=%s code=%d body=%s, want 404 THUMB_UNAVAILABLE", token, w.Code, w.Body.String())
		}
	}
}

// 4) dir 分享：?path= 解析出文件 → 200（有 ffmpeg）；缺 path → 400；path 不存在 → 404。
func TestShareThumbDirResolve(t *testing.T) {
	a := newThumbAPI(t)
	dir, err := a.files.CreateDir(context.Background(), service.SystemOwnerID, service.SystemHomeSpaceID, "", "b19-dir", service.DefaultSiteID)
	if err != nil {
		t.Fatalf("create dir: %v", err)
	}
	imgID := uploadAt(t, a, dir.ID, "b19-dir.png", "image/png", pngBytes(t, 32, 24))
	// 再放一个非缩略图文件，验证仅 image/video 出图
	_ = uploadAt(t, a, dir.ID, "b19-dir.txt", "text/plain", []byte("hello"))
	const token = "b19tokdir"
	insertShare(t, a, "dir", dir.ID, dir.ID, token)

	// 缺 path → 400
	if w := callShareThumb(t, a, token, ""); w.Code != http.StatusBadRequest ||
		!strings.Contains(w.Body.String(), "SHARE_PATH_REQUIRED") {
		t.Fatalf("缺 path code=%d body=%s, want 400 SHARE_PATH_REQUIRED", w.Code, w.Body.String())
	}
	// path 不存在 → 404
	if w := callShareThumb(t, a, token, "path="+url.QueryEscape("nope.png")); w.Code != http.StatusNotFound ||
		!strings.Contains(w.Body.String(), "SHARE_PATH_NOT_FOUND") {
		t.Fatalf("坏 path code=%d body=%s, want 404 SHARE_PATH_NOT_FOUND", w.Code, w.Body.String())
	}
	// 文本文件 → 404 THUMB_UNAVAILABLE（即便在分享目录内）
	if w := callShareThumb(t, a, token, "path="+url.QueryEscape("b19-dir.txt")); w.Code != http.StatusNotFound ||
		!strings.Contains(w.Body.String(), "THUMB_UNAVAILABLE") {
		t.Fatalf("文本文件 code=%d body=%s, want 404 THUMB_UNAVAILABLE", w.Code, w.Body.String())
	}
	// 图片文件 → 200（有 ffmpeg）
	if service.FFmpegAvailable() {
		w := callShareThumb(t, a, token, "path="+url.QueryEscape("b19-dir.png"))
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/jpeg" {
			t.Fatalf("dir 图片 code=%d body=%s, want 200 image/jpeg", w.Code, w.Body.String())
		}
	}
	// 守卫：确认解析到的确是 dir 内的文件
	if _, err := a.files.Get(context.Background(), imgID); err != nil {
		t.Fatalf("dir 内文件应存在: %v", err)
	}
}

// 5) 鉴权：token 不存在 → 404；已撤销 → 404（私密文件不经过此通道）。
func TestShareThumbAuth(t *testing.T) {
	a := newThumbAPI(t)
	id := uploadAt(t, a, service.BlogDirID, "b19-auth.png", "image/png", pngBytes(t, 8, 8))
	const token = "b19tokauth"
	insertShare(t, a, "file", id, "", token)

	// 不存在的 token → 404
	if w := callShareThumb(t, a, "does-not-exist", ""); w.Code != http.StatusNotFound ||
		!strings.Contains(w.Body.String(), "SHARE_NOT_FOUND") {
		t.Fatalf("坏 token code=%d body=%s, want 404 SHARE_NOT_FOUND", w.Code, w.Body.String())
	}
	// 撤销后 → 404
	if _, err := a.db.ExecContext(context.Background(),
		`UPDATE shares SET revoked_at = ? WHERE token = ?`, time.Now().UnixMilli(), token); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if w := callShareThumb(t, a, token, ""); w.Code != http.StatusNotFound ||
		!strings.Contains(w.Body.String(), "SHARE_NOT_FOUND") {
		t.Fatalf("已撤销 code=%d body=%s, want 404 SHARE_NOT_FOUND", w.Code, w.Body.String())
	}
}
