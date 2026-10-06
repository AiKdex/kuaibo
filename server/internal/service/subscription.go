// subscription.go 内容订阅（B6 订阅与通知）：订阅文件或目录，内容更新时向订阅者投递通知。
//
// 设计：
//   - 订阅是「用户 → 目标」的一条边，目标二型：file（单篇）/ dir（目录，其下任意文件更新都命中）。
//   - 幂等：UNIQUE(user_id,target_type,target_id) + INSERT OR IGNORE —— 重复订阅既不报错也不新增行。
//   - 不校验目标是否存在：订阅先于内容存在是合法用法；目标被删时由 PurgeTarget 清行，
//     即便残留也只会指向取不到的目标（投递侧不依赖目标可读）。
//   - 匹配口径：把「文件 id + 全部祖先目录 id」连成一条链，用 target_id IN (链) 一次查出订阅者。
//     这样 file 型（命中自身）与 dir 型（命中某个祖先）走同一条 SQL，无需分支。
//   - 通知是**增值能力**：任何一步失败都不向上报错，只少发一条通知，绝不影响正文写入。
package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// 订阅目标类型。
const (
	SubTargetFile = "file"
	SubTargetDir  = "dir"
)

// maxAncestorDepth 祖先链最大深度（防御脏数据造成的环形 parent）。
const maxAncestorDepth = 32

// DocSubscription 一条订阅。
type DocSubscription struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	CreatedAt  int64  `json:"created_at"`
}

// SubscriptionStore 订阅存储。
type SubscriptionStore struct{ db *sql.DB }

// NewSubscriptionStore 创建订阅存储。
func NewSubscriptionStore(db *sql.DB) *SubscriptionStore { return &SubscriptionStore{db: db} }

// normalizeSubTarget 校验并归一订阅目标。
func normalizeSubTarget(targetType, targetID string) (string, string, error) {
	t := strings.ToLower(strings.TrimSpace(targetType))
	if t != SubTargetFile && t != SubTargetDir {
		return "", "", errors.New("target_type 必须是 file 或 dir")
	}
	id := strings.TrimSpace(targetID)
	if id == "" {
		return "", "", errors.New("target_id 不能为空")
	}
	return t, id, nil
}

