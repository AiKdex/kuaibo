// Package service 提供业务编排层（文件/检索/发布/协作）与系统级服务。
// audit.go 实现追加写审计日志 + prev_hash 哈希链（防篡改，商业化企业版钩子）。
package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Rec 是一次审计记录。
type Rec struct {
	UserID    string         `json:"user_id,omitempty"`
	Action    string         `json:"action"` // login / config.change / share.create / permission.change / file.download ...
	Target    string         `json:"target,omitempty"`
	Detail    map[string]any `json:"detail,omitempty"`
	PrevHash  string         `json:"prev_hash"`
	Hash      string         `json:"hash"`
	CreatedAt int64          `json:"created_at"`
}

// Store 审计写入器。
type AuditStore struct {
	db *sql.DB
}

// New 创建审计存储。
func New(db *sql.DB) *AuditStore { return &AuditStore{db: db} }

// Append 追加一条审计记录，并自动计算哈希链。
func (s *AuditStore) Append(ctx context.Context, userID, action, target string, detail map[string]any) (Rec, error) {
	prev, err := s.lastHash(ctx)
	if err != nil {
		return Rec{}, err
	}
	payload := struct {
		UserID   string         `json:"user_id"`
		Action   string         `json:"action"`
		Target   string         `json:"target"`
		Detail   map[string]any `json:"detail"`
		PrevHash string         `json:"prev_hash"`
	}{userID, action, target, detail, prev}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	rec := Rec{
		UserID: userID, Action: action, Target: target, Detail: detail,
		PrevHash: prev, Hash: hex.EncodeToString(sum[:]), CreatedAt: nowUnix(),
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO audit_log(user_id, action, target, detail, prev_hash, hash, created_at) VALUES(?,?,?,?,?,?,?)`,
		rec.UserID, rec.Action, rec.Target, mustJSON(detail), rec.PrevHash, rec.Hash, rec.CreatedAt)
	if err != nil {
		return Rec{}, fmt.Errorf("append audit: %w", err)
	}
	return rec, nil
}

// lastHash 取链尾哈希（无记录时返回空串，作为创世链头）。
func (s *AuditStore) lastHash(ctx context.Context) (string, error) {
	var h string
	err := s.db.QueryRowContext(ctx,
		`SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&h)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return h, nil
}

// Verify 全链校验：自首条起逐条重算哈希，任何一条不匹配返回错误。
// 供管理后台/审计导出使用；O(n)，仅按需调用。
func (s *AuditStore) Verify(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, action, target, detail, prev_hash, hash, created_at FROM audit_log ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	expectedPrev := ""
	for rows.Next() {
		var id int64
		var userID, action, target, detail, prevHash, hash string
		var createdAt int64
		if err := rows.Scan(&id, &userID, &action, &target, &detail, &prevHash, &hash, &createdAt); err != nil {
			return err
		}
		if prevHash != expectedPrev {
			return fmt.Errorf("audit chain broken at row %d: prev_hash %q != expected %q", id, prevHash, expectedPrev)
		}
		// 与 Append 完全一致的方式重算：detail 先回解析为 map（键序与写入时一致）
		var detailMap map[string]any
		_ = json.Unmarshal([]byte(detail), &detailMap)
		payload := struct {
			UserID   string         `json:"user_id"`
			Action   string         `json:"action"`
			Target   string         `json:"target"`
			Detail   map[string]any `json:"detail"`
			PrevHash string         `json:"prev_hash"`
		}{userID, action, target, detailMap, prevHash}
		b, _ := json.Marshal(payload)
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != hash {
			return fmt.Errorf("audit hash mismatch at row %d", id)
		}
		expectedPrev = hash
	}
	return rows.Err()
}

func mustJSON(d map[string]any) string {
	if d == nil {
		return "{}"
	}
	b, _ := json.Marshal(d)
	return string(b)
}

func nowUnix() int64 {
	return time.Now().Unix()
}
