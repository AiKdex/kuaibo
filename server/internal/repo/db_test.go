package repo

import (
	"database/sql"
	"testing"
)

func openMem(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrateCreatesAllTables(t *testing.T) {
	db := openMem(t)
	// 数据层前置预留的关键表（实施文档 §7.10 / §10.5）
	want := []string{
		"users", "spaces", "files", "index_chunks", "tags", "file_tags",
		"collections", "collection_files", "shares", "comments", "notifications",
		"jobs", "settings", "audit_log", "vectors",
	}
	for _, name := range want {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil {
			t.Fatalf("query %s: %v", name, err)
		}
		if n != 1 {
			t.Errorf("表 %s 未创建", name)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	db := openMem(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestOpenPersistent(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.db"
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	db.Close()
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	defer db2.Close()
	var n int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
}
