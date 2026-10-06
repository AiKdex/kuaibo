package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/repo"
)

func newAudit(t *testing.T) (*sql.DB, *AuditStore) {
	t.Helper()
	db, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, New(db)
}

func TestAuditAppendAndVerify(t *testing.T) {
	_, a := newAudit(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := a.Append(ctx, "u1", "file.download", "f1", map[string]any{"n": i}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestAuditChainDetectsTamper(t *testing.T) {
	db, a := newAudit(t)
	ctx := context.Background()
	_, _ = a.Append(ctx, "u1", "action1", "t", nil)
	_, _ = a.Append(ctx, "u2", "action2", "t", nil)
	// 篡改第一条的 detail
	if _, err := db.Exec(`UPDATE audit_log SET detail='{"hacked":true}' WHERE id=1`); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if err := a.Verify(ctx); err == nil {
		t.Fatal("篡改未被检测到")
	}
}

func TestAuditEmptyVerifyOK(t *testing.T) {
	_, a := newAudit(t)
	if err := a.Verify(context.Background()); err != nil {
		t.Fatalf("empty verify: %v", err)
	}
}

func TestHashPasswordRoundTrip(t *testing.T) {
	h := HashPassword("correct horse battery staple")
	if !VerifyPassword(h, "correct horse battery staple") {
		t.Fatal("正确密码校验失败")
	}
	if VerifyPassword(h, "wrong") {
		t.Fatal("错误密码通过校验")
	}
}
