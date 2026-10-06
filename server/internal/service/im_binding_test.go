// im_binding_test.go B7 IM 绑定测试：绑定码生命周期、身份唯一、改绑、解绑、越权删除、日报推送不再死。
package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// imbCfgStub 最小配置桩（DigestStore 只依赖 GetString/GetBool）。
type imbCfgStub map[string]string

func (s imbCfgStub) GetString(k string) string { return s[k] }
func (s imbCfgStub) GetBool(k string) bool     { return s[k] == "1" }

func newIMBindStore(t *testing.T) (*IMBindingStore, *sql.DB) {
	t.Helper()
	db := newB6DB(t)
	seedUser(t, db, "u-alice", "alice", "爱丽丝")
	seedUser(t, db, "u-bob", "bob", "鲍勃")
	return NewIMBindingStore(db), db
}

// 正常链路：生成码 → IM 侧消费 → 反查身份。
func TestIMBindCodeRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, _ := newIMBindStore(t)

	code, exp, err := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if err != nil {
		t.Fatalf("CreateCode: %v", err)
	}
	if len(code) != imbCodeLen {
		t.Fatalf("code length = %d, want %d (%q)", len(code), imbCodeLen, code)
	}
	if exp <= time.Now().UnixMilli() {
		t.Fatalf("expires_at not in future: %d", exp)
	}

	// 大小写不敏感 + 前后空格容忍（人工转述场景）
	uid, err := s.ConsumeCode(ctx, " "+code+" ", "telegram", "tg-1001", "Alice", "chat-1001")
	if err != nil {
		t.Fatalf("ConsumeCode: %v", err)
	}
	if uid != "u-alice" {
		t.Fatalf("bound uid = %q, want u-alice", uid)
	}

	got, ok := s.ResolveUser(ctx, "telegram", "tg-1001")
	if !ok || got != "u-alice" {
		t.Fatalf("ResolveUser = (%q,%v), want (u-alice,true)", got, ok)
	}
	// 未绑定的 IM 身份不得反查出用户
	if got, ok := s.ResolveUser(ctx, "telegram", "tg-unknown"); ok {
		t.Fatalf("ResolveUser(unknown) = (%q,true), want false", got)
	}
}

// 一次性：同一个码不能消费两次。
func TestIMBindCodeUsedOnce(t *testing.T) {
	ctx := context.Background()
	s, _ := newIMBindStore(t)

	code, _, err := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if err != nil {
		t.Fatalf("CreateCode: %v", err)
	}
	if _, err := s.ConsumeCode(ctx, code, "telegram", "tg-1", "Alice", "c1"); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := s.ConsumeCode(ctx, code, "telegram", "tg-2", "Bob", "c2"); !errors.Is(err, ErrBindCodeUsed) {
		t.Fatalf("second consume err = %v, want ErrBindCodeUsed", err)
	}
	// 第二次消费不得产生任何绑定
	if _, ok := s.ResolveUser(ctx, "telegram", "tg-2"); ok {
		t.Fatal("tg-2 should not be bound")
	}
}

// 过期码不可用。
func TestIMBindCodeExpired(t *testing.T) {
	ctx := context.Background()
	s, db := newIMBindStore(t)

	code, _, err := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if err != nil {
		t.Fatalf("CreateCode: %v", err)
	}
	if _, err := db.Exec(`UPDATE im_binding_codes SET expires_at=? WHERE code=?`,
		time.Now().UnixMilli()-1000, code); err != nil {
		t.Fatalf("force expire: %v", err)
	}
	if _, err := s.ConsumeCode(ctx, code, "telegram", "tg-1", "Alice", "c1"); !errors.Is(err, ErrBindCodeExpired) {
		t.Fatalf("err = %v, want ErrBindCodeExpired", err)
	}
}

