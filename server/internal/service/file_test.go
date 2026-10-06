// Package service 业务编排层测试。
// file_test.go 覆盖文件服务：上传/冲突改名/目录/移动/删除（含软删与循环检查）。
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

func newTestFS(t *testing.T) (*FileStore, context.Context, string, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	b := bus.New()
	aud := New(db)
	st, err := storage.NewLocal(filepath.Join(dir, "files"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	owner := SystemOwnerID
	space := "00000000-0000-0000-0000-000000000002"
	if err := EnsureSystem(context.Background(), db, aud); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return NewFileStore(db, st, b, aud), context.Background(), owner, space
}

func TestFileUploadAndConflictRename(t *testing.T) {
	fs, ctx, owner, space := newTestFS(t)
	f1, err := fs.Upload(ctx, owner, space, "", "a.md", "text/markdown", strings.NewReader("# hi"), -1, DefaultSiteID)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if f1.Name != "a.md" || f1.Kind != "file" {
		t.Fatalf("bad file: %+v", f1)
	}
	// 同名冲突 → a-1.md
	f2, err := fs.Upload(ctx, owner, space, "", "a.md", "text/markdown", strings.NewReader("# hi2"), -1, DefaultSiteID)
	if err != nil {
		t.Fatalf("conflict upload: %v", err)
	}
	if f2.Name != "a-1.md" {
		t.Fatalf("expected a-1.md, got %s", f2.Name)
	}
	items, err := fs.ListDir(ctx, space, "", DefaultSiteID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// 内容可读
	rc, f, err := fs.Content(ctx, f1.ID)
	if err != nil {
		t.Fatalf("content: %v", err)
	}
	defer rc.Close()
	buf := make([]byte, 4)
	n, _ := rc.Read(buf)
	if string(buf[:n]) != "# hi" {
		t.Fatalf("content mismatch: %q", buf[:n])
	}
	_ = f
}

func TestDirMoveDeleteCycle(t *testing.T) {
	fs, ctx, owner, space := newTestFS(t)
	dir1, err := fs.CreateDir(ctx, owner, space, "", "docs", DefaultSiteID)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dir2, err := fs.CreateDir(ctx, owner, space, dir1.ID, "sub", DefaultSiteID)
	if err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	// 循环移动检查：把 docs 移进 sub → 应报错
	if _, err := fs.Move(ctx, owner, dir1.ID, dir2.ID, ""); err == nil {
		t.Fatal("expected cycle error")
	}
	// 正常移动：sub 移到根
	if _, err := fs.Move(ctx, owner, dir2.ID, "", "renamed"); err != nil {
		t.Fatalf("move: %v", err)
	}
	// 删除目录（软删）
	if err := fs.Delete(ctx, owner, dir1.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// docs 已删；renamed（原 sub 移出）仍在根
	items, _ := fs.ListDir(ctx, space, "", DefaultSiteID)
	if len(items) != 1 || items[0].Name != "renamed" {
		t.Fatalf("expected only 'renamed', got %d items", len(items))
	}
	if _, err := fs.Get(ctx, dir1.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestInvalidName(t *testing.T) {
	fs, ctx, owner, space := newTestFS(t)
	if _, err := fs.Upload(ctx, owner, space, "", "../evil", "", strings.NewReader("x"), 1, DefaultSiteID); err != ErrInvalidName {
		t.Fatalf("expected ErrInvalidName, got %v", err)
	}
	if _, err := fs.CreateDir(ctx, owner, space, "", "a/b", DefaultSiteID); err != ErrInvalidName {
		t.Fatalf("expected ErrInvalidName for slash, got %v", err)
	}
}
