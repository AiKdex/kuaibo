// Package service 客服模块（SPEC-CS-001）核心服务。
//
// 定位：客户身份图谱 + 会话线程 + 渠道适配。复用既有 files/AI/事件总线底座，
// 不新建体系（见方案 §6）。本文件承载：
//   - 入站幂等落库（同一 channel+external_id 重投 N 次仍只 1 条）
//   - 身份归一（(channel, external_id) → contact，同邮箱跨渠道合并）
//   - 会话线程 upsert（外部 thread id 归并到同一会话）
//   - 出站队列（幂等键 + 指数退避 + 客服消息窗口）
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ---- 错误 ----
var (
	ErrCSNotFound    = errors.New("cs: 记录不存在")
	ErrCSContactBlock = errors.New("cs: 联系人已屏蔽")
)

// CSNow 客服模块统一时间戳（毫秒，与全站 files.*_at 铁律一致）。
func CSNow() int64 { return time.Now().UnixMilli() }

// ---- 入站消息 ----
type CSInbound struct {
	Channel     string // email|webchat|wecom|telegram|...
	ExternalID  string // 渠道侧消息唯一 id（幂等键）；空=不做幂等
	ThreadID    string // 渠道侧线程/会话 id（邮件=根 Message-ID，IM=会话 id，挂件=visitor session）
	ExternalID2 string // 发送人渠道标识（email 地址 / openid / tg id / visitor_id）
	DisplayName string
	Subject     string
	Text        string
	HTML        string
	Meta        map[string]any
	Attachments []any
}

// ---- 联系人 ----
type CSContact struct {
	ID           string
	DisplayName  string
	Email        string
	Phone        string
	Company      string
	Note         string
	Tags         []string
	OwnerID      string
	Status       string // active|blocked|merged
	LinkedUserID string
	UpdatedAt    int64
}

// ---- 会话 ----
type CSConversation struct {
	ID               string
	ContactID        string
	Channel          string
	ExternalThreadID string
	Subject          string
	Status           string
	Priority         string
	AssigneeID       string
	UnreadCount      int
	LastMessageAt    int64
	FirstReplyAt     int64
	ResolvedAt       int64
	CreatedAt        int64
}

// ---- 消息 ----
type CSMessage struct {
	ID             string
	ConversationID string
	Direction      string // in|out
	Channel        string
	AuthorType     string // contact|agent|ai|system
	AuthorID       string
	Text           string
	HTML           string
	CreatedAt      int64
	ExternalID     string
}

// CSService 客服模块服务。
type CSService struct{ db *sql.DB }

// NewCS 构造客服服务。
func NewCS(db *sql.DB) *CSService { return &CSService{db: db} }

// DB 暴露底层句柄（渠道适配器需要）。
func (s *CSService) DB() *sql.DB { return s.db }

