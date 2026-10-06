package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ---- A3 SRS 主动复习（SM-2 改良间隔重复）----
// 依据《AI知识管理产品落地设计文档》§3.3/§4.2.2：
//   - 基础间隔序列 1→3→7→14→30→60（由倍增系数自然产生）；
//   - 难度系数：AI 评估难度 1-5，难度越高间隔越短（1→1.2x … 5→0.8x）；
//   - 反馈评分：0=完全忘了→重置 1 天；1=有点难→×0.8；2=刚好→×2.0；3=太简单→×3.0。
// 表：review_items（条目）/ review_logs（轨迹），schema.go 幂等建表。

// ReviewStore 复习队列数据访问。
type ReviewStore struct {
	db *sql.DB
}

// NewReviewStore 创建复习服务。
func NewReviewStore(db *sql.DB) *ReviewStore { return &ReviewStore{db: db} }

// ReviewItem 复习条目（含文件摘要信息）。
type ReviewItem struct {
	ID           string `json:"id"`
	FileID       string `json:"file_id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	IntervalDays int    `json:"interval_days"`
	DueAt        int64  `json:"due_at"`
	Reps         int    `json:"reps"`
	Difficulty   int    `json:"difficulty"`
	LastRating   int    `json:"last_rating"`
	Status       string `json:"status"`
}

// ReviewStats 复习统计。
type ReviewStats struct {
	Total       int `json:"total"`        // 复习条目总数
	DueToday    int `json:"due_today"`    // 今日待复习
	Done        int `json:"done"`         // 累计已复习次数
	Paused      int `json:"paused"`       // 已暂停条目
	AvgInterval int `json:"avg_interval"` // 平均间隔（天）
}

// NextInterval 计算下一次复习间隔（天）。纯函数，可单测。
// rating: 0=完全忘了 1=有点难 2=刚好 3=太简单；difficulty: 1-5（难度越高间隔越短）。
func NextInterval(prev int, rating int, difficulty int) int {
	if rating <= 0 {
		return 1
	}
	base := prev
	if base < 1 {
		base = 1
	}
	switch rating {
	case 1:
		base = int(float64(base) * 0.8)
	case 2:
		base = int(float64(base) * 2.0)
	case 3:
		base = int(float64(base) * 3.0)
	}
	if difficulty >= 1 && difficulty <= 5 {
		f := 1.3 - 0.1*float64(difficulty) // 难度1→1.2x，难度5→0.8x
		base = int(float64(base) * f)
	}
	if base < 1 {
		base = 1
	}
	return base
}

// Enroll 将文件加入复习队列（幂等；已存在则不动）。
// 难度兜底：优先取加工流水线已评估的难度（file_ai_summaries.meta.difficulty），未评估默认 3。
func (r *ReviewStore) Enroll(ctx context.Context, userID, fileID string) error {
	var cnt int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM review_items WHERE user_id=? AND file_id=?`, userID, fileID).Scan(&cnt)
	if err != nil || cnt > 0 {
		return err
	}
	difficulty := 3
	var metaJSON string
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(meta,'{}') FROM file_ai_summaries WHERE file_id=? AND status='done'`,
		fileID).Scan(&metaJSON); err == nil && metaJSON != "" {
		var meta struct {
			Difficulty int `json:"difficulty"`
		}
		if json.Unmarshal([]byte(metaJSON), &meta) == nil && meta.Difficulty >= 1 && meta.Difficulty <= 5 {
			difficulty = meta.Difficulty
		}
	}
	now := time.Now().Unix()
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO review_items (id, user_id, file_id, due_at, difficulty, created_at, updated_at) VALUES (?,?,?,0,?,?,?)`,
		uuid.NewString(), userID, fileID, difficulty, now, now)
	return err
}

