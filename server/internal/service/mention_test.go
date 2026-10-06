package service

import (
	"context"
	"testing"
)

// ---- 提及：解析是纯函数 ----

func TestParseMentions(t *testing.T) {
	// 显式语法 + 纯写法混用
	ids, names := ParseMentions("你好 @[张三](abcd1234-1234-1234-1234-1234567890ab) 与 @lisi，顺便 @小明")
	if len(ids) != 1 || ids[0] != "abcd1234-1234-1234-1234-1234567890ab" {
		t.Fatalf("显式语法应解析出 1 个 id，得到 %v", ids)
	}
	if len(names) != 2 || names[0] != "lisi" || names[1] != "小明" {
		t.Fatalf("纯写法应解析出 [lisi 小明]，得到 %v", names)
	}

	// 去重
	_, names2 := ParseMentions("@lisi @lisi @[张三](abcd1234-1234-1234-1234-1234567890ab) @[张三](abcd1234-1234-1234-1234-1234567890ab)")
	if len(names2) != 1 {
		t.Fatalf("重复名字应去重，得到 %v", names2)
	}

	// 邮件地址不该被当成提及
	if _, n3 := ParseMentions("联系 admin@example.com 或 a@b.cn 即可"); len(n3) != 0 {
		t.Fatalf("邮件地址不应解析为提及，得到 %v", n3)
	}
	// 但行首/空白后的 @ 要生效
	if _, n4 := ParseMentions("@lisi 你好"); len(n4) != 1 || n4[0] != "lisi" {
		t.Fatalf("行首 @ 应生效，得到 %v", n4)
	}
	// 空文本
	if ids5, names5 := ParseMentions(""); ids5 != nil || names5 != nil {
		t.Fatalf("空文本应返回 nil，得到 %v %v", ids5, names5)
	}
	// 中段 @（前面是中文标点）应生效
	if _, n6 := ParseMentions("你好，@lisi 请看"); len(n6) != 1 {
		t.Fatalf("中文逗号后的 @ 应生效，得到 %v", n6)
	}
}

// ---- 提及：落库 + 通知（幂等 / 跳过自己 / 排除幽灵用户） ----

func TestMentionRecord(t *testing.T) {
	db := newB6DB(t)
	ctx := context.Background()
	ms := NewMentionStore(db)
	notify := NewNotifyStore(db)

	seedUser(t, db, "author", "author", "作者")
	seedUser(t, db, "reader", "reader", "读者")
	seedUser(t, db, "lisi", "lisi", "李四")

	// 正文里既有显式语法（指向 reader），又有纯写法（指向 lisi/username）
	text := "感谢 @reader 与 @lisi，@author 是我自己"
	created := ms.Record(ctx, notify, MentionInput{
		SourceType: MentionSourceComment, SourceID: "c1", MentionerID: "author",
		Text: text, Title: "有人在评论里提到了你", Link: "/read/p1#comment-c1",
	})
	if created != 2 {
		t.Fatalf("应新落库 2 条（reader/lisi，跳过自己），得到 %d", created)
	}
	if n, _ := notify.UnreadCount(ctx, "reader", false); n != 1 {
		t.Fatalf("reader 未读通知应为 1，得到 %d", n)
	}
	if n, _ := notify.UnreadCount(ctx, "lisi", false); n != 1 {
		t.Fatalf("lisi 未读通知应为 1，得到 %d", n)
	}
	if n, _ := notify.UnreadCount(ctx, "author", false); n != 0 {
		t.Fatalf("自己不该收到提及通知，未读=%d", n)
	}

	// 幂等：同一条内容重复处理不重复落库、不重复通知
	if again := ms.Record(ctx, notify, MentionInput{
		SourceType: MentionSourceComment, SourceID: "c1", MentionerID: "author", Text: text,
	}); again != 0 {
		t.Fatalf("重复处理应 created=0，得到 %d", again)
	}
	if n, _ := notify.UnreadCount(ctx, "reader", false); n != 1 {
		t.Fatalf("重复处理后 reader 未读仍应为 1，得到 %d", n)
	}

	// 指向不存在的用户：既不落库也不通知（防幽灵）
	before := ms.countAll(t)
	if c := ms.Record(ctx, notify, MentionInput{
		SourceType: MentionSourceComment, SourceID: "c2", MentionerID: "author",
		Text: "@[幽灵](deadbeef-dead-beef-dead-beefdeadbeef)",
	}); c != 0 {
		t.Fatalf("幽灵用户不应落库，得到 %d", c)
	}
	if ms.countAll(t) != before {
		t.Fatalf("幽灵用户不应改变行数")
	}

	// 非法来源类型直接拒绝
	if c := ms.Record(ctx, notify, MentionInput{
		SourceType: "bogus", SourceID: "x", Text: "@lisi",
	}); c != 0 {
		t.Fatalf("非法来源类型应拒绝，得到 %d", c)
	}
}