// ResolveContact 按 (channel, external_id) 归一联系人：
// 命中既有身份 → 返回该 contact；否则若能从身份解析出 email 且已有同邮箱 contact → 复用（跨渠道合并）；
// 都没有则新建 contact + identity。
func (s *CSService) ResolveContact(ctx context.Context, channel, externalID, displayName, email string) (*CSContact, error) {
	if channel == "" || externalID == "" {
		return nil, fmt.Errorf("cs: channel 与 external_id 必填")
	}
	now := CSNow()
	// 1) 身份命中
	var contactID string
	err := s.db.QueryRowContext(ctx,
		`SELECT contact_id FROM cs_contact_identities WHERE channel=? AND external_id=?`,
		channel, externalID).Scan(&contactID)
	if err == nil {
		return s.GetContact(ctx, contactID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	// 2) 同邮箱归一（跨渠道：先挂件后邮件 → 同一 contact）
	email = strings.ToLower(strings.TrimSpace(email))
	if email != "" {
		var byEmail string
		if e := s.db.QueryRowContext(ctx,
			`SELECT id FROM cs_contacts WHERE email=? AND status<>'merged' LIMIT 1`, email).Scan(&byEmail); e == nil {
			contactID = byEmail
		}
	}
	if contactID == "" {
		// 3) 新建
		contactID = uuid.NewString()
		name := displayName
		if name == "" {
			name = email
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO cs_contacts(id, display_name, email, status, created_at, updated_at) VALUES(?,?,?,'active',?,?)`,
			contactID, name, email, now, now); err != nil {
			return nil, err
		}
	}
	// 4) 绑定身份（唯一约束兜底并发：冲突则重查）
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO cs_contact_identities(id, contact_id, channel, external_id, display, first_seen_at, last_seen_at)
		 VALUES(?,?,?,?,?,?,?)`,
		uuid.NewString(), contactID, channel, externalID, displayName, now, now); err != nil {
		return nil, err
	}
	return s.GetContact(ctx, contactID)
}

// GetContact 取联系人。
func (s *CSService) GetContact(ctx context.Context, id string) (*CSContact, error) {
	var c CSContact
	var tags string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, display_name, email, phone, company, note, tags, owner_id, status, linked_user_id, updated_at
		 FROM cs_contacts WHERE id=?`, id).
		Scan(&c.ID, &c.DisplayName, &c.Email, &c.Phone, &c.Company, &c.Note, &tags, &c.OwnerID, &c.Status, &c.LinkedUserID, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCSNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(tags), &c.Tags)
	return &c, nil
}

// UpsertConversation 按 (channel, external_thread_id) 归并会话；空 thread 则每次新建。
func (s *CSService) UpsertConversation(ctx context.Context, contactID, channel, threadID, subject string) (*CSConversation, error) {
	now := CSNow()
	if threadID != "" {
		var id string
		err := s.db.QueryRowContext(ctx,
			`SELECT id FROM cs_conversations WHERE channel=? AND external_thread_id=?`, channel, threadID).Scan(&id)
		if err == nil {
			_, _ = s.db.ExecContext(ctx, `UPDATE cs_conversations SET last_message_at=?, status=CASE WHEN status='resolved' THEN 'open' ELSE status END WHERE id=?`, now, id)
			return s.GetConversation(ctx, id)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	id := uuid.NewString()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO cs_conversations(id, contact_id, channel, external_thread_id, subject, status, last_message_at, created_at)
		 VALUES(?,?,?,?,?,'open',?,?)`,
		id, contactID, channel, threadID, subject, now, now); err != nil {
		return nil, err
	}
	return s.GetConversation(ctx, id)
}

// GetConversation 取会话。
func (s *CSService) GetConversation(ctx context.Context, id string) (*CSConversation, error) {
	var c CSConversation
	err := s.db.QueryRowContext(ctx,
		`SELECT id, contact_id, channel, external_thread_id, subject, status, priority, assignee_id,
		        unread_count, last_message_at, first_reply_at, resolved_at, created_at
		 FROM cs_conversations WHERE id=?`, id).
		Scan(&c.ID, &c.ContactID, &c.Channel, &c.ExternalThreadID, &c.Subject, &c.Status, &c.Priority,
			&c.AssigneeID, &c.UnreadCount, &c.LastMessageAt, &c.FirstReplyAt, &c.ResolvedAt, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCSNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListConversations 收件箱列表（按 last_message_at 倒序，可按状态过滤）。
func (s *CSService) ListConversations(ctx context.Context, status string, limit int) ([]CSConversation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, contact_id, channel, external_thread_id, subject, status, priority, assignee_id,
	             unread_count, last_message_at, first_reply_at, resolved_at, created_at
	      FROM cs_conversations`
	args := []any{}
	if status != "" && status != "all" {
		q += ` WHERE status=?`
		args = append(args, status)
	}
	q += ` ORDER BY last_message_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CSConversation{}
	for rows.Next() {
		var c CSConversation
		if err := rows.Scan(&c.ID, &c.ContactID, &c.Channel, &c.ExternalThreadID, &c.Subject, &c.Status, &c.Priority,
			&c.AssigneeID, &c.UnreadCount, &c.LastMessageAt, &c.FirstReplyAt, &c.ResolvedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AppendInbound 落一条入站消息（幂等）。
// 幂等保证：同 (channel, external_id) 唯一索引 + INSERT OR IGNORE；重复投递返回 already=true。
func (s *CSService) AppendInbound(ctx context.Context, convID string, in CSInbound) (msgID string, already bool, err error) {
	now := CSNow()
	meta, _ := json.Marshal(in.Meta)
	att, _ := json.Marshal(in.Attachments)
	id := uuid.NewString()
	res, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO cs_messages(id, conversation_id, direction, channel, external_id, in_reply_to,
		    author_type, author_id, body_text, body_html, attachments, meta, created_at)
		 VALUES(?,?,'in',?,?,'','contact',?,?,?,?,?,?)`,
		id, convID, in.Channel, in.ExternalID, in.ExternalID2, in.Text, in.HTML, string(att), string(meta), now)
	if err != nil {
		return "", false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// 已存在（幂等命中）：取既有 id
		_ = s.db.QueryRowContext(ctx,
			`SELECT id FROM cs_messages WHERE channel=? AND external_id=?`, in.Channel, in.ExternalID).Scan(&id)
		return id, true, nil
	}
	// 会话计数：未读 +1、更新 last_message_at
	_, _ = s.db.ExecContext(ctx,
		`UPDATE cs_conversations SET last_message_at=?, unread_count=unread_count+1 WHERE id=?`, now, convID)
	return id, false, nil
}

// AppendOutbound 落一条出站消息（坐席/AI），并清零未读、记首次响应。
func (s *CSService) AppendOutbound(ctx context.Context, convID, authorType, authorID, text, html string) (string, error) {
	now := CSNow()
	id := uuid.NewString()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO cs_messages(id, conversation_id, direction, channel, external_id, in_reply_to,
		    author_type, author_id, body_text, body_html, attachments, meta, created_at)
		 SELECT ?,?, 'out', channel, '', '', ?,?,?,?, '[]', '{}', ? FROM cs_conversations WHERE id=?`,
		id, convID, authorType, authorID, text, html, now, convID); err != nil {
		return "", err
	}
	_, _ = s.db.ExecContext(ctx,
		`UPDATE cs_conversations SET last_message_at=?, unread_count=0,
		        first_reply_at=CASE WHEN first_reply_at=0 THEN ? ELSE first_reply_at END
		 WHERE id=?`, now, now, convID)
	return id, nil
}

// ListMessages 会话消息历史（按时间升序）。
func (s *CSService) ListMessages(ctx context.Context, convID string, limit int) ([]CSMessage, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, conversation_id, direction, channel, author_type, author_id, body_text, body_html, created_at
		 FROM cs_messages WHERE conversation_id=? ORDER BY created_at ASC, id ASC LIMIT ?`, convID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CSMessage{}
	for rows.Next() {
		var m CSMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Direction, &m.Channel, &m.AuthorType, &m.AuthorID, &m.Text, &m.HTML, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---- 出站队列 ----

// EnqueueOutbound 入队出站（幂等键去重：同 client_msg_id 只入队一次）。
func (s *CSService) EnqueueOutbound(ctx context.Context, convID, channel, idemKey string, payload map[string]any) (string, bool, error) {
	now := CSNow()
	body, _ := json.Marshal(payload)
	id := uuid.NewString()
	res, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO cs_outbound(id, conversation_id, channel, idempotency_key, payload, status, next_attempt_at, created_at, updated_at)
		 VALUES(?,?,?,?,?,'queued',?,?,?)`,
		id, convID, channel, idemKey, string(body), now, now, now)
	if err != nil {
		return "", false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var existID string
		_ = s.db.QueryRowContext(ctx, `SELECT id FROM cs_outbound WHERE idempotency_key=?`, idemKey).Scan(&existID)
		return existID, true, nil
	}
	return id, false, nil
}

// ClaimDueOutbound 取到期待发（queued/failed 且到点），置 sending，返回条目。
type CSOutbound struct {
	ID             string
	ConversationID string
	Channel        string
	IdemKey        string
	Payload        string
	Attempts       int
}
func (s *CSService) ClaimDueOutbound(ctx context.Context, limit int) ([]CSOutbound, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, conversation_id, channel, idempotency_key, payload, attempts
		 FROM cs_outbound WHERE status IN ('queued','failed') AND next_attempt_at<=?
		 ORDER BY next_attempt_at ASC LIMIT ?`, CSNow(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CSOutbound{}
	for rows.Next() {
		var o CSOutbound
		if err := rows.Scan(&o.ID, &o.ConversationID, &o.Channel, &o.IdemKey, &o.Payload, &o.Attempts); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, o := range out {
		_, _ = s.db.ExecContext(ctx, `UPDATE cs_outbound SET status='sending', updated_at=? WHERE id=?`, CSNow(), o.ID)
	}
	return out, nil
}

// MarkOutboundSent 标记已发。
func (s *CSService) MarkOutboundSent(ctx context.Context, id string) error {
	now := CSNow()
	_, err := s.db.ExecContext(ctx, `UPDATE cs_outbound SET status='sent', sent_at=?, updated_at=? WHERE id=?`, now, now, id)
	return err
}

// MarkOutboundFailed 标记失败并指数退避（2^attempts，上限 3600s）。
func (s *CSService) MarkOutboundFailed(ctx context.Context, id string, attempts int, reason string) error {
	now := CSNow()
	shift := attempts
	if shift > 12 {
		shift = 12
	}
	backoff := int64(1<<shift) * 1000
	if backoff > 3600_000 {
		backoff = 3600_000
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE cs_outbound SET status='failed', attempts=?, last_error=?, next_attempt_at=?, updated_at=? WHERE id=?`,
		attempts+1, reason, now+backoff, now, id)
	return err
}

// MarkOutboundBlocked 标记被客服窗口/策略阻断（可见，不静默）。
func (s *CSService) MarkOutboundBlocked(ctx context.Context, id, reason string) error {
	now := CSNow()
	_, err := s.db.ExecContext(ctx,
		`UPDATE cs_outbound SET status='blocked', last_error=?, updated_at=? WHERE id=?`, reason, now, id)
	return err
}

// AppendSystemNotice 往会话写一条 system 消息（失败/阻断等工作台可见提示）。
func (s *CSService) AppendSystemNotice(ctx context.Context, convID, text string) error {
	_, err := s.AppendOutbound(ctx, convID, "system", "", text, "")
	return err
}

// SetConversationStatus 更新会话状态。
func (s *CSService) SetConversationStatus(ctx context.Context, convID, status string) error {
	now := CSNow()
	resolved := int64(0)
	if status == "resolved" || status == "closed" {
		resolved = now
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE cs_conversations SET status=?, resolved_at=CASE WHEN ?=0 THEN 0 ELSE ? END WHERE id=?`,
		status, resolved, resolved, convID)
	return err
}

// ListContacts 联系人列表。
func (s *CSService) ListContacts(ctx context.Context, q string, limit int) ([]CSContact, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	sqlStr := `SELECT id, display_name, email, phone, company, note, tags, owner_id, status, linked_user_id, updated_at FROM cs_contacts`
	args := []any{}
	if q != "" {
		sqlStr += ` WHERE display_name LIKE ? OR email LIKE ?`
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	sqlStr += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CSContact{}
	for rows.Next() {
		var c CSContact
		var tags string
		if err := rows.Scan(&c.ID, &c.DisplayName, &c.Email, &c.Phone, &c.Company, &c.Note, &tags, &c.OwnerID, &c.Status, &c.LinkedUserID, &c.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tags), &c.Tags)
		out = append(out, c)
	}
	return out, rows.Err()
}
