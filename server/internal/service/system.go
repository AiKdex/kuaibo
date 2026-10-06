// Package service 提供业务编排层（文件/检索/发布/协作）与系统级服务。
// 当前包含：审计（audit.go）、系统种子（system.go）。
package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/argon2"
)

// SystemOwnerID 是单用户模式的固定 owner（多用户开启后由注册流程分配）。
const SystemOwnerID = "00000000-0000-0000-0000-000000000001"

// SystemHomeSpaceID 是单用户模式的 home 空间 ID（与 EnsureSystem 种子一致）。
const SystemHomeSpaceID = "00000000-0000-0000-0000-000000000002"

// EnsureSystem 确保系统 owner 用户与 home 空间存在（幂等）。
// admin 密码来源：env AIKMAP_ADMIN_PASSWORD；未设置则生成随机密码并打印一次（仅首次创建）。
func EnsureSystem(ctx context.Context, db *sql.DB, aud *AuditStore) error {
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id=?`, SystemOwnerID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		pass := os.Getenv("AIKMAP_ADMIN_PASSWORD")
		if pass == "" {
			b := make([]byte, 9)
			_, _ = rand.Read(b)
			pass = hex.EncodeToString(b)[:16]
			fmt.Printf("==> 初始管理员密码（仅显示一次）：%s\n", pass)
		}
		hash := HashPassword(pass)
		now := nowUnix()
		if _, err := db.ExecContext(ctx, `
			INSERT INTO users(id, username, email, pass_hash, display_name, role, status, preferences, created_at, updated_at)
			VALUES(?, 'admin', '', ?, '管理员', 'owner', 'active', '{}', ?, ?)`,
			SystemOwnerID, hash, now, now); err != nil {
			return fmt.Errorf("seed admin: %w", err)
		}
	}
	var spaceExists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM spaces WHERE owner_id=?`, SystemOwnerID).Scan(&spaceExists); err != nil {
		return err
	}
	if spaceExists == 0 {
		now := nowUnix()
		if _, err := db.ExecContext(ctx, `
			INSERT INTO spaces(id, owner_id, name, kind, created_at, updated_at)
			VALUES(?, ?, '我的空间', 'home', ?, ?)`,
			SystemHomeSpaceID, SystemOwnerID, now, now); err != nil {
			return fmt.Errorf("seed home space: %w", err)
		}
	}
	if aud != nil {
		_, _ = aud.Append(ctx, SystemOwnerID, "system.ensure", "users", map[string]any{"ok": true})
	}
	return nil
}

// HashPassword 使用 Argon2id 派生密码哈希（格式：$argon2id$v=19$m=65536,t=3,p=2$salt$hash）。
func HashPassword(password string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s",
		hex.EncodeToString(salt), hex.EncodeToString(key))
}

// VerifyPassword 校验密码哈希。
// 格式：$argon2id$v=19$m=65536,t=3,p=2$<salt_hex>$<hash_hex>
func VerifyPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	// ["", "argon2id", "v=19", "m=65536,t=3,p=2", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var m, t, p int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := hex.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(t), uint32(m), uint8(p), uint32(len(want)))
	return string(got) == string(want)
}
