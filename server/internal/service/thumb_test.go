// thumb_test.go 缩略图生成（B15）单元测试：只覆盖**可确定**的部分。
//
// 为什么不在这里跑真 ffmpeg：真机链路（能生成、是合法 JPEG、落库、可复用）放在
// handler 侧带真实 FileStore 的测试里，并且**无 ffmpeg 时 Skip** —— 单元测试不该依赖
// 宿主机装了什么。这里锁定的是三件不能退化的事：
//  1. 抽帧位置计算（短片不抽到空白尾帧、长片不抽到中段）；
//  2. 缩放表达式**只缩不放**（放大只会更糊且更大，是缩略图最常见的退化）；
//  3. ffmpeg 缺失必须返回可识别的 ErrThumbNoFFmpeg（调用方据此安静降级、不写坏状态）。
package service

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

// 1) 抽帧位置：0/负值 → 0；常规取 1/10；长片夹在 1s。
func TestThumbSeekMS(t *testing.T) {
	cases := []struct {
		dur  int64
		want int64
	}{
		{0, 0},
		{-100, 0},
		{1000, 100},     // 1s 短片 → 100ms（不抽到末尾空白帧）
		{5000, 500},     // 5s → 500ms
		{10000, 1000},   // 10s → 刚好 1s
		{600000, 1000},  // 10min → 夹到 1s，不抽中段
		{3600000, 1000}, // 1h → 同样是 1s
	}
	for _, c := range cases {
		if got := ThumbSeekMS(c.dur, DefaultThumbSeekMS); got != c.want {
			t.Errorf("ThumbSeekMS(%d, cap) = %d, want %d", c.dur, got, c.want)
		}
	}
}

// 2) 缩放表达式必须只缩不放（min(W, iw)）；高度用 -2 让 ffmpeg 按比例算并对齐偶数。
func TestThumbScaleFilterNoUpscale(t *testing.T) {
	f := thumbScaleFilter(DefaultThumbMaxEdge)
	if !strings.Contains(f, "min(480,iw)") {
		t.Fatalf("缩放表达式应含 min(480,iw)（防小图被放大），实得 %q", f)
	}
	if !strings.HasSuffix(f, ":-2") {
		t.Fatalf("高度应为 -2（按比例自动 + 偶数对齐），实得 %q", f)
	}
	if !strings.Contains(f, "'") {
		t.Fatalf("表达式内的逗号须由 ffmpeg 单引号保护，实得 %q", f)
	}
}

// 3) 二进制缺失 → 可识别的降级错误，而不是普通 error（调用方靠 errors.Is 分流）。
func TestMakeThumbWithoutFFmpegDegrades(t *testing.T) {
	old := ffmpegBin
	ffmpegBin = "aiklog-test-no-such-ffmpeg-binary"
	t.Cleanup(func() { ffmpegBin = old })

	if FFmpegAvailable() {
		t.Fatal("注入不存在的二进制后 FFmpegAvailable 应为 false")
	}
	err := MakeThumb(context.Background(), "in.jpg", "out.jpg", false, 0, DefaultThumbOptions())
	if !errors.Is(err, ErrThumbNoFFmpeg) {
		t.Fatalf("应返回 ErrThumbNoFFmpeg，实得 %v", err)
	}
}

// 4) 空路径在查二进制之前就被拒（不产生无意义的外部进程调用机会）。
func TestMakeThumbRejectsEmptyPaths(t *testing.T) {
	old := ffmpegBin
	ffmpegBin = "aiklog-test-no-such-ffmpeg-binary"
	t.Cleanup(func() { ffmpegBin = old })
	for _, c := range [][2]string{{"", "out.jpg"}, {"in.jpg", ""}, {"  ", "out.jpg"}} {
		if err := MakeThumb(context.Background(), c[0], c[1], false, 0, DefaultThumbOptions()); err == nil {
			t.Fatalf("src=%q dst=%q 应报错", c[0], c[1])
		} else if errors.Is(err, ErrThumbNoFFmpeg) {
			t.Fatalf("空路径应在查二进制前被拒，实得 %v", err)
		}
	}
}

