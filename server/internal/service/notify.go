// notify.go 消息通知中心：写入（采集/导入导出/索引等终态）与读取（列表/已读/未读数）。
// 表结构见 repo/schema.go notifications：id/user_id/type/payload/read_at/created_at。
// 多用户：读接口按 user_id 隔离，管理员额外可见站点级（SystemOwnerID）通知；
// 类型：mention|subscription|space|task|comment|index|collect|impex|ingest|org.transfer。
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
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

// Add 写入一条通知（归属 SystemOwnerID；payload 支持 title/message/link/extra 结构化字段）。
func (s *NotifyStore) Add(ctx context.Context, ntype string, payload map[string]any) error {
	return s.AddUser(ctx, SystemOwnerID, ntype, payload)
}

// AddUser 写入一条**面向指定用户**的通知（多用户阶段：Org 移交流等按接手人投递）。
// userID 为空时回落到 SystemOwnerID（单用户形态兼容）。
func (s *NotifyStore) AddUser(ctx context.Context, userID, ntype string, payload map[string]any) error {
	if userID == "" {
		userID = SystemOwnerID
	}
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
	// 注意：payload 列声明是 TEXT，必须绑 string —— 绑 []byte 会被驱动存成 BLOB，
	// 那样列类型与声明不符，且 LIKE / GLOB 在 BLOB 上恒不匹配（历史行即如此）。
	raw, _ := json.Marshal(extra)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (id, user_id, type, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, userID, ntype, string(raw), time.Now().Unix())
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
		`UPDATE notifications SET payload = ? WHERE id = ?`, string(flat2), id)
	return err
}

// scopeWhere 生成「该用户可见范围」的 SQL 条件与参数（参数化，无拼接注入面）。
//
// 可见范围：
//   - 自己的通知（user_id = 自己）
//   - 管理员额外可见站点级通知（user_id = SystemOwnerID）——历史单用户数据都挂在这个 id 下，
//     采集/导入导出等终态通知也在其中，管理员需要看得到。
//
// 传入 userID 为空时回落到 SystemOwnerID（服务令牌/定时任务的兼容形态）。
func scopeWhere(userID string, includeSystem bool) (string, []any) {
	if strings.TrimSpace(userID) == "" {
		return `user_id = ?`, []any{SystemOwnerID}
	}
	if includeSystem && userID != SystemOwnerID {
		return `(user_id = ? OR user_id = ?)`, []any{userID, SystemOwnerID}
	}
	return `user_id = ?`, []any{userID}
}

// List 通知列表（新→旧）。unreadOnly=true 仅未读；limit<=0 默认 50。
//
// 多用户隔离：只返回 userID 自己的通知；includeSystem（管理员）时附带站点级通知。
// 修复此前「任何登录用户都能看到全部通知（含他人 org.transfer）」的越权问题。
func (s *NotifyStore) List(ctx context.Context, userID string, includeSystem bool, limit int, unreadOnly bool) ([]Notification, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	where, args := scopeWhere(userID, includeSystem)
	q := `SELECT id, type, payload, read_at, created_at FROM notifications WHERE ` + where
	if unreadOnly {
		q += ` AND read_at IS NULL`
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
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
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	uc, err := s.UnreadCount(ctx, userID, includeSystem)
	if err != nil {
		return nil, 0, err
	}
	return items, uc, nil
}

// MarkRead 标记单条已读。**只能标记自己可见范围内**的通知，越界当「不存在」处理。
func (s *NotifyStore) MarkRead(ctx context.Context, userID string, includeSystem bool, id string) error {
	where, args := scopeWhere(userID, includeSystem)
	q := `UPDATE notifications SET read_at = ? WHERE id = ? AND read_at IS NULL AND ` + where
	fargs := append([]any{time.Now().Unix(), id}, args...)
	res, err := s.db.ExecContext(ctx, q, fargs...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("通知不存在、已读或无权操作")
	}
	return nil
}

// MarkAllRead 把可见范围内的未读全部标记已读。
func (s *NotifyStore) MarkAllRead(ctx context.Context, userID string, includeSystem bool) error {
	where, args := scopeWhere(userID, includeSystem)
	q := `UPDATE notifications SET read_at = ? WHERE read_at IS NULL AND ` + where
	fargs := append([]any{time.Now().Unix()}, args...)
	_, err := s.db.ExecContext(ctx, q, fargs...)
	return err
}

// UnreadCount 未读数（轮询用）。
func (s *NotifyStore) UnreadCount(ctx context.Context, userID string, includeSystem bool) (int, error) {
	where, args := scopeWhere(userID, includeSystem)
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE read_at IS NULL AND `+where, args...).Scan(&n)
	return n, err
}