// 重新生成会让旧码立即失效（避免旧码被翻出来后仍可用）。
func TestIMBindRegenerateInvalidatesOld(t *testing.T) {
	ctx := context.Background()
	s, _ := newIMBindStore(t)

	oldCode, _, err := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if err != nil {
		t.Fatalf("CreateCode#1: %v", err)
	}
	newCode, _, err := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if err != nil {
		t.Fatalf("CreateCode#2: %v", err)
	}
	if oldCode == newCode {
		t.Fatal("codes should differ")
	}
	if _, err := s.ConsumeCode(ctx, oldCode, "telegram", "tg-1", "Alice", "c1"); !errors.Is(err, ErrBindCodeUsed) {
		t.Fatalf("old code err = %v, want ErrBindCodeUsed", err)
	}
	if _, err := s.ConsumeCode(ctx, newCode, "telegram", "tg-1", "Alice", "c1"); err != nil {
		t.Fatalf("new code consume: %v", err)
	}
}

// 码限定平台时不接受其它平台消费；未限定（空）则任意平台可用。
func TestIMBindPlatformMismatch(t *testing.T) {
	ctx := context.Background()
	s, _ := newIMBindStore(t)

	tgOnly, _, err := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if err != nil {
		t.Fatalf("CreateCode: %v", err)
	}
	if _, err := s.ConsumeCode(ctx, tgOnly, "wecom", "wc-1", "Alice", ""); !errors.Is(err, ErrBindCodeInvalid) {
		t.Fatalf("cross-platform err = %v, want ErrBindCodeInvalid", err)
	}

	anyPlat, _, err := s.CreateCode(ctx, "u-alice", "", time.Minute)
	if err != nil {
		t.Fatalf("CreateCode(any): %v", err)
	}
	if _, err := s.ConsumeCode(ctx, anyPlat, "wecom", "wc-1", "Alice", ""); err != nil {
		t.Fatalf("any-platform consume: %v", err)
	}
}

// 缺平台身份（无法识别是谁）时拒绝绑定。
func TestIMBindNoIdentity(t *testing.T) {
	ctx := context.Background()
	s, _ := newIMBindStore(t)

	code, _, err := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if err != nil {
		t.Fatalf("CreateCode: %v", err)
	}
	if _, err := s.ConsumeCode(ctx, code, "telegram", "", "Alice", "c1"); !errors.Is(err, ErrBindNoIdentity) {
		t.Fatalf("err = %v, want ErrBindNoIdentity", err)
	}
}

// 同一 IM 身份只对应一个站内用户；换码再绑＝改绑，旧用户失去该身份。
func TestIMBindRebindTransfersIdentity(t *testing.T) {
	ctx := context.Background()
	s, db := newIMBindStore(t)

	c1, _, _ := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if _, err := s.ConsumeCode(ctx, c1, "telegram", "tg-shared", "Alice", "chat-a"); err != nil {
		t.Fatalf("alice bind: %v", err)
	}

	c2, _, _ := s.CreateCode(ctx, "u-bob", "telegram", time.Minute)
	if _, err := s.ConsumeCode(ctx, c2, "telegram", "tg-shared", "Bob", "chat-b"); err != nil {
		t.Fatalf("bob rebind: %v", err)
	}

	uid, ok := s.ResolveUser(ctx, "telegram", "tg-shared")
	if !ok || uid != "u-bob" {
		t.Fatalf("after rebind ResolveUser = (%q,%v), want (u-bob,true)", uid, ok)
	}
	// 唯一约束：同一 IM 身份只留一行
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM im_bindings WHERE platform='telegram' AND platform_user_id='tg-shared'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows for tg-shared = %d, want 1", n)
	}
	// alice 不再持有该身份
	if items, err := s.ListByUser(ctx, "u-alice"); err != nil {
		t.Fatalf("ListByUser(alice): %v", err)
	} else if len(items) != 0 {
		t.Fatalf("alice bindings = %d, want 0", len(items))
	}
	// 改绑后 chat_id 换到新会话
	var chat string
	if err := db.QueryRow(`SELECT chat_id FROM im_bindings WHERE platform='telegram' AND platform_user_id='tg-shared'`).Scan(&chat); err != nil {
		t.Fatalf("chat_id: %v", err)
	}
	if chat != "chat-b" {
		t.Fatalf("chat_id = %q, want chat-b", chat)
	}
}

