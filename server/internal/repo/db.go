// Package repo 提供 SQLite 存储访问（modernc.org/sqlite，纯 Go 无 CGO）。
// 起步单文件数据库 + WAL；规模上升后按架构 §14.5 六条纪律切换 Postgres，数据模型不变。
package repo

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Open 打开（或创建）SQLite 数据库并执行迁移。
// path: 数据库文件路径；":memory:" 用于测试。
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// modernc 驱动默认单连接更安全，但检索/写入并发场景放开连接池由 busy_timeout 兜底
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// hasColumn 检查表是否存在指定列（幂等列迁移辅助）。
func hasColumn(db *sql.DB, table, col string) bool {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == col {
			return true
		}
	}
	return false
}

// Migrate 执行全部 DDL（幂等，CREATE IF NOT EXISTS）+ 幂等列迁移。
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(DDL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// 列迁移：旧库 knowledge_edges 无 anchor 列时补充（段落级引用 K13）
	rows, err := db.Query(`PRAGMA table_info(knowledge_edges)`)
	if err != nil {
		return fmt.Errorf("migrate pragma: %w", err)
	}
	defer rows.Close()
	hasAnchor := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("migrate pragma scan: %w", err)
		}
		if name == "anchor" {
			hasAnchor = true
		}
	}
	if !hasAnchor {
		if _, err := db.Exec(`ALTER TABLE knowledge_edges ADD COLUMN anchor TEXT`); err != nil {
			return fmt.Errorf("migrate add anchor: %w", err)
		}
	}
	// 列迁移：knowledge_edges 无 file_id 列时补充（K20 AI 实体关系建边的来源文件，幂等清理用）
	if !hasColumn(db, "knowledge_edges", "file_id") {
		if _, err := db.Exec(`ALTER TABLE knowledge_edges ADD COLUMN file_id TEXT`); err != nil {
			return fmt.Errorf("migrate add edges file_id: %w", err)
		}
	}
	// 列迁移：旧库 file_ai_summaries 无 meta 列时补充（K19 入库元数据抽取）
	mrows, err := db.Query(`PRAGMA table_info(file_ai_summaries)`)
	if err != nil {
		return fmt.Errorf("migrate pragma meta: %w", err)
	}
	defer mrows.Close()
	hasMeta := false
	for mrows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := mrows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("migrate pragma meta scan: %w", err)
		}
		if name == "meta" {
			hasMeta = true
		}
	}
	if !hasMeta {
		if _, err := db.Exec(`ALTER TABLE file_ai_summaries ADD COLUMN meta TEXT NOT NULL DEFAULT '{}'`); err != nil {
			return fmt.Errorf("migrate add meta: %w", err)
		}
	}
	// 爱库录精简发行：已移除采集相关列迁移（sources / collect_runs）

	// 列迁移：文件夹整体分享（强授权）——shares 无 scope 列时补充（file=单文件|dir=目录整体）
	if !hasColumn(db, "shares", "scope") {
		if _, err := db.Exec(`ALTER TABLE shares ADD COLUMN scope TEXT NOT NULL DEFAULT 'file'`); err != nil {
			return fmt.Errorf("migrate add shares.scope: %w", err)
		}
	}
	// 列迁移：文件夹整体分享——shares 无 dir_id 列时补充（scope='dir' 时指向目录文件）
	if !hasColumn(db, "shares", "dir_id") {
		if _, err := db.Exec(`ALTER TABLE shares ADD COLUMN dir_id TEXT`); err != nil {
			return fmt.Errorf("migrate add shares.dir_id: %w", err)
		}
	}
	// 列迁移：博客插件版本兼容——blog_plugins 无 min_core_version 列时补充（协议 3.1）
	if !hasColumn(db, "blog_plugins", "min_core_version") {
		if _, err := db.Exec(`ALTER TABLE blog_plugins ADD COLUMN min_core_version TEXT NOT NULL DEFAULT '0'`); err != nil {
			return fmt.Errorf("migrate add blog_plugins.min_core_version: %w", err)
		}
	}
	// 列迁移：应用中心——blog_plugins 无 settings_schema/kind 列时补充（在线目录动态设置表单 + 插件/主题区分）
	if !hasColumn(db, "blog_plugins", "settings_schema") {
		if _, err := db.Exec(`ALTER TABLE blog_plugins ADD COLUMN settings_schema TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return fmt.Errorf("migrate add blog_plugins.settings_schema: %w", err)
		}
	}
	if !hasColumn(db, "blog_plugins", "kind") {
		if _, err := db.Exec(`ALTER TABLE blog_plugins ADD COLUMN kind TEXT NOT NULL DEFAULT 'plugin'`); err != nil {
			return fmt.Errorf("migrate add blog_plugins.kind: %w", err)
		}
	}
	// 列迁移：事件契约 v1 hooks（subscribe/publish 白名单，docs/事件总线契约.md §3）——旧库无列时补充
	if !hasColumn(db, "blog_plugins", "hooks") {
		if _, err := db.Exec(`ALTER TABLE blog_plugins ADD COLUMN hooks TEXT NOT NULL DEFAULT '{}'`); err != nil {
			return fmt.Errorf("migrate add blog_plugins.hooks: %w", err)
		}
	}
	// 列迁移：公开稳定链接 slug（第三轮反馈 2.2a：博客文章发布生成、改名不碎链）——旧库 files 无 slug 列时补充
	if !hasColumn(db, "files", "slug") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN slug TEXT`); err != nil {
			return fmt.Errorf("migrate add files.slug: %w", err)
		}
	}
	// 列迁移：files.sort_order（分类手动排序权重；NULL=动态，博客分类排序底座 A1）
	if !hasColumn(db, "files", "sort_order") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN sort_order INTEGER`); err != nil {
			return fmt.Errorf("migrate add files.sort_order: %w", err)
		}
	}
	// 列迁移：files.pin_scope / pin_order（文章置顶底座 2.1：全局/分类置顶 + 置顶内排序）
	if !hasColumn(db, "files", "pin_scope") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN pin_scope TEXT NOT NULL DEFAULT 'none'`); err != nil {
			return fmt.Errorf("migrate add files.pin_scope: %w", err)
		}
	}
	if !hasColumn(db, "files", "pin_order") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN pin_order INTEGER`); err != nil {
			return fmt.Errorf("migrate add files.pin_order: %w", err)
		}
	}
	// 列迁移：files.access_pwd（2.2 文章访问密码；sha256 哈希，NULL=无密码）
	if !hasColumn(db, "files", "access_pwd") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN access_pwd TEXT`); err != nil {
			return fmt.Errorf("migrate add files.access_pwd: %w", err)
		}
	}
	// 列迁移：files.publish_at（2.3 定时发布；毫秒时间戳，NULL=立即）
	if !hasColumn(db, "files", "publish_at") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN publish_at INTEGER`); err != nil {
			return fmt.Errorf("migrate add files.publish_at: %w", err)
		}
	}
	// 列迁移：files.view_count（阅读计数；公开页 PV 上报聚合到文章级，站长后台可见）
	if !hasColumn(db, "files", "view_count") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN view_count INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("migrate add files.view_count: %w", err)
		}
	}
	// 列迁移：files.cover / excerpt（封面图 file_id + 自定义摘要；博客列表卡片与 RSS/OG 用）
	if !hasColumn(db, "files", "cover") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN cover TEXT`); err != nil {
			return fmt.Errorf("migrate add files.cover: %w", err)
		}
	}
	if !hasColumn(db, "files", "excerpt") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN excerpt TEXT`); err != nil {
			return fmt.Errorf("migrate add files.excerpt: %w", err)
		}
	}
	// 列迁移：files.seo_title / seo_desc（单篇 SEO 元数据；SSR head 的 title/description/OG 优先取用）
	if !hasColumn(db, "files", "seo_title") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN seo_title TEXT`); err != nil {
			return fmt.Errorf("migrate add files.seo_title: %w", err)
		}
	}
	if !hasColumn(db, "files", "seo_desc") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN seo_desc TEXT`); err != nil {
			return fmt.Errorf("migrate add files.seo_desc: %w", err)
		}
	}
	// 评论表升级 v2：status（审核状态）+ guest_name（访客昵称）+ user_id 放宽可空（访客评论）。
	// SQLite 无法直接改列约束，按守卫一次性重建（仅旧库缺 status 列时执行）。
	if !hasColumn(db, "comments", "status") {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("migrate comments v2 begin: %w", err)
		}
		stmts := []string{
			`CREATE TABLE comments_v2 (
			   id         TEXT PRIMARY KEY,
			   file_id    TEXT NOT NULL REFERENCES files(id),
			   user_id    TEXT REFERENCES users(id),
			   anchor     TEXT,
			   body       TEXT NOT NULL,
			   parent_id  TEXT REFERENCES comments(id),
			   status     TEXT NOT NULL DEFAULT 'approved',
			   guest_name TEXT NOT NULL DEFAULT '',
			   created_at INTEGER NOT NULL
			 )`,
			`INSERT INTO comments_v2 (id, file_id, user_id, anchor, body, parent_id, status, guest_name, created_at)
			 SELECT id, file_id, user_id, anchor, body, parent_id, 'approved', '', created_at FROM comments`,
			`DROP TABLE comments`,
			`ALTER TABLE comments_v2 RENAME TO comments`,
		}
		for _, q := range stmts {
			if _, err := tx.Exec(q); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migrate comments v2: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrate comments v2 commit: %w", err)
		}
	}
	// Webhook 投递端点表（事件契约 v1 投递器）
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS blog_webhooks (
		id         TEXT PRIMARY KEY,
		url        TEXT NOT NULL,
		secret     TEXT NOT NULL DEFAULT '',
		topics     TEXT NOT NULL DEFAULT '[]',
		enabled    INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("migrate create blog_webhooks: %w", err)
	}
	// slug 唯一索引：列存在后无条件建（IF NOT EXISTS 幂等；新库建表后建，旧库加列后建）
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_files_slug ON files(slug) WHERE slug IS NOT NULL`); err != nil {
		return fmt.Errorf("migrate add files.slug index: %w", err)
	}
	// 多用户：文章作者归属——files 无 author_id 列时补充（空=owner；发布写入实际作者）
	if !hasColumn(db, "files", "author_id") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN author_id TEXT`); err != nil {
			return fmt.Errorf("migrate add files.author_id: %w", err)
		}
	}
	// 多用户：邮箱验证码表（注册/绑定邮箱；10 分钟有效，一次性消费）
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS email_codes (
		id         TEXT PRIMARY KEY,
		email      TEXT NOT NULL,
		code       TEXT NOT NULL,
		purpose    TEXT NOT NULL DEFAULT 'register', -- register|bind
		expires_at INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		used_at    INTEGER
	)`); err != nil {
		return fmt.Errorf("migrate create email_codes: %w", err)
	}
	return nil
}

// Now 统一时间戳源（Unix 秒）。
func Now() int64 { return time.Now().Unix() }