// Subscribe 订阅（幂等）；返回是否为本次新建。
func (s *SubscriptionStore) Subscribe(ctx context.Context, userID, targetType, targetID string) (bool, error) {
	if strings.TrimSpace(userID) == "" {
		return false, errors.New("需要登录用户")
	}
	t, id, err := normalizeSubTarget(targetType, targetID)
	if err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO doc_subscriptions (id, user_id, target_type, target_id, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		uuid.NewString(), userID, t, id, time.Now().Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Unsubscribe 退订（幂等）；返回是否真的删掉了一行。
func (s *SubscriptionStore) Unsubscribe(ctx context.Context, userID, targetType, targetID string) (bool, error) {
	t, id, err := normalizeSubTarget(targetType, targetID)
	if err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM doc_subscriptions WHERE user_id=? AND target_type=? AND target_id=?`,
		userID, t, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// IsSubscribed 是否已订阅该目标。
func (s *SubscriptionStore) IsSubscribed(ctx context.Context, userID, targetType, targetID string) (bool, error) {
	t, id, err := normalizeSubTarget(targetType, targetID)
	if err != nil {
		return false, err
	}
	var c int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM doc_subscriptions WHERE user_id=? AND target_type=? AND target_id=?`,
		userID, t, id).Scan(&c); err != nil {
		return false, err
	}
	return c > 0, nil
}

// ListByUser 某用户订阅的全部目标（新→旧）。
func (s *SubscriptionStore) ListByUser(ctx context.Context, userID string) ([]DocSubscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, target_type, target_id, created_at FROM doc_subscriptions
		  WHERE user_id=? ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DocSubscription{}
	for rows.Next() {
		var d DocSubscription
		if err := rows.Scan(&d.ID, &d.UserID, &d.TargetType, &d.TargetID, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CountByUser 某用户订阅数。
func (s *SubscriptionStore) CountByUser(ctx context.Context, userID string) (int, error) {
	var c int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM doc_subscriptions WHERE user_id=?`, userID).Scan(&c)
	return c, err
}

// chainIDs 返回 [fileID, 父目录 id, 祖父目录 id, ...]（不含空串，深度有上限防环）。
func (s *SubscriptionStore) chainIDs(ctx context.Context, fileID string) []string {
	if fileID == "" {
		return nil
	}
	out := []string{fileID}
	seen := map[string]bool{fileID: true}
	cur := fileID
	for i := 0; i < maxAncestorDepth; i++ {
		var pid sql.NullString
		if err := s.db.QueryRowContext(ctx, `SELECT parent_id FROM files WHERE id=?`, cur).Scan(&pid); err != nil {
			break
		}
		if !pid.Valid || pid.String == "" || seen[pid.String] {
			break
		}
		seen[pid.String] = true
		out = append(out, pid.String)
		cur = pid.String
	}
	return out
}

// SubscriberIDs 订阅了「该文件」或「其任一祖先目录」的用户 id 列表（去重）。
func (s *SubscriptionStore) SubscriberIDs(ctx context.Context, fileID string) ([]string, error) {
	chain := s.chainIDs(ctx, fileID)
	if len(chain) == 0 {
		return nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(chain)), ",")
	args := make([]any, len(chain))
	for i, v := range chain {
		args[i] = v
	}
	// chain 同时包含自身与祖先，故 file 型与 dir 型用同一条 IN 即可覆盖（见文件头说明）。
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT user_id FROM doc_subscriptions WHERE target_id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		if u != "" {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

// PurgeTarget 目标（文件或目录）被彻底删除时清掉指向它的订阅行。
func (s *SubscriptionStore) PurgeTarget(ctx context.Context, targetType, targetID string) error {
	if targetID == "" {
		return nil
	}
	if targetType == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM doc_subscriptions WHERE target_id=?`, targetID)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM doc_subscriptions WHERE target_type=? AND target_id=?`, targetType, targetID)
	return err
}

// PurgeTargets 批量清理（Purge 一个目录树时用）。
func (s *SubscriptionStore) PurgeTargets(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM doc_subscriptions WHERE target_id IN (`+ph+`)`, args...)
	return err
}

// FileChangeNotice 一次「内容变更」通知的入参。
type FileChangeNotice struct {
	FileID      string // 发生变更的文件
	EditorID    string // 变更者本人（不给自己发通知）
	Title       string // 通知标题
	Link        string // 通知跳转（SPA 路由片段，如 /read/<id>）
	DebounceSec int    // >0 时：同一 (用户,文件) 在窗口内已发过则跳过（防自动保存刷屏）
	Now         int64  // 便于测试注入；0 取当前时间
}

// DispatchFileChange 向订阅者投递「内容已更新」通知；返回实际投递人数。
//
// 契约：**永不返回错误**。通知是增值能力，任何一步失败只少发一条，不影响正文写入。
func (s *SubscriptionStore) DispatchFileChange(ctx context.Context, notify *NotifyStore, n FileChangeNotice) int {
	if notify == nil || n.FileID == "" {
		return 0
	}
	ids, err := s.SubscriberIDs(ctx, n.FileID)
	if err != nil {
		return 0
	}
	now := n.Now
	if now == 0 {
		now = time.Now().Unix()
	}
	sent := 0
	for _, uid := range ids {
		if uid == "" || uid == n.EditorID {
			continue
		}
		if n.DebounceSec > 0 && s.recentlyNotified(ctx, uid, n.FileID, now-int64(n.DebounceSec)) {
			continue
		}
		title := n.Title
		if title == "" {
			title = "订阅的内容有更新"
		}
		if err := notify.AddUser(ctx, uid, "subscription", map[string]any{
			"title":   title,
			"message": "你订阅的内容有更新",
			"link":    n.Link,
			"extra":   map[string]any{"file_id": n.FileID},
		}); err != nil {
			continue
		}
		sent++
	}
	return sent
}

// recentlyNotified 判定 (用户,文件) 在 since 之后是否已发过订阅通知。
//
// 用 instr() 而非 LIKE：notifications.payload 在历史数据里是 **BLOB**（早期 AddUser 直接绑了
// json.Marshal 的 []byte），而 LIKE / GLOB 在 BLOB 上恒不匹配。instr() 对 BLOB 与 TEXT 都成立，
// 故新旧行都能正确去重（B6 同时把 AddUser 改绑 string，新行落 TEXT）。
func (s *SubscriptionStore) recentlyNotified(ctx context.Context, userID, fileID string, since int64) bool {
	var c int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications
		  WHERE user_id=? AND type='subscription' AND created_at > ?
		    AND instr(payload, ?) > 0`,
		userID, since, `"file_id":"`+fileID+`"`).Scan(&c)
	return err == nil && c > 0
}