// Queue 今日待复习清单（due_at<=now 且未暂停；due_at=0 的首次条目纳入）。
func (r *ReviewStore) Queue(ctx context.Context, userID string, limit int) ([]ReviewItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	now := time.Now().Unix()
	rows, err := r.db.QueryContext(ctx, `
		SELECT ri.id, ri.file_id, f.name, f.kind, ri.interval_days, ri.due_at,
		       ri.reps, ri.difficulty, ri.last_rating, ri.status
		FROM review_items ri JOIN files f ON f.id = ri.file_id
		WHERE ri.user_id=? AND ri.status='active' AND (ri.due_at<=? OR ri.due_at=0)
		ORDER BY ri.due_at ASC, ri.created_at ASC LIMIT ?`, userID, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ReviewItem{}
	for rows.Next() {
		var it ReviewItem
		if err := rows.Scan(&it.ID, &it.FileID, &it.Name, &it.Kind, &it.IntervalDays,
			&it.DueAt, &it.Reps, &it.Difficulty, &it.LastRating, &it.Status); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Rate 提交复习反馈并推进间隔（幂等：同文件同秒只记一次）。
func (r *ReviewStore) Rate(ctx context.Context, userID, fileID string, rating int) (*ReviewItem, error) {
	if rating < 0 || rating > 3 {
		rating = 2
	}
	now := time.Now().Unix()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var it ReviewItem
	err = tx.QueryRowContext(ctx,
		`SELECT id, interval_days, reps, difficulty FROM review_items WHERE user_id=? AND file_id=?`,
		userID, fileID).Scan(&it.ID, &it.IntervalDays, &it.Reps, &it.Difficulty)
	if err == sql.ErrNoRows {
		// 未入队则自动入队（评分即首次复习）
		it.ID = uuid.NewString()
		it.IntervalDays = 0
		it.Reps = 0
		it.Difficulty = 3
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO review_items (id, user_id, file_id, due_at, created_at, updated_at) VALUES (?,?,?,0,?,?)`,
			it.ID, userID, fileID, now, now); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	newInterval := NextInterval(it.IntervalDays, rating, it.Difficulty)
	due := now + int64(newInterval)*86400
	if _, err = tx.ExecContext(ctx,
		`UPDATE review_items SET interval_days=?, due_at=?, reps=reps+1, last_rating=?, updated_at=? WHERE id=?`,
		newInterval, due, rating, now, it.ID); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO review_logs (id, user_id, file_id, rating, prev_interval, new_interval, created_at) VALUES (?,?,?,?,?,?,?)`,
		uuid.NewString(), userID, fileID, rating, it.IntervalDays, newInterval, now); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &ReviewItem{
		ID:           it.ID,
		FileID:       fileID,
		IntervalDays: newInterval,
		DueAt:        due,
		Reps:         it.Reps + 1,
		Difficulty:   it.Difficulty,
		LastRating:   rating,
		Status:       "active",
	}, nil
}

// SetDifficulty 写入 AI 难度评估（A2 加工流水线接入）。
func (r *ReviewStore) SetDifficulty(ctx context.Context, userID, fileID string, difficulty int) error {
	if difficulty < 1 || difficulty > 5 {
		difficulty = 3
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE review_items SET difficulty=?, updated_at=? WHERE user_id=? AND file_id=?`,
		difficulty, time.Now().Unix(), userID, fileID)
	return err
}

// SetStatus 暂停/恢复复习。
func (r *ReviewStore) SetStatus(ctx context.Context, userID, fileID, status string) error {
	if status != "active" && status != "paused" {
		status = "active"
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE review_items SET status=?, updated_at=? WHERE user_id=? AND file_id=?`,
		status, time.Now().Unix(), userID, fileID)
	return err
}

// Stats 复习统计。
func (r *ReviewStore) Stats(ctx context.Context, userID string) (*ReviewStats, error) {
	s := &ReviewStats{}
	now := time.Now().Unix()
	_ = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN due_at<=? OR due_at=0 THEN 1 ELSE 0 END),0),
		        COALESCE(SUM(CASE WHEN status='paused' THEN 1 ELSE 0 END),0),
		        COALESCE(AVG(interval_days),0)
		 FROM review_items WHERE user_id=?`, now, userID).
		Scan(&s.Total, &s.DueToday, &s.Paused, &s.AvgInterval)
	_ = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM review_logs WHERE user_id=?`, userID).Scan(&s.Done)
	s.AvgInterval = int(s.AvgInterval)
	return s, nil
}
