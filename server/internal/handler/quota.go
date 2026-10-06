// quota.go 存储配额与空间角色（WebDAV / Org 接线所需的共享辅助）。
//
// 本壳为精简发行，没有独立配额服务；这里给出**最小可用口径**：
//   - 配额项取自站点配置（storage.quota_upload_mb / storage.quota_total_mb），0 或空 = 不限；
//   - 已用容量 = files.size 求和（按用户），不限时不查库（零开销）；
//   - 空间角色 = 属主 owner，否则查 space_members（viewer/editor/owner）。
//
// 上游若有完整配额服务，替换本文件实现即可，调用方签名不变。
package handler

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

const (
	// quotaUploadMB 单次上传大小配额项（单位 MB；WebDAV PUT 落盘前校验）。
	quotaUploadMB = "quota_upload_mb"
	// quotaTotalMB 用户累计容量配额项（单位 MB；0=不限）。
	quotaTotalMB = "quota_total_mb"
)

// quotaUse 判定用户在指定配额项上是否还有余量：ok=false 即超限。
// amount 单位与配额项一致（quotaUploadMB=MB）。配置为空/0 视为不限，恒 true。
func (a *API) quotaUse(userID, kind string, amount int64) (bool, error) {
	if amount < 0 {
		return false, nil
	}
	switch kind {
	case quotaUploadMB:
		limit := a.intSetting("storage.quota_upload_mb", 0)
		if limit <= 0 {
			return true, nil
		}
		return amount <= int64(limit), nil
	case quotaTotalMB:
		limit := a.intSetting("storage.quota_total_mb", 0)
		if limit <= 0 {
			return true, nil
		}
		var used int64
		// 已用 = 该用户名下未删除文件体积之和（字节 → MB 向上取整）
		if err := a.db.QueryRowContext(context.Background(),
			`SELECT COALESCE(SUM(size),0) FROM files WHERE owner_id=? AND deleted_at IS NULL`, userID).
			Scan(&used); err != nil && err != sql.ErrNoRows {
			return false, err
		}
		usedMB := (used + (1 << 20) - 1) / (1 << 20)
		return usedMB+amount <= int64(limit), nil
	}
	// 未知配额项：不阻断（宽容解析，与上游未知字段口径一致）
	return true, nil
}

// intSetting 读取整型站点配置（空值/非法值回落 def）。
func (a *API) intSetting(key string, def int) int {
	if a.cfg == nil {
		return def
	}
	if v := strings.TrimSpace(a.cfg.GetString(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// spaceRole 取用户在指定空间的角色（owner|editor|viewer）；ok=false 表示无访问权限。
// 判定顺序：空间属主 → space_members 显式授权。
func (a *API) spaceRole(ctx context.Context, userID, spaceID string) (string, bool) {
	if userID == "" || spaceID == "" {
		return "", false
	}
	var owner string
	if err := a.db.QueryRowContext(ctx, `SELECT owner_id FROM spaces WHERE id=?`, spaceID).Scan(&owner); err == nil {
		if owner == userID {
			return "owner", true
		}
	}
	var role string
	if err := a.db.QueryRowContext(ctx,
		`SELECT role FROM space_members WHERE space_id=? AND user_id=?`, spaceID, userID).Scan(&role); err == nil && role != "" {
		return role, true
	}
	return "", false
}
