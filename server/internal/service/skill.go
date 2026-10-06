// skill.go 技能包商业化（B9）。
//
// 与「应用中心」的关系：应用中心分发的是**站点能力**（主题 / 插件 / 站点授权），
// 技能包分发的是**可授权的能力资产**（tool / skill / workflow / extension）。
// 两者共用 entitlement 思想，但**不共用表** —— 应用中心的授权落在 site_licenses，
// 技能包落在 skill_grants，grantee 可以是 user / space / instance，维度不同。
//
// 上游立场（其 schema.go 原注释）：只留数据模型、UI 待验证后再做。本壳同策略 ——
// 本批落地「模型 + 目录只读 + 免费包自助领取 + 管理员上架/发放」，
// **不做**支付通道对接与分成结算（属商业化运营动作，不是代码缺口）。
package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SkillKinds 技能包 kind 白名单（与上游注释一致）。
var SkillKinds = []string{"tool", "skill", "workflow", "extension"}

// SkillGranteeTypes 授权对象类型白名单。
var SkillGranteeTypes = []string{"user", "space", "instance"}

// 技能包相关错误。
var (
	ErrSkillNotFound   = errors.New("技能包不存在")
	ErrSkillNotFree    = errors.New("该技能包不是免费包，无法自助领取")
	ErrSkillNotPub     = errors.New("技能包未上架")
	ErrSkillBadKind    = errors.New("kind 仅支持 tool / skill / workflow / extension")
	ErrSkillBadGrantee = errors.New("grantee_type 仅支持 user / space / instance")
)