// 5) 由派生对象 key 反推前缀：孤儿清理时源文件行已不存在，只能靠 key 反推。
func TestDerivedPrefixOfKey(t *testing.T) {
	cases := []struct{ key, want string }{
		{"spaces/S1/.derived/F1/thumb.jpg", "spaces/S1/.derived/F1"},
		{"thumb.jpg", "thumb.jpg"}, // 无分隔符：原样返回，不 panic
		{"/thumb.jpg", "/thumb.jpg"},
	}
	for _, c := range cases {
		if got := DerivedPrefixOfKey(c.key); got != c.want {
			t.Errorf("DerivedPrefixOfKey(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}

// 6) key 拼装形态固定：spaces/{spaceID}/.derived/{fileID}/...
// 改动这里等于改动存储布局（历史派生对象会变孤儿），故用测试钉死。
func TestDerivedKeyLayout(t *testing.T) {
	if got := DerivedKey("S1", "F1", "thumb.jpg"); got != "spaces/S1/.derived/F1/thumb.jpg" {
		t.Fatalf("派生对象 key 布局变了：%q", got)
	}
	if got := DerivedPrefix("S1", "F1"); got != "spaces/S1/.derived/F1" {
		t.Fatalf("派生对象前缀变了：%q", got)
	}
	if got := DerivedSpacePrefix("S1"); got != "spaces/S1/.derived" {
		t.Fatalf("空间派生前缀变了：%q", got)
	}
}

// 7) 派生对象的写入/读取/按前缀清理（隔离验证，不经过 handler）。
func TestDerivedObjectRoundTripAndPrefixDelete(t *testing.T) {
	st, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	s := NewFileStore(nil, st, nil, nil)
	ctx := context.Background()
	key := DerivedKey("S1", "F1", "thumb.jpg")
	if err := s.PutDerived(ctx, key, strings.NewReader("jpegbytes"), 9); err != nil {
		t.Fatalf("PutDerived: %v", err)
	}
	if _, ok := s.DerivedStat(ctx, key); !ok {
		t.Fatal("写入后应能 Stat 到派生对象")
	}
	rc, err := s.DerivedReader(ctx, key)
	if err != nil {
		t.Fatalf("DerivedReader: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "jpegbytes" {
		t.Fatalf("派生对象内容不一致: %q", got)
	}
	if prefix := DerivedPrefixOfKey(key); prefix != DerivedPrefix("S1", "F1") {
		t.Fatalf("前缀反推不一致: %q", prefix)
	}
	n := s.DeleteDerivedPrefix(ctx, DerivedPrefix("S1", "F1"))
	if n != 1 {
		t.Fatalf("按前缀应删除 1 个对象，实得 %d", n)
	}
	if _, ok := s.DerivedStat(ctx, key); ok {
		t.Fatal("按前缀删除后对象应消失")
	}
	// 幂等：再删一次不得报错（清理路径随时可能被重复触发）
	if n2 := s.DeleteDerivedPrefix(ctx, DerivedPrefix("S1", "F1")); n2 != 0 {
		t.Fatalf("重复清理应删 0 个，实得 %d", n2)
	}
}

// 9) B16 参数化：配置读取（空串=未配置回退默认）、越界夹取、非数字回退。
func TestThumbOptionsFromConfig(t *testing.T) {
	// 缺失键 → 全默认
	if got := ThumbOptionsFrom(fakeCfg{}); got != DefaultThumbOptions() {
		t.Fatalf("缺失键应回退默认，实得 %+v", got)
	}
	// 显式配置 → 生效（config.Store 会把 int 值 fmt.Sprint 成字符串）
	got := ThumbOptionsFrom(fakeCfg{
		KeyThumbMaxEdge: "320", KeyThumbSeekMS: "2500", KeyThumbQuality: "8",
	})
	if got.MaxEdge != 320 || got.SeekMS != 2500 || got.Quality != 8 {
		t.Fatalf("配置未生效：%+v", got)
	}
	// 越界 → 夹取（不报错：宁可按边界生成，也不要整站没缩略图）
	got = ThumbOptionsFrom(fakeCfg{
		KeyThumbMaxEdge: "99999", KeyThumbSeekMS: "999999", KeyThumbQuality: "1",
	})
	if got.MaxEdge != ThumbMaxEdgeMax || got.SeekMS != ThumbSeekMSMax || got.Quality != ThumbQualityMin {
		t.Fatalf("越界未夹取：%+v", got)
	}
	// 小值（>0 但低于下界）→ 夹到下界：站长配 1 的意图是「要小图」，回退 480 等于把它配的值吃掉；
	// 只有 <=0（无意义值）才回退默认。
	got = ThumbOptionsFrom(fakeCfg{KeyThumbMaxEdge: "1", KeyThumbQuality: "-3"})
	if got.MaxEdge != ThumbMaxEdgeMin || got.Quality != DefaultThumbQuality {
		t.Fatalf("小值应夹到下界、非正数才回退默认：%+v", got)
	}
	// 非数字 → 回退默认
	if got := ThumbOptionsFrom(fakeCfg{KeyThumbMaxEdge: "abc"}); got.MaxEdge != DefaultThumbMaxEdge {
		t.Fatalf("非数字应回退默认，实得 %d", got.MaxEdge)
	}
	// 🔴 seek=0 是**合法值**（从第 0 帧抽），不得被当成「未配置」回退成 1000
	if got := ThumbOptionsFrom(fakeCfg{KeyThumbSeekMS: "0"}); got.SeekMS != 0 {
		t.Fatalf("seek=0 应被接受，实得 %d", got.SeekMS)
	}
	// 空串 = 未配置（与 capability.* 同口径）
	if got := ThumbOptionsFrom(fakeCfg{KeyThumbMaxEdge: ""}); got.MaxEdge != DefaultThumbMaxEdge {
		t.Fatalf("空串应视为未配置，实得 %d", got.MaxEdge)
	}
	// nil 配置 → 默认
	if got := ThumbOptionsFrom(nil); got != DefaultThumbOptions() {
		t.Fatalf("nil cfg 应回退默认，实得 %+v", got)
	}
}

// 10) 缩放表达式必须使用**传入的**长边上限（配置改了要真的作用到 ffmpeg 参数上）。
func TestThumbScaleFilterUsesConfiguredEdge(t *testing.T) {
	if f := thumbScaleFilter(320); !strings.Contains(f, "min(320,iw)") {
		t.Fatalf("应使用传入上限，实得 %q", f)
	}
}

// 11) 抽帧位置上限可配：capMS=0 → 恒抽第 0 帧（站长显式配置的合法选择）。
func TestThumbSeekMSWithCustomCap(t *testing.T) {
	if got := ThumbSeekMS(60000, 0); got != 0 {
		t.Fatalf("cap=0 应恒为 0，实得 %d", got)
	}
	if got := ThumbSeekMS(60000, 3000); got != 3000 {
		t.Fatalf("cap=3000 应夹到 3000，实得 %d", got)
	}
	if got := ThumbSeekMS(5000, 3000); got != 500 {
		t.Fatalf("未触上限时应取 1/10，实得 %d", got)
	}
}

// 8) 孤儿对账：文件行被删掉后，其媒体行（含派生引用）必须被识别出来，并带上引用供调用方清文件。
func TestDeleteOrphansReturnsDerivedRef(t *testing.T) {
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "orphan.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	b := bus.New()
	aud := New(db)
	if err := EnsureSystem(ctx, db, aud); err != nil {
		t.Fatalf("ensure system: %v", err)
	}
	if err := EnsureBlogSpace(ctx, db); err != nil {
		t.Fatalf("ensure blog space: %v", err)
	}
	st, err := storage.NewLocal(filepath.Join(dir, "files"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	fs := NewFileStore(db, st, b, aud)
	ms := NewMediaStore(db)

	f, err := fs.Upload(ctx, SystemOwnerID, SystemHomeSpaceID, BlogDirID, "orphan.png", "image/png",
		strings.NewReader("png"), 3, DefaultSiteID)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	key := DerivedKey(f.SpaceID, f.ID, "thumb.jpg")
	if err := fs.PutDerived(ctx, key, strings.NewReader("thumb"), 5); err != nil {
		t.Fatalf("PutDerived: %v", err)
	}
	if err := ms.Upsert(ctx, &MediaInfo{FileID: f.ID, Width: 1, Height: 1, ProbeStatus: MediaOK, ThumbnailRef: key}); err != nil {
		t.Fatalf("upsert media: %v", err)
	}
	// 软删不应被当成孤儿（回收站里的文件可恢复，其媒体信息必须留着）
	if _, err := db.ExecContext(ctx, `UPDATE files SET deleted_at=? WHERE id=?`, now(), f.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if orph, err := ms.DeleteOrphans(ctx); err != nil || len(orph) != 0 {
		t.Fatalf("软删文件的媒体行不该被判为孤儿：%v / %v", orph, err)
	}
	// 真正删掉文件行 → 成为孤儿
	if _, err := db.ExecContext(ctx, `DELETE FROM file_comments_tags WHERE 1=0`); err != nil {
		_ = err // 该表可能不存在，忽略（只为证明无副作用）
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM files WHERE id=?`, f.ID); err != nil {
		t.Fatalf("hard delete: %v", err)
	}
	orph, err := ms.DeleteOrphans(ctx)
	if err != nil {
		t.Fatalf("DeleteOrphans: %v", err)
	}
	if len(orph) != 1 || orph[0].FileID != f.ID || orph[0].ThumbnailRef != key {
		t.Fatalf("孤儿识别错误：%+v", orph)
	}
	if _, ok := ms.Get(ctx, f.ID); ok {
		t.Fatal("孤儿媒体行应已被删除")
	}
	// 调用方按引用清派生对象
	if n := fs.DeleteDerivedPrefix(ctx, DerivedPrefixOfKey(orph[0].ThumbnailRef)); n != 1 {
		t.Fatalf("应清掉 1 个派生对象，实得 %d", n)
	}
	if _, ok := fs.DerivedStat(ctx, key); ok {
		t.Fatal("派生对象应已清掉")
	}
}

// fakeCfg 桩：模拟 config.Store.GetString —— 整型值在真实现里会被 fmt.Sprint 成字符串。
type fakeCfg map[string]string

func (f fakeCfg) GetString(k string) string { return f[k] }
