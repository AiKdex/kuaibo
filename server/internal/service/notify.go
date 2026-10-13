// notify.go 消息通知中心：写入（采集/导入导出/索引等终态）与读取（列表/已读/未读数）。
// 表结构见 repo/schema.go notifications：id/type/payload/read_at/created_at。
// 单用户阶段：统一归属 SystemOwnerID；类型 mention|space|task|comment|index|collect|impex。
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Notification 通知条目（对外 JSON 形态）。
type Notification struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Message   string         `json:"message,omitempty"`
	Link      string         `json:"link,omitempty"`
	ReadAt    *int64         `json:"read_at,omitempty"`
	CreatedAt int64          `json:"created_at"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// NotifyStore 通知存储。
type NotifyStore struct {
	db *sql.DB
}

// NewNotifyStore 创建通知存储。
func NewNotifyStore(db *sql.DB) *NotifyStore { return &NotifyStore{db: db} }

// Add 写入一条通知（payload 支持 title/message/link/extra 结构化字段）。
func (s *NotifyStore) Add(ctx context.Context, ntype string, payload map[string]any) error {
	id := uuid.NewString()
	title, _ := payload["title"].(string)
	if title == "" {
		title = ntype
	}
	msg, _ := payload["message"].(string)
	link, _ := payload["link"].(string)
	extra := map[string]any{}
	if v, ok := payload["extra"].(map[string]any); ok {
		extra = v
	}
	raw, _ := json.Marshal(extra)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (id, user_id, type, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, SystemOwnerID, ntype, raw, time.Now().Unix())
	if err != nil {
		return err
	}
	// 冗余字段落 payload（除 extra 外平铺），供列表直接读出；保留完整结构便于扩展。
	flat := map[string]any{}
	_ = json.Unmarshal(raw, &flat)
	if title != "" {
		flat["title"] = title
	}
	if msg != "" {
		flat["message"] = msg
	}
	if link != "" {
		flat["link"] = link
	}
	flat2, _ := json.Marshal(flat)
	_, err = s.db.ExecContext(ctx,
		`UPDATE notifications SET payload = ? WHERE id = ?`, flat2, id)
	return err
}

// List 通知列表（新→旧）。unreadOnly=true 仅未读；limit<=0 默认 50。
func (s *NotifyStore) List(ctx context.Context, limit int, unreadOnly bool) ([]Notification, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, type, payload, read_at, created_at FROM notifications`
	args := []any{}
	if unreadOnly {
		q += ` WHERE read_at IS NULL`
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []Notification{}
	for rows.Next() {
		var n Notification
		var payload string
		var readAt sql.NullInt64
		if err := rows.Scan(&n.ID, &n.Type, &payload, &readAt, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		if readAt.Valid {
			v := readAt.Int64
			n.ReadAt = &v
		}
		var p map[string]any
		_ = json.Unmarshal([]byte(payload), &p)
		n.Title, _ = p["title"].(string)
		n.Message, _ = p["message"].(string)
		n.Link, _ = p["link"].(string)
		n.Extra = p
		items = append(items, n)
	}
	uc, err := s.UnreadCount(ctx)
	if err != nil {
		return nil, 0, err
	}
	return items, uc, nil
}

// MarkRead 标记单条已读。
func (s *NotifyStore) MarkRead(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = ? WHERE id = ? AND read_at IS NULL`,
		time.Now().Unix(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("通知不存在或已读")
	}
	return nil
}

// MarkAllRead 全部标记已读。
func (s *NotifyStore) MarkAllRead(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = ? WHERE read_at IS NULL`, time.Now().Unix())
	return err
}

// UnreadCount 未读数（轮询用）。
func (s *NotifyStore) UnreadCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE read_at IS NULL`).Scan(&n)
	return n, err
}