func (m *MentionStore) countAll(t *testing.T) int {
	t.Helper()
	var n int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM mentions`).Scan(&n); err != nil {
		t.Fatalf("count mentions: %v", err)
	}
	return n
}

// ---- 提及：列表 / 未读 / 全部已读 / 清理 ----

func TestMentionListAndRead(t *testing.T) {
	db := newB6DB(t)
	ctx := context.Background()
	ms := NewMentionStore(db)
	notify := NewNotifyStore(db)

	seedUser(t, db, "author", "author", "作者")
	seedUser(t, db, "reader", "reader", "读者")

	for _, sid := range []string{"c1", "c2"} {
		if c := ms.Record(ctx, notify, MentionInput{
			SourceType: MentionSourceComment, SourceID: sid, MentionerID: "author", Text: "@reader",
		}); c != 1 {
			t.Fatalf("记录 %s 应成功", sid)
		}
	}
	items, err := ms.ListByUser(ctx, "reader", 10)
	if err != nil || len(items) != 2 {
		t.Fatalf("reader 应看到 2 条提及，得到 %d err=%v", len(items), err)
	}
	if n, _ := ms.UnreadCount(ctx, "reader"); n != 2 {
		t.Fatalf("未读应为 2，得到 %d", n)
	}
	if err := ms.MarkAllRead(ctx, "reader"); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	if n, _ := ms.UnreadCount(ctx, "reader"); n != 0 {
		t.Fatalf("已读后未读应为 0，得到 %d", n)
	}
	// 别人的提及不受影响
	if n, _ := ms.UnreadCount(ctx, "author"); n != 0 {
		t.Fatalf("author 不应有提及，得到 %d", n)
	}

	// 清理：源内容删除后提及行一并消失
	if err := ms.PurgeSource(ctx, MentionSourceComment, "c1"); err != nil {
		t.Fatalf("PurgeSource: %v", err)
	}
	items2, _ := ms.ListByUser(ctx, "reader", 10)
	if len(items2) != 1 {
		t.Fatalf("清理后应剩 1 条，得到 %d", len(items2))
	}
	if err := ms.PurgeSources(ctx, MentionSourceComment, []string{"c2"}); err != nil {
		t.Fatalf("PurgeSources: %v", err)
	}
	if items3, _ := ms.ListByUser(ctx, "reader", 10); len(items3) != 0 {
		t.Fatalf("批量清理后应为 0 条，得到 %d", len(items3))
	}
}

// ---- 通知中心：多用户隔离（修掉「任何人可见全部通知」的越权） ----

func TestNotifyScopedByUser(t *testing.T) {
	db := newB6DB(t)
	ctx := context.Background()
	notify := NewNotifyStore(db)

	seedUser(t, db, "ua", "ua", "甲")
	seedUser(t, db, "ub", "ub", "乙")
	// notify.Add 归属 SystemOwnerID（站点级通知），FK 生效故该用户必须真实存在
	seedUser(t, db, SystemOwnerID, "sys", "系统")

	if err := notify.AddUser(ctx, "ua", "ingest", map[string]any{"title": "给甲的"}); err != nil {
		t.Fatal(err)
	}
	if err := notify.AddUser(ctx, "ub", "org.transfer", map[string]any{"title": "给乙的"}); err != nil {
		t.Fatal(err)
	}
	// 站点级通知（SystemOwnerID）
	if err := notify.Add(ctx, "collect", map[string]any{"title": "站点级"}); err != nil {
		t.Fatal(err)
	}

	// 甲（非管理员）只看得到自己的
	itemsA, unreadA, err := notify.List(ctx, "ua", false, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(itemsA) != 1 || itemsA[0].Title != "给甲的" || unreadA != 1 {
		t.Fatalf("甲应只见自己的 1 条，得到 %d 条 unread=%d", len(itemsA), unreadA)
	}
	// 乙看不到甲的
	itemsB, _, _ := notify.List(ctx, "ub", false, 50, false)
	if len(itemsB) != 1 || itemsB[0].Title != "给乙的" {
		t.Fatalf("乙应只见自己的，得到 %+v", itemsB)
	}
	// 管理员额外可见站点级
	itemsAdmin, _, _ := notify.List(ctx, "ua", true, 50, false)
	if len(itemsAdmin) != 2 {
		t.Fatalf("管理员应见 自己的+站点级 = 2 条，得到 %d", len(itemsAdmin))
	}
	// 越界标记已读：乙不能标掉甲的通知
	if err := notify.MarkRead(ctx, "ub", false, itemsA[0].ID); err == nil {
		t.Fatalf("乙标记甲的通知应报错")
	}
	if n, _ := notify.UnreadCount(ctx, "ua", false); n != 1 {
		t.Fatalf("甲的通知仍应未读，得到 %d", n)
	}
	// 自己的可以标
	if err := notify.MarkRead(ctx, "ua", false, itemsA[0].ID); err != nil {
		t.Fatalf("甲标记自己的应成功: %v", err)
	}
	// MarkAllRead 只影响自己
	if err := notify.MarkAllRead(ctx, "ub", false); err != nil {
		t.Fatal(err)
	}
	if n, _ := notify.UnreadCount(ctx, "ua", true); n != 1 {
		t.Fatalf("甲的站点级通知应仍未被乙标掉，得到 %d", n)
	}
}
