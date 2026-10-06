// mention.go @提及（B6）：把正文/评论里的 @用户 解析、落库，并向被提及者投递通知。
//
// 语法（两种，显式优先）：
//
//	1. @[显示名](user_id)  —— 显式写法，无歧义，直接拿到 id，不做反查。
//	2. @用户名             —— 纯写法，按 users.username / users.display_name 反查（只认 active）。
//
// 设计：
//   - **解析是纯函数**（ParseMentions）：不查库、不改库，便于单测与复用（工作流/AI 侧也能用）。
//   - **落库幂等**：UNIQUE(source_type,source_id,mentioned_user_id) + INSERT OR IGNORE，
//     同一条内容重复解析不会产生重复提及，也不会重复通知（只有真正插入成功才发通知）。
//   - 语法留扩展位：显式写法保证「改名不碎链」，纯写法保证「手写方便」；两者可共存。
//   - 通知是**增值能力**：失败不阻断主流程。
package service

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// 提及来源类型。
const (
	MentionSourceComment = "comment"
	MentionSourcePost    = "post"
)

var (
	// mentionExplicitRe @[显示名](user_id)。名字允许任意非 ] 换行字符；id 取常见 id 形态。
	// 注意：Go 的 RE2 不支持 \u 转义，故用反引号原始串 + \x{...} 写法表达非 ASCII 范围。
	mentionExplicitRe = regexp.MustCompile(`@\[([^\]\n]{1,64})\]\(([0-9a-zA-Z_-]{8,64})\)`)
	// mentionPlainRe @名字。\p{L} 已含中文，\p{N} 含数字。
	mentionPlainRe = regexp.MustCompile(`@([\p{L}\p{N}_.\-]{1,32})`)
)

// Mention 一条提及记录。
type Mention struct {
	ID              string `json:"id"`
	SourceType      string `json:"source_type"`
	SourceID        string `json:"source_id"`
	MentionedUserID string `json:"mentioned_user_id"`
	MentionerID     string `json:"mentioner_id"`
	ReadAt          *int64 `json:"read_at,omitempty"`
	CreatedAt       int64  `json:"created_at"`
}

// MentionStore 提及存储。
type MentionStore struct{ db *sql.DB }

// NewMentionStore 创建提及存储。
func NewMentionStore(db *sql.DB) *MentionStore { return &MentionStore{db: db} }

// ParseMentions 解析文本里的提及。**纯函数**：不查库。
// 返回：显式给出的 user_id 列表、需要反查的名字列表；各自去重且保序。
func ParseMentions(text string) (ids []string, names []string) {
	if text == "" {
		return nil, nil
	}
	idSeen := map[string]bool{}
	for _, m := range mentionExplicitRe.FindAllStringSubmatch(text, -1) {
		id := strings.TrimSpace(m[2])
		if id != "" && !idSeen[id] {
			idSeen[id] = true
			ids = append(ids, id)
		}
	}
	nameSeen := map[string]bool{}
	for _, loc := range mentionPlainRe.FindAllStringSubmatchIndex(text, -1) {
		// 排除邮件/连续标识符里的 @（如 a@b.com、x@y_z）——RE2 无 lookbehind，故在 Go 侧判前驱字符。
		if loc[0] > 0 {
			prev, _ := utf8.DecodeLastRuneInString(text[:loc[0]])
			if unicode.IsLetter(prev) || unicode.IsDigit(prev) ||
				prev == '.' || prev == '_' || prev == '-' || prev == '@' {
				continue
			}
		}
		name := text[loc[2]:loc[3]]
		if name != "" && !nameSeen[name] {
			nameSeen[name] = true
			names = append(names, name)
		}
	}
	return ids, names
}

