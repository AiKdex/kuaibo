package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/repo"
)

// ---- B6 共用测试脚手架 ----

func newB6DB(t *testing.T) *sql.DB {
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
	return db
}

func seedUser(t *testing.T, db *sql.DB, id, username, display string) {
	t.Helper()
	now := time.Now().Unix()
	if _, err := db.Exec(
		`INSERT INTO users (id, username, pass_hash, display_name, role, status, preferences, created_at, updated_at)
		 VALUES (?, ?, 'x', ?, 'member', 'active', '{}', ?, ?)`,
		id, username, display, now, now); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func seedSpace(t *testing.T, db *sql.DB, id, owner string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO spaces (id, owner_id, name, kind, created_at, updated_at) VALUES (?, ?, 'home', 'home', ?, ?)`,
		id, owner, time.Now().Unix(), time.Now().Unix()); err != nil {
		t.Fatalf("seed space %s: %v", id, err)
	}
}

func seedFileRow(t *testing.T, db *sql.DB, id, space, owner, parent, name, kind string) {
	t.Helper()
	var parentArg any
	if parent == "" {
		parentArg = nil
	} else {
		parentArg = parent
	}
	now := time.Now().UnixMilli()
	if _, err := db.Exec(
		`INSERT INTO files (id, space_id, owner_id, parent_id, name, kind, storage_ref, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, space, owner, parentArg, name, kind, "ref/"+id, now, now); err != nil {
		t.Fatalf("seed file %s: %v", id, err)
	}
}

// ---- 订阅：基础闭环与幂等 ----

func TestSubscriptionRoundTrip(t *testing.T) {
	db := newB6DB(t)
	ctx := context.Background()
	st := NewSubscriptionStore(db)

	created, err := st.Subscribe(ctx, "u1", SubTargetFile, "f1")
	if err != nil || !created {
		t.Fatalf("首次订阅应 created=true: created=%v err=%v", created, err)
	}
	// 幂等：重复订阅不报错、不新增行
	created2, err := st.Subscribe(ctx, "u1", SubTargetFile, "f1")
	if err != nil || created2 {
		t.Fatalf("重复订阅应 created=false 且不报错: created=%v err=%v", created2, err)
	}
	if c, _ := st.CountByUser(ctx, "u1"); c != 1 {
		t.Fatalf("订阅数应为 1，得到 %d", c)
	}
	if ok, _ := st.IsSubscribed(ctx, "u1", SubTargetFile, "f1"); !ok {
		t.Fatalf("IsSubscribed 应为 true")
	}
	// 大小写/空白宽容：'FILE' + 前后空格应归一为同一行（幂等）
	if created3, err := st.Subscribe(ctx, "u1", " FILE ", " f1 "); err != nil || created3 {
		t.Fatalf("归一后应视为已订阅: created=%v err=%v", created3, err)
	}
	if c, _ := st.CountByUser(ctx, "u1"); c != 1 {
		t.Fatalf("归一后订阅数仍应为 1，得到 %d", c)
	}

	items, err := st.ListByUser(ctx, "u1")
	if err != nil || len(items) != 1 || items[0].TargetID != "f1" {
		t.Fatalf("ListByUser 不对: %+v err=%v", items, err)
	}

	removed, err := st.Unsubscribe(ctx, "u1", SubTargetFile, "f1")
	if err != nil || !removed {
		t.Fatalf("退订应 removed=true: %v %v", removed, err)
	}
	removed2, err := st.Unsubscribe(ctx, "u1", SubTargetFile, "f1")
	if err != nil || removed2 {
		t.Fatalf("重复退订应 removed=false 且不报错: %v %v", removed2, err)
	}
	if ok, _ := st.IsSubscribed(ctx, "u1", SubTargetFile, "f1"); ok {
		t.Fatalf("退订后 IsSubscribed 应为 false")
	}
}

// ---- 订阅：非法目标被拒 ----

func TestSubscriptionBadTarget(t *testing.T) {
	db := newB6DB(t)
	ctx := context.Background()
	st := NewSubscriptionStore(db)

	if _, err := st.Subscribe(ctx, "u1", "bad", "f1"); err == nil {
		t.Fatalf("非法 target_type 应报错")
	}
	if _, err := st.Subscribe(ctx, "u1", SubTargetFile, "  "); err == nil {
		t.Fatalf("空 target_id 应报错")
	}
	if _, err := st.Subscribe(ctx, "", SubTargetFile, "f1"); err == nil {
		t.Fatalf("未登录（空 user）应报错")
	}
}

// ---- 订阅：文件订阅 + 祖先目录订阅 命中同一条链 ----

func TestSubscriberIDsChain(t *testing.T) {
	db := newB6DB(t)
	ctx := context.Background()
	st := NewSubscriptionStore(db)

	seedUser(t, db, "u1", "u1", "甲")
	seedUser(t, db, "u2", "u2", "乙")
	seedSpace(t, db, "sp1", "u1")
	// 目录树：root(dir) → sub(dir) → post(file)
	seedFileRow(t, db, "dirRoot", "sp1", "u1", "", "博客", "dir")
	seedFileRow(t, db, "dirSub", "sp1", "u1", "dirRoot", "技术", "dir")
	seedFileRow(t, db, "post1", "sp1", "u1", "dirSub", "文章.md", "file")

	if _, err := st.Subscribe(ctx, "u1", SubTargetFile, "post1"); err != nil {
		t.Fatalf("u1 订阅文件: %v", err)
	}
	if _, err := st.Subscribe(ctx, "u2", SubTargetDir, "dirRoot"); err != nil {
		t.Fatalf("u2 订阅根目录: %v", err)
	}
	ids, err := st.SubscriberIDs(ctx, "post1")
	if err != nil {
		t.Fatalf("SubscriberIDs: %v", err)
	}
	got := map[string]bool{}
	for _, v := range ids {
		got[v] = true
	}
	if !got["u1"] || !got["u2"] || len(got) != 2 {
		t.Fatalf("应命中 u1(文件订阅)+u2(祖先目录订阅)，得到 %v", ids)
	}
	// 无关文件：目录订阅不命中
	if ids2, _ := st.SubscriberIDs(ctx, "dirSub"); len(ids2) != 1 || ids2[0] != "u2" {
		t.Fatalf("目录自身只应命中 u2，得到 %v", ids2)
	}
}

// ---- 订阅：投递通知（跳过编辑者 / 窗口内去重） ----

func TestDispatchFileChange(t *testing.T) {
	db := newB6DB(t)
	ctx := context.Background()
	st := NewSubscriptionStore(db)
	notify := NewNotifyStore(db)

	seedUser(t, db, "editor", "editor", "编辑者")
	seedUser(t, db, "reader", "reader", "读者")
	seedSpace(t, db, "sp1", "editor")
	seedFileRow(t, db, "dirRoot", "sp1", "editor", "", "博客", "dir")
	seedFileRow(t, db, "post1", "sp1", "editor", "dirRoot", "文章.md", "file")

	// 编辑者自己也订阅了（不应收到自己的通知），读者订阅了祖先目录
	if _, err := st.Subscribe(ctx, "editor", SubTargetFile, "post1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Subscribe(ctx, "reader", SubTargetDir, "dirRoot"); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	sent := st.DispatchFileChange(ctx, notify, FileChangeNotice{
		FileID: "post1", EditorID: "editor", Title: "《文章》已更新",
		Link: "/read/post1", DebounceSec: 600, Now: now,
	})
	if sent != 1 {
		t.Fatalf("应只给读者发 1 条（跳过编辑者），得到 %d", sent)
	}
	// 读者未读数 = 1；编辑者 = 0
	if n, _ := notify.UnreadCount(ctx, "reader", false); n != 1 {
		t.Fatalf("读者未读应为 1，得到 %d", n)
	}
	if n, _ := notify.UnreadCount(ctx, "editor", false); n != 0 {
		t.Fatalf("编辑者不应收到自己的通知，未读=%d", n)
	}
	// 同一窗口内再投递 → 去重（0 条）
	if again := st.DispatchFileChange(ctx, notify, FileChangeNotice{
		FileID: "post1", EditorID: "editor", Link: "/read/post1", DebounceSec: 600, Now: now,
	}); again != 0 {
		t.Fatalf("窗口内应去重为 0，得到 %d", again)
	}
	// 窗口之外 → 再发一条
	if later := st.DispatchFileChange(ctx, notify, FileChangeNotice{
		FileID: "post1", EditorID: "editor", Link: "/read/post1", DebounceSec: 600, Now: now + 1000,
	}); later != 1 {
		t.Fatalf("窗口外应再发 1 条，得到 %d", later)
	}
	if n, _ := notify.UnreadCount(ctx, "reader", false); n != 2 {
		t.Fatalf("读者未读应为 2，得到 %d", n)
	}
}

// ---- 订阅：清理 ----

func TestSubscriptionPurgeTargets(t *testing.T) {
	db := newB6DB(t)
	ctx := context.Background()
	st := NewSubscriptionStore(db)

	if _, err := st.Subscribe(ctx, "u1", SubTargetFile, "f1"); err != nil {
		t.Fatal(err)
	}
	// 另一个用户的同名目标不受影响
	if _, err := st.Subscribe(ctx, "u2", SubTargetFile, "f1"); err != nil {
		t.Fatal(err)
	}
	if err := st.PurgeTarget(ctx, SubTargetFile, "f1"); err != nil {
		t.Fatalf("PurgeTarget: %v", err)
	}
	if c, _ := st.CountByUser(ctx, "u1"); c != 0 {
		t.Fatalf("清理后 u1 订阅数应为 0，得到 %d", c)
	}
	if c, _ := st.CountByUser(ctx, "u2"); c != 0 {
		t.Fatalf("清理后 u2 订阅数应为 0，得到 %d", c)
	}
}
