package storage

import (
	"context"
	"io"
	"strings"
	"testing"
)

func newLocal(t *testing.T) *Local {
	t.Helper()
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return l
}

func TestPutGetDelete(t *testing.T) {
	l := newLocal(t)
	ctx := context.Background()
	if err := l.Put(ctx, "a/b/note.md", strings.NewReader("hello"), 5); err != nil {
		t.Fatalf("put: %v", err)
	}
	rc, meta, err := l.Get(ctx, "a/b/note.md")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close() // Windows 句柄：必须在 Delete 前关闭
	if string(b) != "hello" {
		t.Fatalf("content=%q", b)
	}
	if meta.Size != 5 {
		t.Fatalf("size=%d", meta.Size)
	}
	if err := l.Delete(ctx, "a/b/note.md"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, err := l.Get(ctx, "a/b/note.md"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestMove(t *testing.T) {
	l := newLocal(t)
	ctx := context.Background()
	_ = l.Put(ctx, "old.md", strings.NewReader("x"), 1)
	if err := l.Move(ctx, "old.md", "new/place.md"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if _, _, err := l.Get(ctx, "old.md"); err != ErrNotFound {
		t.Fatal("old 仍存在")
	}
	rc, _, err := l.Get(ctx, "new/place.md")
	if err != nil {
		t.Fatalf("get new: %v", err)
	}
	rc.Close()
}

func TestListPrefix(t *testing.T) {
	l := newLocal(t)
	ctx := context.Background()
	for _, k := range []string{"d1/a.txt", "d1/b.txt", "d2/c.txt"} {
		if err := l.Put(ctx, k, strings.NewReader("v"), 1); err != nil {
			t.Fatal(err)
		}
	}
	items, err := l.List(ctx, "d1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len=%d", len(items))
	}
}

func TestPathTraversalBlocked(t *testing.T) {
	l := newLocal(t)
	ctx := context.Background()
	// 尝试跳出根目录
	if err := l.Put(ctx, "../../escape.md", strings.NewReader("x"), 1); err == nil {
		t.Fatal("路径穿越未被拦截")
	}
}