// ResolveNames 把名字反查成用户 id（username 或 display_name 命中即可；只认 active 用户）。
func (s *MentionStore) ResolveNames(ctx context.Context, names []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		var id string
		err := s.db.QueryRowContext(ctx,
			`SELECT id FROM users WHERE (username=? OR display_name=?) AND status='active' LIMIT 1`,
			n, n).Scan(&id)
		if err != nil || id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// MentionInput Record 的入参。
type MentionInput struct {
	SourceType  string // comment|post
	SourceID    string
	MentionerID string // 提及发起人（不给自己发通知）
	Text        string
	Title       string // 通知标题
	Link        string // 通知跳转
}

// Record 解析并落库，同时向**新产生**的被提及者投递通知；返回新落库条数。
// 契约：**永不返回致命错误** —— 提及是增值能力，失败只少记一条。
func (s *MentionStore) Record(ctx context.Context, notify *NotifyStore, in MentionInput) int {
	st := strings.TrimSpace(in.SourceType)
	if (st != MentionSourceComment && st != MentionSourcePost) || strings.TrimSpace(in.SourceID) == "" {
		return 0
	}
	ids, names := ParseMentions(in.Text)
	if len(ids) == 0 && len(names) == 0 {
		return 0
	}
	if more, err := s.ResolveNames(ctx, names); err == nil {
		ids = append(ids, more...)
	}
	seen := map[string]bool{}
	created := 0
	for _, uid := range ids {
		if uid == "" || uid == in.MentionerID || seen[uid] {
			continue
		}
		seen[uid] = true
		if !s.userActive(ctx, uid) {
			continue
		}
		res, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO mentions
			   (id, source_type, source_id, mentioned_user_id, mentioner_id, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), in.SourceType, in.SourceID, uid, in.MentionerID, time.Now().Unix())
		if err != nil {
			continue
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			continue // 已记录过：不重复通知
		}
		created++
		if notify == nil {
			continue
		}
		title := in.Title
		if title == "" {
			title = "有人在内容里提到了你"
		}
		_ = notify.AddUser(ctx, uid, "mention", map[string]any{
			"title":   title,
			"message": "有人在内容里提到了你",
			"link":    in.Link,
			"extra": map[string]any{
				"source_type": in.SourceType,
				"source_id":   in.SourceID,
				"from":        in.MentionerID,
			},
		})
	}
	return created
}

// userActive 该用户是否存在且启用（显式语法直接给 id，仍需校验，避免提及幽灵用户）。
func (s *MentionStore) userActive(ctx context.Context, uid string) bool {
	var c int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE id=? AND status='active'`, uid).Scan(&c)
	return err == nil && c > 0
}

// ListByUser 我被提及的记录（新→旧）。
func (s *MentionStore) ListByUser(ctx context.Context, userID string, limit int) ([]Mention, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, source_type, source_id, mentioned_user_id, mentioner_id, read_at, created_at
		   FROM mentions WHERE mentioned_user_id=? ORDER BY created_at DESC, id DESC LIMIT ?`,
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Mention{}
	for rows.Next() {
		var m Mention
		var readAt sql.NullInt64
		if err := rows.Scan(&m.ID, &m.SourceType, &m.SourceID, &m.MentionedUserID,
			&m.MentionerID, &readAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		if readAt.Valid {
			v := readAt.Int64
			m.ReadAt = &v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UnreadCount 我的未读提及数。
func (s *MentionStore) UnreadCount(ctx context.Context, userID string) (int, error) {
	var c int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mentions WHERE mentioned_user_id=? AND read_at IS NULL`, userID).Scan(&c)
	return c, err
}

// MarkAllRead 把我的提及全部标为已读。
func (s *MentionStore) MarkAllRead(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE mentions SET read_at=? WHERE mentioned_user_id=? AND read_at IS NULL`,
		time.Now().Unix(), userID)
	return err
}

// PurgeSource 源内容（评论/文章）被删除时清掉它的提及行。
func (s *MentionStore) PurgeSource(ctx context.Context, sourceType, sourceID string) error {
	if sourceID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM mentions WHERE source_type=? AND source_id=?`, sourceType, sourceID)
	return err
}

// PurgeSources 批量清理（文章连同其评论一起删时用）。
func (s *MentionStore) PurgeSources(ctx context.Context, sourceType string, sourceIDs []string) error {
	if len(sourceIDs) == 0 {
		return nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(sourceIDs)), ",")
	args := make([]any, 0, len(sourceIDs)+1)
	args = append(args, sourceType)
	for _, v := range sourceIDs {
		args = append(args, v)
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM mentions WHERE source_type=? AND source_id IN (`+ph+`)`, args...)
	return err
}