// SkillPackage 技能包（上架条目）。
type SkillPackage struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Manifest    string `json:"manifest"`
	AuthorID    string `json:"author_id"`
	Status      string `json:"status"`
	PriceCents  int64  `json:"price_cents"`
	License     string `json:"license"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// SkillGrant 授权/分发记录。
type SkillGrant struct {
	ID          string `json:"id"`
	PackageID   string `json:"package_id"`
	GranteeType string `json:"grantee_type"`
	GranteeID   string `json:"grantee_id"`
	Status      string `json:"status"`
	Source      string `json:"source"`
	ExpiresAt   int64  `json:"expires_at"` // 0 = 永久
	CreatedAt   int64  `json:"created_at"`
}

// SkillStore 技能包数据访问。
type SkillStore struct{ db *sql.DB }

// NewSkillStore 创建技能包仓储。
func NewSkillStore(db *sql.DB) *SkillStore { return &SkillStore{db: db} }

// ValidSkillKind 校验 kind 白名单。
func ValidSkillKind(k string) bool {
	for _, v := range SkillKinds {
		if v == k {
			return true
		}
	}
	return false
}

// ValidSkillGrantee 校验 grantee_type 白名单。
func ValidSkillGrantee(t string) bool {
	for _, v := range SkillGranteeTypes {
		if v == t {
			return true
		}
	}
	return false
}

const skillPkgCols = "id, name, version, kind, title, description, manifest, author_id, status, price_cents, license, created_at, updated_at"

// rowScanner 兼容 *sql.Row 与 *sql.Rows。
type rowScanner interface{ Scan(dest ...any) error }

func scanSkillPackage(sc rowScanner) (*SkillPackage, error) {
	var p SkillPackage
	if err := sc.Scan(&p.ID, &p.Name, &p.Version, &p.Kind, &p.Title, &p.Description, &p.Manifest,
		&p.AuthorID, &p.Status, &p.PriceCents, &p.License, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPackages 列出上架技能包；kind 为空则不过滤。
func (s *SkillStore) ListPackages(ctx context.Context, kind string, limit int) ([]SkillPackage, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	q := "SELECT " + skillPkgCols + " FROM skill_packages WHERE status='published'"
	args := []any{}
	if strings.TrimSpace(kind) != "" {
		q += " AND kind=?"
		args = append(args, kind)
	}
	q += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillPackage{}
	for rows.Next() {
		p, err := scanSkillPackage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPackage 取单个技能包（不论状态）。
func (s *SkillStore) GetPackage(ctx context.Context, id string) (*SkillPackage, bool) {
	row := s.db.QueryRowContext(ctx, "SELECT "+skillPkgCols+" FROM skill_packages WHERE id=?", id)
	p, err := scanSkillPackage(row)
	if err != nil {
		return nil, false
	}
	return p, true
}

// UpsertPackage 上架/更新技能包。id 为空则新建。
func (s *SkillStore) UpsertPackage(ctx context.Context, p *SkillPackage) error {
	if !ValidSkillKind(p.Kind) {
		return ErrSkillBadKind
	}
	if p.Status == "" {
		p.Status = "draft"
	}
	if p.Version == "" {
		p.Version = "0.1.0"
	}
	if p.Manifest == "" {
		p.Manifest = "{}"
	}
	now := time.Now().UnixMilli()
	if strings.TrimSpace(p.ID) == "" {
		p.ID = uuid.NewString()
		p.CreatedAt, p.UpdatedAt = now, now
		_, err := s.db.ExecContext(ctx,
			"INSERT INTO skill_packages "+
				"(id, name, version, kind, title, description, manifest, author_id, status, price_cents, license, created_at, updated_at) "+
				"VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)",
			p.ID, p.Name, p.Version, p.Kind, p.Title, p.Description, p.Manifest,
			p.AuthorID, p.Status, p.PriceCents, p.License, p.CreatedAt, p.UpdatedAt)
		return err
	}
	p.UpdatedAt = now
	res, err := s.db.ExecContext(ctx,
		"UPDATE skill_packages SET name=?, version=?, kind=?, title=?, description=?, manifest=?, "+
			"status=?, price_cents=?, license=?, updated_at=? WHERE id=?",
		p.Name, p.Version, p.Kind, p.Title, p.Description, p.Manifest,
		p.Status, p.PriceCents, p.License, p.UpdatedAt, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSkillNotFound
	}
	return nil
}

// Grant 发放授权（幂等：UNIQUE(package_id, grantee_type, grantee_id) 命中则更新状态）。
func (s *SkillStore) Grant(ctx context.Context, packageID, granteeType, granteeID, source string, expiresAt int64) error {
	if !ValidSkillGrantee(granteeType) {
		return ErrSkillBadGrantee
	}
	if _, ok := s.GetPackage(ctx, packageID); !ok {
		return ErrSkillNotFound
	}
	if source == "" {
		source = "manual"
	}
	var exp any
	if expiresAt > 0 {
		exp = expiresAt
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO skill_grants (id, package_id, grantee_type, grantee_id, status, source, expires_at, created_at) "+
			"VALUES (?,?,?,?, 'active', ?, ?, ?) "+
			"ON CONFLICT(package_id, grantee_type, grantee_id) DO UPDATE SET "+
			"status='active', source=excluded.source, expires_at=excluded.expires_at",
		uuid.NewString(), packageID, granteeType, granteeID, source, exp, time.Now().UnixMilli())
	return err
}

// Revoke 吊销授权。
func (s *SkillStore) Revoke(ctx context.Context, packageID, granteeType, granteeID string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE skill_grants SET status='revoked' WHERE package_id=? AND grantee_type=? AND grantee_id=?",
		packageID, granteeType, granteeID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSkillNotFound
	}
	return nil
}

// ListGrants 列出某授权对象的全部**生效中**授权（active 且未过期）。
func (s *SkillStore) ListGrants(ctx context.Context, granteeType, granteeID string) ([]SkillGrant, error) {
	if !ValidSkillGrantee(granteeType) {
		return nil, ErrSkillBadGrantee
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, package_id, grantee_type, grantee_id, status, source, COALESCE(expires_at, 0), created_at "+
			"FROM skill_grants "+
			"WHERE grantee_type=? AND grantee_id=? AND status='active' AND (expires_at IS NULL OR expires_at > ?) "+
			"ORDER BY created_at DESC",
		granteeType, granteeID, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillGrant{}
	for rows.Next() {
		var g SkillGrant
		if err := rows.Scan(&g.ID, &g.PackageID, &g.GranteeType, &g.GranteeID, &g.Status,
			&g.Source, &g.ExpiresAt, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// HasGrant 判断是否持有某包的有效授权。
func (s *SkillStore) HasGrant(ctx context.Context, packageID, granteeType, granteeID string) bool {
	var n int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM skill_grants "+
			"WHERE package_id=? AND grantee_type=? AND grantee_id=? AND status='active' "+
			"AND (expires_at IS NULL OR expires_at > ?)",
		packageID, granteeType, granteeID, time.Now().UnixMilli()).Scan(&n)
	return err == nil && n > 0
}

// Claim 自助领取（仅限免费且已上架；付费包返回 ErrSkillNotFree）。
func (s *SkillStore) Claim(ctx context.Context, packageID, granteeType, granteeID string) error {
	p, ok := s.GetPackage(ctx, packageID)
	if !ok {
		return ErrSkillNotFound
	}
	if p.Status != "published" {
		return ErrSkillNotPub
	}
	if p.PriceCents > 0 {
		return ErrSkillNotFree
	}
	return s.Grant(ctx, packageID, granteeType, granteeID, "free", 0)
}

// PurgeGrants 删除某授权对象的全部授权（用户/空间注销时调用，预留）。
func (s *SkillStore) PurgeGrants(ctx context.Context, granteeType, granteeID string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM skill_grants WHERE grantee_type=? AND grantee_id=?", granteeType, granteeID)
	return err
}
