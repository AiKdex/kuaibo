// Package service 的 site.go 实现多站点数据访问（SPEC-MS-001 M0）。
//
// 设计要点：
//   - 一个 Site = 一个独立站点（独立域名 / 子域名 / 子目录），内容(files)/分类(tags)按 site 隔离。
//   - sites 表只承载身份与路由（不混配置）；per-site 配置走结构化 site_settings 表（v1.1.0 定稿）。
//   - 默认站 id='default'、domain='*'：单站点（未购授权）场景完全等价现状，行为零变化。
//   - ResolveSite 实现 §4.1 解析优先级：自定义域名 > 子域名 > 子目录 > 默认站。
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

// defaultSiteID 默认站固定 id；存量单站数据全部归属它。
const defaultSiteID = "default"

// DefaultSiteID 默认站 id（站点数授权 base 之外的基础站；write 路径回落值）。
const DefaultSiteID = "default"

// Site 站点（身份/路由）。
type Site struct {
	ID         string `json:"id"`
	Slug       string `json:"slug"`
	Domain     string `json:"domain"`     // 自定义域名（精确匹配）；默认站为 '*'
	Subdomain  string `json:"subdomain"`  // 子域名前缀（配合平台 base domain，如 'shop' → shop.aiklog.cn）
	PathPrefix string `json:"path_prefix"` // 同域名子目录（如 '/docs'；运维层 rewrite 注入）
	OwnerID    string `json:"owner_id"`
	Status     string `json:"status"` // active | suspended | deleted
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at,omitempty"`
}

// SiteSettings per-site 配置（结构化列，v1.1.0 定稿：不复用全局 settings 的 scope）。
type SiteSettings struct {
	SiteID                   string          `json:"site_id"`
	DefaultTheme             string          `json:"default_theme"`              // 站长设的默认主题
	AllowVisitorThemeSwitch  bool            `json:"allow_visitor_theme_switch"` // false=关访客切换、强制 default_theme
	Title                    string          `json:"title"`
	Subtitle                 string          `json:"subtitle"`
	Locale                   string          `json:"locale"`
	SEOTitle                 string          `json:"seo_title"`
	SEODescription           string          `json:"seo_description"`
	SEOKeywords              string          `json:"seo_keywords"`
	Ext                      json.RawMessage `json:"ext"` // 兜底：logo/favicon/备案号/自定义CSS 等
}

// SiteStore 多站点数据访问（M0）。
type SiteStore struct {
	db *sql.DB
}

// NewSiteStore 创建 SiteStore。
func NewSiteStore(db *sql.DB) *SiteStore {
	return &SiteStore{db: db}
}

const siteCols = "id, slug, domain, subdomain, path_prefix, owner_id, status, created_at"

// scanner 是 *sql.Row / *sql.Rows 共有的 Scan 接口，便于复用 scanSite。
type scanner interface{ Scan(dest ...interface{}) error }

func scanSite(s scanner) (*Site, error) {
	site := &Site{}
	if err := s.Scan(&site.ID, &site.Slug, &site.Domain, &site.Subdomain, &site.PathPrefix, &site.OwnerID, &site.Status, &site.CreatedAt); err != nil {
		return nil, err
	}
	return site, nil
}

func scanSiteSettings(s scanner) (*SiteSettings, error) {
	ss := &SiteSettings{}
	var allow int
	var ext []byte
	if err := s.Scan(&ss.SiteID, &ss.DefaultTheme, &allow, &ss.Title, &ss.Subtitle, &ss.Locale, &ss.SEOTitle, &ss.SEODescription, &ss.SEOKeywords, &ext); err != nil {
		return nil, err
	}
	ss.AllowVisitorThemeSwitch = allow != 0
	ss.Ext = ext // json.RawMessage 底层为 []byte，可直接赋值
	return ss, nil
}

