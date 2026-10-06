package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/repo"
)

func newCS(t *testing.T) (*CSService, context.Context) {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Migrate(db); err != nil {
		t.Fatal(err)
	}
	// Windows 上未关的 sqlite 句柄会让 t.TempDir() 清理失败（文件被占用）→ 必须显式关闭
	t.Cleanup(func() { _ = db.Close() })
	return NewCS(db), context.Background()
}

// 1) 身份归一：同 (channel, external_id) 两次 ResolveContact → 同一 contact。
func TestCSResolveContactSameIdentity(t *testing.T) {
	s, ctx := newCS(t)
	c1, err := s.ResolveContact(ctx, "webchat", "v-1", "访客甲", "")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.ResolveContact(ctx, "webchat", "v-1", "访客甲", "")
	if err != nil {
		t.Fatal(err)
	}
	if c1.ID != c2.ID {
		t.Fatalf("同渠道同身份应复用档案：%s vs %s", c1.ID, c2.ID)
	}
}

// 2) 跨渠道归一：同邮箱先 webchat 后 email → 同一 contact。
func TestCSResolveContactCrossChannelByEmail(t *testing.T) {
	s, ctx := newCS(t)
	c1, err := s.ResolveContact(ctx, "webchat", "v-9", "甲", "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.ResolveContact(ctx, "email", "a@example.com", "甲", "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if c1.ID != c2.ID {
		t.Fatalf("同邮箱跨渠道应归一：%s vs %s", c1.ID, c2.ID)
	}
}

// 3) 入站幂等：同 external_id 重投 3 次 → 消息仍 1 条。
func TestCSInboundIdempotent(t *testing.T) {
	s, ctx := newCS(t)
	c, _ := s.ResolveContact(ctx, "webchat", "v-2", "乙", "")
	conv, err := s.UpsertConversation(ctx, c.ID, "webchat", "webchat:v-2", "咨询")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, already, err := s.AppendInbound(ctx, conv.ID, CSInbound{
			Channel: "webchat", ExternalID: "m-1", Text: "在吗？",
		}); err != nil {
			t.Fatal(err)
		} else if i == 0 && already {
			t.Fatal("首次投递不应判为重复")
		} else if i > 0 && !already {
			t.Fatal("重复投递应判为 already")
		}
	}
	msgs, err := s.ListMessages(ctx, conv.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("重投 3 次后应只有 1 条消息，得 %d", len(msgs))
	}
	// 未读只 +1
	got, _ := s.GetConversation(ctx, conv.ID)
	if got.UnreadCount != 1 {
		t.Fatalf("未读应只 +1，得 %d", got.UnreadCount)
	}
}

// 4) 线程归并：同 (channel, thread_id) → 同一会话。
func TestCSConversationThreadMerge(t *testing.T) {
	s, ctx := newCS(t)
	c, _ := s.ResolveContact(ctx, "email", "b@example.com", "丙", "b@example.com")
	c1, err := s.UpsertConversation(ctx, c.ID, "email", "<thread-1>", "主题A")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.UpsertConversation(ctx, c.ID, "email", "<thread-1>", "主题A")
	if err != nil {
		t.Fatal(err)
	}
	if c1.ID != c2.ID {
		t.Fatal("同线程应归并到同一会话")
	}
	// 空 thread 每次新建（不撞部分唯一索引）
	e1, _ := s.UpsertConversation(ctx, c.ID, "email", "", "无线程1")
	e2, _ := s.UpsertConversation(ctx, c.ID, "email", "", "无线程2")
	if e1.ID == e2.ID {
		t.Fatal("空 thread 应各自新建会话")
	}
}

// 5) 出站幂等：同 idempotency_key 入队两次 → 只 1 条。
func TestCSOutboundIdempotent(t *testing.T) {
	s, ctx := newCS(t)
	c, _ := s.ResolveContact(ctx, "webchat", "v-3", "丁", "")
	conv, _ := s.UpsertConversation(ctx, c.ID, "webchat", "webchat:v-3", "")
	if _, dup, err := s.EnqueueOutbound(ctx, conv.ID, "webchat", "client-msg-1", map[string]any{"text": "你好"}); err != nil || dup {
		t.Fatalf("首次入队不应重复：dup=%v err=%v", dup, err)
	}
	if _, dup, err := s.EnqueueOutbound(ctx, conv.ID, "webchat", "client-msg-1", map[string]any{"text": "你好"}); err != nil || !dup {
		t.Fatalf("同幂等键应判重：dup=%v err=%v", dup, err)
	}
}

// 6) 坐席回复清零未读并记首次响应（SLA 起点）。
func TestCSOutboundClearsUnread(t *testing.T) {
	s, ctx := newCS(t)
	c, _ := s.ResolveContact(ctx, "webchat", "v-4", "戊", "")
	conv, _ := s.UpsertConversation(ctx, c.ID, "webchat", "webchat:v-4", "")
	_, _, _ = s.AppendInbound(ctx, conv.ID, CSInbound{Channel: "webchat", ExternalID: "m-a", Text: "问题"})
	before, _ := s.GetConversation(ctx, conv.ID)
	if before.UnreadCount != 1 {
		t.Fatalf("前置未读应为 1，得 %d", before.UnreadCount)
	}
	if _, err := s.AppendOutbound(ctx, conv.ID, "agent", "admin", "答复", ""); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetConversation(ctx, conv.ID)
	if after.UnreadCount != 0 {
		t.Fatalf("坐席回复后未读应清零，得 %d", after.UnreadCount)
	}
	if after.FirstReplyAt == 0 {
		t.Fatal("首次回复应记录 first_reply_at（SLA 起点）")
	}
}
