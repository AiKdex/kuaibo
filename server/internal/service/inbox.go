package service

import (
	"context"
	"database/sql"
	"time"
)

// ---- P1-3 收件箱 ----
// 收件箱 = 采集根目录（默认名"采集"，config inbox.dir 可改）下的文件，按入库时间倒序。
// inbox_state：0=未处理（未读）、1=已归档；归档仅标记，不移动文件（文件管理仍可管理原文件）。

// InboxStore 收件箱服务。
type InboxStore struct {
	db *sql.DB
}

// NewInboxStore 创建收件箱服务。
func NewInboxStore(db *sql.DB) *InboxStore { return &InboxStore{db: db} }

// InboxItem 收件箱条目。
type InboxItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Mime       string `json:"mime"`
	Size       int64  `json:"size"`
	State      int    `json:"inbox_state"`
	CreatedAt  int64  `json:"created_at"`
	ParentID   string `json:"parent_id"`
	ParentName string `json:"parent_name,omitempty"`
}

// dirName 收件箱目录名（默认"采集"）。
func dirName(cfg interface{ GetString(string) string }) string {
	if cfg == nil {
		return "采集"
	}
	if n := cfg.GetString("inbox.dir"); n != "" {
		return n
	}
	return "采集"
}

// List 收件箱文件列表（filter=unread|all；limit<=0 默认 50）。
func (s *InboxStore) List(ctx context.Context, spaceID, filter string, limit int, cfg interface{ GetString(string) string }) ([]InboxItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var dirID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM files WHERE space_id=? AND name=? AND kind='dir' AND parent_id IS NULL AND deleted_at IS NULL`,
		spaceID, dirName(cfg)).Scan(&dirID)
	if err != nil {
		if err == sql.ErrNoRows {
			return []InboxItem{}, nil
		}
		return nil, err
	}
	q := `SELECT id, name, mime, size, inbox_state, created_at, parent_id FROM files
	      WHERE parent_id=? AND kind='file' AND deleted_at IS NULL`
	args := []any{dirID}
	if filter == "unread" {
		q += ` AND inbox_state=0`
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []InboxItem{}
	for rows.Next() {
		var it InboxItem
		if err := rows.Scan(&it.ID, &it.Name, &it.Mime, &it.Size, &it.State, &it.CreatedAt, &it.ParentID); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

// UnreadCount 未处理数（侧栏红点）。
func (s *InboxStore) UnreadCount(ctx context.Context, spaceID string, cfg interface{ GetString(string) string }) (int, error) {
	var dirID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM files WHERE space_id=? AND name=? AND kind='dir' AND parent_id IS NULL AND deleted_at IS NULL`,
		spaceID, dirName(cfg)).Scan(&dirID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	var n int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM files WHERE parent_id=? AND kind='file' AND deleted_at IS NULL AND inbox_state=0`, dirID).
		Scan(&n)
	return n, nil
}

// Archive 归档单条。
func (s *InboxStore) Archive(ctx context.Context, id, spaceID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE files SET inbox_state=1, updated_at=? WHERE id=? AND kind='file' AND deleted_at IS NULL AND space_id=?`,
		time.Now().Unix(), id, spaceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ArchiveAll 一键归档（未处理 → 已归档）。
func (s *InboxStore) ArchiveAll(ctx context.Context, spaceID string, cfg interface{ GetString(string) string }) (int, error) {
	var dirID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM files WHERE space_id=? AND name=? AND kind='dir' AND parent_id IS NULL AND deleted_at IS NULL`,
		spaceID, dirName(cfg)).Scan(&dirID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE files SET inbox_state=1, updated_at=? WHERE parent_id=? AND kind='file' AND deleted_at IS NULL AND inbox_state=0`,
		time.Now().Unix(), dirID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