// EnsureDefaultSite 幂等创建默认站 + 默认站配置（升级兼容；存量数据归默认站）。
func (s *SiteStore) EnsureDefaultSite(ctx context.Context) error {
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO sites(id, slug, domain, subdomain, path_prefix, owner_id, status, created_at)
		VALUES(?,?,?,?,?,?,?,?)`, defaultSiteID, defaultSiteID, "*", "", "", "", "active", now); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO site_settings(site_id, default_theme, allow_visitor_theme_switch)
		VALUES(?,?,?)`, defaultSiteID, "aiklog", 1); err != nil {
		return err
	}
	return nil
}

// CreateSite 新建站点（slug 唯一；id 自动生成；自动初始化该站配置）。
func (s *SiteStore) CreateSite(ctx context.Context, site *Site) (*Site, error) {
	if site.Slug == "" {
		return nil, errors.New("service: site slug is required")
	}
	if site.ID == "" {
		site.ID = uuid.NewString()
	}
	if site.Status == "" {
		site.Status = "active"
	}
	now := time.Now().Unix()
	site.CreatedAt = now
	site.UpdatedAt = now
	if _, err := s.db.ExecContext(ctx, `INSERT INTO sites(id, slug, domain, subdomain, path_prefix, owner_id, status, created_at)
		VALUES(?,?,?,?,?,?,?,?)`, site.ID, site.Slug, site.Domain, site.Subdomain, site.PathPrefix, site.OwnerID, site.Status, now); err != nil {
		return nil, err
	}
	// 初始化该站配置（继承默认主题）。
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO site_settings(site_id, default_theme, allow_visitor_theme_switch)
		VALUES(?,?,?)`, site.ID, "aiklog", 1); err != nil {
		return nil, err
	}
	return site, nil
}

// GetSite 按 id 取站（含 deleted 也取到，调用方自行判 status）；未命中返回 sql.ErrNoRows。
func (s *SiteStore) GetSite(ctx context.Context, id string) (*Site, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+siteCols+` FROM sites WHERE id=?`, id)
	return scanSite(row)
}

// ListSites 列出非删除站（按创建时间升序）。
func (s *SiteStore) ListSites(ctx context.Context) ([]*Site, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+siteCols+` FROM sites WHERE status!='deleted' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Site
	for rows.Next() {
		site, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, site)
	}
	return out, rows.Err()
}

// UpdateSite 更新站点身份/路由字段。
func (s *SiteStore) UpdateSite(ctx context.Context, site *Site) error {
	if site.ID == "" {
		return errors.New("service: site id required")
	}
	if site.Status == "" {
		site.Status = "active"
	}
	site.UpdatedAt = time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `UPDATE sites SET slug=?, domain=?, subdomain=?, path_prefix=?, owner_id=?, status=?, updated_at=? WHERE id=?`,
		site.Slug, site.Domain, site.Subdomain, site.PathPrefix, site.OwnerID, site.Status, site.UpdatedAt, site.ID)
	return err
}

