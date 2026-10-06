// derived_test.go 派生资产统计与清理（B16）。
//
// 覆盖三件不能退化的事：
//  1. key → 源文件 id 的解析（清了谁、算谁的，全靠它）；
//  2. 统计口径（对象数/字节/覆盖文件数/扩展名分布）；
//  3. 孤儿判定与清理路径（这是 B16 存在的理由）。
package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

// 1) 由派生对象 key 解析源文件 id；非派生路径一律空串（不 panic、不误判）。
func TestDerivedFileIDOfKey(t *testing.T) {
	cases := []struct{ key, want string }{
		{"spaces/S1/.derived/F1/thumb.jpg", "F1"},
		{"spaces/S1/.derived/F2/sub/x.jpg", "F2"},
		{"thumb.jpg", ""},
		{"spaces/S1/files/F1/a.jpg", ""}, // 不是派生目录
		{"spaces/S1/.derived/F1", ""},    // 段数不足
		{"", ""},
	}
	for _, c := range cases {
		if got := DerivedFileIDOfKey(c.key); got != c.want {
			t.Errorf("DerivedFileIDOfKey(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}

// 2) 统计口径 + 按文件清 / 全清 / 幂等（无 db：只验列举与删除路径）。
func TestDerivedStatsAndPurge(t *testing.T) {
	st, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	s := NewFileStore(nil, st, nil, nil)
	ctx := context.Background()
	sp := SystemHomeSpaceID

	put := func(fid, name, body string) {
		t.Helper()
		if err := s.PutDerived(ctx, DerivedKey(sp, fid, name), strings.NewReader(body), int64(len(body))); err != nil {
			t.Fatalf("PutDerived(%s): %v", name, err)
		}
	}
	put("F1", "thumb.jpg", "aaaa")   // 4B
	put("F2", "thumb.jpg", "bbbbbb") // 6B
	put("F2", "cover.png", "cc")     // 2B

	got := s.DerivedStats(ctx)
	if !got.Listable {
		t.Fatal("本地后端应可列举")
	}
	if got.Objects != 3 || got.Bytes != 12 {
		t.Fatalf("对象数/字节错误：%+v", got)
	}
	if got.Files != 2 {
		t.Fatalf("应覆盖 2 个源文件，实得 %d", got.Files)
	}
	if got.Spaces != 1 {
		t.Fatalf("应覆盖 1 个空间，实得 %d", got.Spaces)
	}
	if got.ByExt[".jpg"] != 2 || got.ByExt[".png"] != 1 {
		t.Fatalf("扩展名分布错误：%+v", got.ByExt)
	}
	// 🔴 无 db 时不做孤儿判定：宁可漏报，也不可误报（误报会诱导站长清掉正常缓存）
	if got.Orphans != 0 {
		t.Fatalf("无 db 时不应报孤儿，实得 %d", got.Orphans)
	}

	// 按文件清：只动 F2，不牵连 F1
	n, freed := s.PurgeDerived(ctx, "F2")
	if n != 2 || freed != 8 {
		t.Fatalf("按文件清应删 2 个/8B，实得 %d/%d", n, freed)
	}
	if _, ok := s.DerivedStat(ctx, DerivedKey(sp, "F2", "thumb.jpg")); ok {
		t.Fatal("F2 的派生对象应已清掉")
	}
	if _, ok := s.DerivedStat(ctx, DerivedKey(sp, "F1", "thumb.jpg")); !ok {
		t.Fatal("F1 的派生对象不该被牵连")
	}

	// 全清
	n2, freed2 := s.PurgeDerived(ctx, "")
	if n2 != 1 || freed2 != 4 {
		t.Fatalf("全清应删 1 个/4B，实得 %d/%d", n2, freed2)
	}
	if after := s.DerivedStats(ctx); after.Objects != 0 || after.Spaces != 0 {
		t.Fatalf("全清后应为空：%+v", after)
	}
	// 幂等：清理路径随时可能被重复触发
	if n3, _ := s.PurgeDerived(ctx, ""); n3 != 0 {
		t.Fatalf("重复清理应删 0，实得 %d", n3)
	}
}

// 3) 孤儿判定：源文件行还在 → 不算孤儿；文件行被删 → 算孤儿并可被清掉。
//
// 这是 B16 的核心价值用例：B15 的清理路径依赖「源文件走到过删除流程」，
// 而直接删库/回滚 DB 会让派生对象变成无人认领的孤儿。
func TestDerivedStatsDetectsOrphans(t *testing.T) {
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "derived.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	aud := New(db)
	if err := EnsureSystem(ctx, db, aud); err != nil {
		t.Fatalf("ensure system: %v", err)
	}
	if err := EnsureBlogSpace(ctx, db); err != nil {
		t.Fatalf("ensure blog space: %v", err)
	}
	loc, err := storage.NewLocal(filepath.Join(dir, "files"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	fs := NewFileStore(db, loc, bus.New(), aud)

	f, err := fs.Upload(ctx, SystemOwnerID, SystemHomeSpaceID, BlogDirID, "o.png", "image/png",
		strings.NewReader("png"), 3, DefaultSiteID)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	key := DerivedKey(f.SpaceID, f.ID, "thumb.jpg")
	if err := fs.PutDerived(ctx, key, strings.NewReader("x"), 1); err != nil {
		t.Fatalf("PutDerived: %v", err)
	}

	if got := fs.DerivedStats(ctx); got.Orphans != 0 {
		t.Fatalf("源文件仍在时不该报孤儿：%+v", got)
	}

	// 绕过删除流程直接抹掉文件行 → 派生对象成为孤儿（B15 的两条清理路径都覆盖不到）
	if _, err := db.ExecContext(ctx, `DELETE FROM files WHERE id=?`, f.ID); err != nil {
		t.Fatalf("hard delete: %v", err)
	}
	got := fs.DerivedStats(ctx)
	if got.Objects != 1 || got.Orphans != 1 || got.OrphanBytes != 1 {
		t.Fatalf("应识别出 1 个孤儿（1B）：%+v", got)
	}
	if n, freed := fs.PurgeDerived(ctx, f.ID); n != 1 || freed != 1 {
		t.Fatalf("应清掉该孤儿，实得 %d/%d", n, freed)
	}
	if _, ok := fs.DerivedStat(ctx, key); ok {
		t.Fatal("孤儿应已被清掉")
	}
}