// 解绑（IM 端 /unbind 走 UnbindIdentity；Web 端走 Unbind 且只能删自己的）。
func TestIMBindUnbindAndScopedDelete(t *testing.T) {
	ctx := context.Background()
	s, _ := newIMBindStore(t)

	code, _, _ := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if _, err := s.ConsumeCode(ctx, code, "telegram", "tg-1", "Alice", "c1"); err != nil {
		t.Fatalf("consume: %v", err)
	}
	items, err := s.ListByUser(ctx, "u-alice")
	if err != nil || len(items) != 1 {
		t.Fatalf("ListByUser = (%d,%v), want 1 item", len(items), err)
	}
	id := items[0].ID

	// 越权：bob 删 alice 的绑定 → 不存在
	if err := s.Unbind(ctx, "u-bob", id); !errors.Is(err, ErrBindNotFound) {
		t.Fatalf("cross-user unbind err = %v, want ErrBindNotFound", err)
	}
	// alice 自己删 → 成功
	if err := s.Unbind(ctx, "u-alice", id); err != nil {
		t.Fatalf("owner unbind: %v", err)
	}
	if _, ok := s.ResolveUser(ctx, "telegram", "tg-1"); ok {
		t.Fatal("binding should be gone")
	}
	// 重复删 → 不存在
	if err := s.Unbind(ctx, "u-alice", id); !errors.Is(err, ErrBindNotFound) {
		t.Fatalf("re-unbind err = %v, want ErrBindNotFound", err)
	}
	// IM 端解绑：未绑定时明确报错
	if _, err := s.UnbindIdentity(ctx, "telegram", "tg-1"); !errors.Is(err, ErrBindNotFound) {
		t.Fatalf("UnbindIdentity(unbound) err = %v, want ErrBindNotFound", err)
	}
}

// 日报推送的死路径回归：表建起来 + 无绑定时 Send 必须成功返回（此前必然报 "no such table"）。
func TestDigestSendWithoutBinding(t *testing.T) {
	ctx := context.Background()
	db := newB6DB(t)
	seedUser(t, db, "u-alice", "alice", "爱丽丝")

	d := NewDigestStore(db, imbCfgStub{})
	if err := d.Send(ctx, "u-alice"); err != nil {
		t.Fatalf("Send with no binding should succeed, got: %v", err)
	}

	// 绑了 telegram 但站点未配 bot token：跳过（不报错、不静默吞掉配置缺失之外的错误）
	s := NewIMBindingStore(db)
	code, _, _ := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if _, err := s.ConsumeCode(ctx, code, "telegram", "tg-1", "Alice", "chat-1"); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if err := d.Send(ctx, "u-alice"); err != nil {
		t.Fatalf("Send with token unset should skip, got: %v", err)
	}
}

// 日报统计口径：files.created_at 是毫秒，若用秒比较会把全部文件算成"今日入库"。
func TestDigestTodayCountUsesMillis(t *testing.T) {
	ctx := context.Background()
	db := newB6DB(t)
	seedUser(t, db, "u-alice", "alice", "爱丽丝")
	seedSpace(t, db, "sp-imb", "u-alice")

	// 造一个"很久以前"的文件（毫秒时间戳，30 天前）；FK 强制 → space 必须真实、parent 用 NULL
	old := time.Now().AddDate(0, 0, -30).UnixMilli()
	if _, err := db.Exec(
		`INSERT INTO files (id, space_id, owner_id, parent_id, name, kind, storage_ref, created_at, updated_at)
		 VALUES ('f-old','sp-imb','u-alice',NULL,'old.md','file','ref/f-old',?,?)`, old, old); err != nil {
		t.Fatalf("seed old file: %v", err)
	}
	d := NewDigestStore(db, imbCfgStub{})
	rep, err := d.Build(ctx, "u-alice")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if rep.InboxToday != 0 {
		t.Fatalf("InboxToday = %d, want 0（30 天前的文件不该算今日入库；若=全部文件数说明仍在秒/毫秒混用）", rep.InboxToday)
	}
	if rep.TotalFiles == 0 {
		t.Fatal("TotalFiles should count the seeded file")
	}
}