// DeleteSite 软删（status='deleted'），保留数据。
func (s *SiteStore) DeleteSite(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sites SET status='deleted', updated_at=? WHERE id=?`, time.Now().Unix(), id)
	return err
}

// CountActiveSites 统计非删除站数量（cap 闸门用）。
func (s *SiteStore) CountActiveSites(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sites WHERE status!='deleted'`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// GetSiteSettings 取 per-site 配置；未命中返回 sql.ErrNoRows。
func (s *SiteStore) GetSiteSettings(ctx context.Context, siteID string) (*SiteSettings, error) {
	row := s.db.QueryRowContext(ctx, `SELECT site_id, default_theme, allow_visitor_theme_switch, title, subtitle, locale, seo_title, seo_description, seo_keywords, ext
		FROM site_settings WHERE site_id=?`, siteID)
	return scanSiteSettings(row)
}

// UpsertSiteSettings 写入（覆盖）per-site 配置。
func (s *SiteStore) UpsertSiteSettings(ctx context.Context, ss *SiteSettings) error {
	if ss.SiteID == "" {
		return errors.New("service: site_id required")
	}
	if ss.DefaultTheme == "" {
		ss.DefaultTheme = "default"
	}
	if ss.Locale == "" {
		ss.Locale = "zh-CN"
	}
	if len(ss.Ext) == 0 {
		ss.Ext = json.RawMessage(`{}`)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO site_settings(site_id, default_theme, allow_visitor_theme_switch, title, subtitle, locale, seo_title, seo_description, seo_keywords, ext)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(site_id) DO UPDATE SET
			default_theme=excluded.default_theme,
			allow_visitor_theme_switch=excluded.allow_visitor_theme_switch,
			title=excluded.title,
			subtitle=excluded.subtitle,
			locale=excluded.locale,
			seo_title=excluded.seo_title,
			seo_description=excluded.seo_description,
			seo_keywords=excluded.seo_keywords,
			ext=excluded.ext`,
		ss.SiteID, ss.DefaultTheme, boolToInt(ss.AllowVisitorThemeSwitch), ss.Title, ss.Subtitle, ss.Locale, ss.SEOTitle, ss.SEODescription, ss.SEOKeywords, string(ss.Ext))
	return err
}

// ResolveSite 按 Host/Path 解析站点（SPEC-MS-001 §4.1 优先级：自定义域名 > 子域名 > 子目录 > 默认站）。
// host：请求 Host 头（可带端口）；path：请求路径（含 §4.4 注入头 X-Aiklog-Site-Path 时由调用方传入）；
// baseDomain：平台基础域名（如 aiklog.cn），用于从 Host 剥离取子域名，未配置可传空。
func (s *SiteStore) ResolveSite(ctx context.Context, host, path, baseDomain string) (*Site, error) {
	host = stripPort(strings.ToLower(strings.TrimSpace(host)))
	if host != "" {
		if site, err := s.lookup(ctx, `SELECT `+siteCols+` FROM sites WHERE domain=? AND status='active'`, host); err == nil && site != nil {
			return site, nil
		}
	}
	if host != "" && baseDomain != "" {
		baseDomain = strings.ToLower(strings.TrimSpace(baseDomain))
		prefix := host
		if baseDomain != "" && strings.HasSuffix(prefix, "."+baseDomain) {
			prefix = strings.TrimSuffix(prefix, "."+baseDomain)
		} else if prefix == baseDomain {
			prefix = ""
		}
		if prefix != "" && prefix != host {
			if site, err := s.lookup(ctx, `SELECT `+siteCols+` FROM sites WHERE subdomain=? AND status='active'`, prefix); err == nil && site != nil {
				return site, nil
			}
		}
	}
	if seg := firstPathSegment(path); seg != "" {
		if site, err := s.lookup(ctx, `SELECT `+siteCols+` FROM sites WHERE trim(path_prefix,'/')=? AND status='active'`, seg); err == nil && site != nil {
			return site, nil
		}
	}
	return s.GetSite(ctx, defaultSiteID)
}

// lookup 单行查询，未命中（sql.ErrNoRows）返回 nil, nil，便于 ResolveSite 链式尝试。
func (s *SiteStore) lookup(ctx context.Context, query string, args ...interface{}) (*Site, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
	site, err := scanSite(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return site, nil
}

// stripPort 去除 Host 头的端口（IPv6 简化处理）。
func stripPort(h string) string {
	if h == "" {
		return h
	}
	if i := strings.LastIndexByte(h, ':'); i >= 0 {
		// IPv6 形如 [::1]:8080 或 ::1；仅当非 IPv6 字面量时去除末尾端口
		if !strings.HasPrefix(h, "[") && strings.Count(h, ":") == 1 {
			return h[:i]
		}
	}
	return h
}

// firstPathSegment 取路径首段（去前导斜杠），如 "/docs/page" → "docs"。
func firstPathSegment(p string) string {
	p = strings.TrimPrefix(strings.TrimSpace(p), "/")
	if p == "" {
		return ""
	}
	if i := strings.IndexByte(p, '/'); i >= 0 {
		p = p[:i]
	}
	return p
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ─────────────────────────────────────────────────────────────────────────────
// M2 站点数授权（SPEC-MS-001 §5）：site_licenses 表 + cap 计算。
// cap = 1（基础免费） + Σ(生效 permanent.seats) + Σ(生效 subscription.seats)。
// ─────────────────────────────────────────────────────────────────────────────

// SiteLicense 一张站点数授权（permanent 买断 / subscription 订阅期有效）。
type SiteLicense struct {
	ID          string `json:"id"`           // 授权唯一 id（通常来自签发的 jti）
	Edition     string `json:"edition"`      // 档位标识：tier3 | tier10 | addon1 | sub_year …
	Seats       int    `json:"seats"`        // 该授权提供的站点数
	LicenseType string `json:"license_type"` // permanent | subscription
	ExpiresAt   int64  `json:"expires_at"`   // subscription 用；permanent=0
	Status      string `json:"status"`       // active | revoked
	GrantedAt   int64  `json:"granted_at"`
}

const siteLicenseCols = "id, edition, seats, license_type, expires_at, status, granted_at"

func scanSiteLicense(s scanner) (*SiteLicense, error) {
	l := &SiteLicense{}
	if err := s.Scan(&l.ID, &l.Edition, &l.Seats, &l.LicenseType, &l.ExpiresAt, &l.Status, &l.GrantedAt); err != nil {
		return nil, err
	}
	return l, nil
}

// GrantSiteLicense 幂等签发/更新一张站点数授权（按 id 冲突覆盖）。
func (s *SiteStore) GrantSiteLicense(ctx context.Context, l *SiteLicense) error {
	if l.ID == "" {
		return errors.New("service: license id required")
	}
	if l.Status == "" {
		l.Status = "active"
	}
	if l.GrantedAt == 0 {
		l.GrantedAt = time.Now().Unix()
	}
	if l.Seats < 0 {
		l.Seats = 0
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO site_licenses(id, edition, seats, license_type, expires_at, status, granted_at)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			edition=excluded.edition, seats=excluded.seats, license_type=excluded.license_type,
			expires_at=excluded.expires_at, status=excluded.status, granted_at=excluded.granted_at`,
		l.ID, l.Edition, l.Seats, l.LicenseType, l.ExpiresAt, l.Status, l.GrantedAt)
	return err
}

// ListSiteLicenses 列出全部站点数授权（含已吊销，便于后台展示）。
func (s *SiteStore) ListSiteLicenses(ctx context.Context) ([]*SiteLicense, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+siteLicenseCols+` FROM site_licenses ORDER BY granted_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SiteLicense
	for rows.Next() {
		l, err := scanSiteLicense(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SiteCap 计算当前实例的站点数上限与明细（SPEC-MS-001 §5.1）。
// 返回：base(恒 1) / permanentSeats / subscriptionSeats(仅生效中) / totalCap。
// subscription 过期（expires_at>0 且 <=now）或 status!='active' 的额度不计入。
func (s *SiteStore) SiteCap(ctx context.Context) (base, perm, sub, total int, err error) {
	base = 1 // 基础免费档：永远允许 1 个站（默认站）
	rows, qerr := s.db.QueryContext(ctx, `SELECT seats, license_type, expires_at, status FROM site_licenses`)
	if qerr != nil {
		return base, 0, 0, base, qerr
	}
	defer rows.Close()
	now := time.Now().Unix()
	for rows.Next() {
		var seats int
		var ltype, status string
		var exp int64
		if err := rows.Scan(&seats, &ltype, &exp, &status); err != nil {
			return base, perm, sub, total, err
		}
		if status != "active" {
			continue
		}
		if ltype == "subscription" && exp > 0 && exp <= now {
			continue // 订阅已过期，临时额度移除
		}
		if ltype == "subscription" {
			sub += seats
		} else {
			perm += seats
		}
	}
	total = base + perm + sub
	return base, perm, sub, total, nil
}

// SiteCapSummary 聚合 cap 与已用，供后台与验收端点使用。
func (s *SiteStore) SiteCapSummary(ctx context.Context) (base, perm, sub, total, used int, err error) {
	base, perm, sub, total, err = s.SiteCap(ctx)
	if err != nil {
		return
	}
	used, err = s.CountActiveSites(ctx)
	return
}
