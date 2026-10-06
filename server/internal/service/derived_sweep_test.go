// derived_sweep_test.go 孤儿派生对象定时/手动兜底清理（B18）。
//
// 覆盖三件不能退化的事：
//  1. 源文件行还在 → 保留（不误删正常缓存）；
//  2. 源文件被直删 / 回滚库 → 派生对象成孤儿，被清掉；
//  3. 幂等（重复 sweep 不再删任何东西）。
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

// SweepOrphans 删孤儿、留正常：与 DerivedStats 的孤儿判定口径一致。
func TestSweepOrphans(t *testing.T) {
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "sweep.db"))
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

	// 两个真实文件 + 各自的派生对象
	f1, err := fs.Upload(ctx, SystemOwnerID, SystemHomeSpaceID, BlogDirID, "a.png", "image/png",
		strings.NewReader("aaaa"), 4, DefaultSiteID)
	if err != nil {
		t.Fatalf("upload f1: %v", err)
	}
	f2, err := fs.Upload(ctx, SystemOwnerID, SystemHomeSpaceID, BlogDirID, "b.png", "image/png",
		strings.NewReader("bbbb"), 4, DefaultSiteID)
	if err != nil {
		t.Fatalf("upload f2: %v", err)
	}
	k1 := DerivedKey(f1.SpaceID, f1.ID, "thumb.jpg")
	k2 := DerivedKey(f2.SpaceID, f2.ID, "thumb.jpg")
	if err := fs.PutDerived(ctx, k1, strings.NewReader("x"), 1); err != nil {
		t.Fatalf("put k1: %v", err)
	}
	if err := fs.PutDerived(ctx, k2, strings.NewReader("y"), 1); err != nil {
		t.Fatalf("put k2: %v", err)
	}

	// 首次 sweep：两个源文件都还在，不应删任何东西
	n, freed, err := fs.SweepOrphans(ctx)
	if err != nil {
		t.Fatalf("sweep1: %v", err)
	}
	if n != 0 || freed != 0 {
		t.Fatalf("源文件都在时不应清理，实得 %d/%d", n, freed)
	}
	if _, ok := fs.DerivedStat(ctx, k1); !ok {
		t.Fatal("k1 应保留")
	}
	if _, ok := fs.DerivedStat(ctx, k2); !ok {
		t.Fatal("k2 应保留")
	}

	// 直删 f2 文件行 → f2 的派生对象成孤儿
	if _, err := db.ExecContext(ctx, `DELETE FROM files WHERE id=?`, f2.ID); err != nil {
		t.Fatalf("hard delete f2: %v", err)
	}
	n2, freed2, err := fs.SweepOrphans(ctx)
	if err != nil {
		t.Fatalf("sweep2: %v", err)
	}
	if n2 != 1 || freed2 != 1 {
		t.Fatalf("应清 1 个孤儿/1B，实得 %d/%d", n2, freed2)
	}
	if _, ok := fs.DerivedStat(ctx, k2); ok {
		t.Fatal("孤儿 k2 应被清掉")
	}
	if _, ok := fs.DerivedStat(ctx, k1); !ok {
		t.Fatal("正常 k1 不应被牵连")
	}

	// 再 sweep：已无可清，幂等返回 0
	n3, _, _ := fs.SweepOrphans(ctx)
	if n3 != 0 {
		t.Fatalf("重复 sweep 应删 0，实得 %d", n3)
	}
}

// SweepOrphans 无 db 时绝对不删（宁可漏报，绝不误删）。
func TestSweepOrphansNoDB(t *testing.T) {
	st, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	s := NewFileStore(nil, st, nil, nil)
	ctx := context.Background()
	key := DerivedKey(SystemHomeSpaceID, "NOPE", "thumb.jpg")
	if err := s.PutDerived(ctx, key, strings.NewReader("x"), 1); err != nil {
		t.Fatalf("put: %v", err)
	}
	n, _, err := s.SweepOrphans(ctx)
	if err != nil || n != 0 {
		t.Fatalf("无 db 时不应删，实得 n=%d err=%v", n, err)
	}
	if _, ok := s.DerivedStat(ctx, key); !ok {
		t.Fatal("无 db 时对象应原样保留")
	}
}
