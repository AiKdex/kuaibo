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
	// 列迁移：能力插件协议 v2（docs/能力插件协议-v2草案.md）——capabilities/backend_entry/routes/schema 区间。
	// 注意：v2 的 kind=ui|capacity 语义落 capability_mode 列，不复用 kind（本壳 kind=plugin|theme 为应用中心
	// 安装类型分类，两者取值域冲突，见协作区 REQ-004）。
	v2Cols := []struct {
		col, ddl string
	}{
		{"capabilities", `capabilities TEXT NOT NULL DEFAULT '[]'`},
		{"backend_entry", `backend_entry TEXT NOT NULL DEFAULT ''`},
		{"routes", `routes TEXT NOT NULL DEFAULT '[]'`},
		{"min_schema", `min_schema INTEGER NOT NULL DEFAULT 0`},
		{"max_schema", `max_schema INTEGER`},
		{"capability_mode", `capability_mode TEXT NOT NULL DEFAULT 'ui'`},
	}
	for _, c := range v2Cols {
		if !hasColumn(db, "blog_plugins", c.col) {
			if _, err := db.Exec(`ALTER TABLE blog_plugins ADD COLUMN ` + c.ddl); err != nil {
				return fmt.Errorf("migrate add blog_plugins.%s: %w", c.col, err)
			}
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
	// 付费访问后端（生产站变现底座 B 项）：access_grants 凭证表
	// 支付商收款后经支付对接插件以 HMAC 转发 POST /api/v1/pay/notify，幂等签发 grant。
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS access_grants (
		id          TEXT PRIMARY KEY,
		file_id     TEXT NOT NULL,
		grantee     TEXT NOT NULL DEFAULT '',
		grant_token TEXT NOT NULL,
		source      TEXT NOT NULL DEFAULT '',
		expires_at  INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("migrate create access_grants: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_access_grants_lookup ON access_grants(file_id, grant_token)`); err != nil {
		return fmt.Errorf("migrate index access_grants: %w", err)
	}
	// 列迁移：files.access_mode（none|password|paid；内容付费网关开关）
	if !hasColumn(db, "files", "access_mode") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN access_mode TEXT NOT NULL DEFAULT 'none'`); err != nil {
			return fmt.Errorf("migrate add files.access_mode: %w", err)
		}
	}
	// 列迁移：files.price_cents（付费单价，单位：分）
	if !hasColumn(db, "files", "price_cents") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN price_cents INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("migrate add files.price_cents: %w", err)
		}
	}
	// 列迁移：files.paid_preview（付费前预览策略：none|partial|full）
	if !hasColumn(db, "files", "paid_preview") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN paid_preview TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("migrate add files.paid_preview: %w", err)
		}
	}
	// 列迁移：files.custodian_user_id（内容托管人；Org/内容移交用，可空）
	if !hasColumn(db, "files", "custodian_user_id") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN custodian_user_id TEXT`); err != nil {
			return fmt.Errorf("migrate add files.custodian_user_id: %w", err)
		}
	}
	// 列迁移：files.inbox_state（收件箱状态：0=未处理 1=已归档；Inbox 模块复用列，无独立表）
	if !hasColumn(db, "files", "inbox_state") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN inbox_state INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("migrate add files.inbox_state: %w", err)
		}
	}
	// 列迁移：blog_plugins.pack_type（应用包类别：''=普通应用/插件/主题，'source-pack'=采集源包）
	// 源包与插件同走应用中心安装链路，需可区分以便卸载时同步清理其导入的 sources（SPEC-SP-001 §6）。
	if !hasColumn(db, "blog_plugins", "pack_type") {
		if _, err := db.Exec(`ALTER TABLE blog_plugins ADD COLUMN pack_type TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("migrate add blog_plugins.pack_type: %w", err)
		}
	}
	// ---- CoreModules 模块表（上游阻塞项解除包 ddl.sql；字段与主系统一致）----
	// 采集源（SourceStore）：频道/栏目概念并入本表（上游无独立 ChannelStore）
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sources (
		id            TEXT PRIMARY KEY,
		city          TEXT NOT NULL,
		name          TEXT NOT NULL,
		kind          TEXT NOT NULL DEFAULT 'job',
		channel_type  TEXT NOT NULL DEFAULT 'job_position',
		fetch_mode    TEXT NOT NULL DEFAULT 'http',
		target_dir    TEXT,
		url           TEXT NOT NULL,
		template      TEXT NOT NULL DEFAULT '{}',
		enabled       INTEGER NOT NULL DEFAULT 1,
		last_run_at   INTEGER NOT NULL DEFAULT 0,
		last_status   TEXT,
		last_count    INTEGER NOT NULL DEFAULT 0,
		created_at    INTEGER NOT NULL,
		updated_at    INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("migrate create sources: %w", err)
	}
	// 列迁移：sources.pack_id（源包 id；SPEC-SP-001 §6：同 id 重装覆盖、卸载按 pack_id 清理）
	if !hasColumn(db, "sources", "pack_id") {
		if _, err := db.Exec(`ALTER TABLE sources ADD COLUMN pack_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("migrate add sources.pack_id: %w", err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sources_pack ON sources(pack_id)`); err != nil {
		return fmt.Errorf("migrate index sources.pack_id: %w", err)
	}
	// 采集运行记录（Collector 每轮一条；状态经 GET /collect/runs 查询）
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS collect_runs (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		city             TEXT NOT NULL,
		trigger          TEXT NOT NULL DEFAULT 'manual',
		status           TEXT NOT NULL DEFAULT 'running',
		fetched          INTEGER NOT NULL DEFAULT 0,
		created          INTEGER NOT NULL DEFAULT 0,
		skipped          INTEGER NOT NULL DEFAULT 0,
		rejected         INTEGER NOT NULL DEFAULT 0,
		failed           INTEGER NOT NULL DEFAULT 0,
		error            TEXT,
		rejected_detail  TEXT,
		started_at       INTEGER NOT NULL,
		finished_at      INTEGER
	)`); err != nil {
		return fmt.Errorf("migrate create collect_runs: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_collect_runs_city ON collect_runs(city, started_at)`); err != nil {
		return fmt.Errorf("migrate index collect_runs: %w", err)
	}
	// ---- Org 模块依赖补齐（service/org.go 已随上游包落盘，但 fork 侧缺表缺列）----
	// 组织树节点 = tags 表中 kind='org' 的行（owner_id='org' 保留字），故 tags 需 kind 列。
	if !hasColumn(db, "tags", "kind") {
		if _, err := db.Exec(`ALTER TABLE tags ADD COLUMN kind TEXT NOT NULL DEFAULT 'tag'`); err != nil {
			return fmt.Errorf("migrate add tags.kind: %w", err)
		}
	}
	// 部门空间绑定组织节点（M1：spaces.kind='team' + node_id；service/org.go:429/438/470
	// 部门空间查询直接 JOIN/筛选 spaces.node_id，缺列会 no such column 报 500）。上游权威迁移对齐。
	if !hasColumn(db, "spaces", "node_id") {
		if _, err := db.Exec(`ALTER TABLE spaces ADD COLUMN node_id TEXT`); err != nil {
			return fmt.Errorf("migrate add spaces.node_id: %w", err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS ix_spaces_node ON spaces(node_id)`); err != nil {
		return fmt.Errorf("migrate index spaces.node_id: %w", err)
	}
	// 组织任职（用户 ↔ 组织节点，带任期 since/until，历史可回溯）
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS org_memberships (
		id         TEXT PRIMARY KEY,
		user_id    TEXT NOT NULL,
		node_id    TEXT NOT NULL,
		since      INTEGER NOT NULL,
		until      INTEGER,
		role       TEXT NOT NULL DEFAULT 'member',
		created_at INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("migrate create org_memberships: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_org_memberships_user ON org_memberships(user_id, until)`); err != nil {
		return fmt.Errorf("migrate index org_memberships: %w", err)
	}
	// 移交流（离职/调岗交接：draft → previewed → executed → archived|cancelled）
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS org_transfers (
		id            TEXT PRIMARY KEY,
		type          TEXT NOT NULL,
		from_user     TEXT NOT NULL,
		to_user       TEXT NOT NULL,
		space_id      TEXT,
		node_ids      TEXT NOT NULL DEFAULT '[]',
		file_ids      TEXT NOT NULL DEFAULT '[]',
		status        TEXT NOT NULL DEFAULT 'draft',
		note          TEXT,
		created_by    TEXT NOT NULL,
		created_at    INTEGER NOT NULL,
		updated_at    INTEGER NOT NULL,
		executed_at   INTEGER,
		accepted_at   INTEGER,
		completed_at  INTEGER
	)`); err != nil {
		return fmt.Errorf("migrate create org_transfers: %w", err)
	}
	// 移交流明细（每条待移交资产一行：membership / file / space）
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS org_transfer_items (
		id             TEXT PRIMARY KEY,
		transfer_id    TEXT NOT NULL,
		asset_type     TEXT NOT NULL,
		asset_id       TEXT NOT NULL,
		asset_name     TEXT NOT NULL DEFAULT '',
		action         TEXT NOT NULL DEFAULT '',
		target_user_id TEXT,
		status         TEXT NOT NULL DEFAULT 'pending',
		note           TEXT,
		done_at        INTEGER
	)`); err != nil {
		return fmt.Errorf("migrate create org_transfer_items: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_org_transfer_items_tid ON org_transfer_items(transfer_id)`); err != nil {
		return fmt.Errorf("migrate index org_transfer_items: %w", err)
	}
	// 空间成员（Org/共享空间角色判定：owner 走 spaces.owner_id，其余成员查本表）。
	// 注：service/org.go 的部门空间查询已依赖本表，但 fork 侧此前未建——补齐以免 JOIN 报 no such table。
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS space_members (
		space_id  TEXT NOT NULL,
		user_id   TEXT NOT NULL,
		role      TEXT NOT NULL DEFAULT 'viewer', -- owner|editor|viewer
		joined_at INTEGER NOT NULL,
		PRIMARY KEY (space_id, user_id)
	)`); err != nil {
		return fmt.Errorf("migrate create space_members: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_space_members_user ON space_members(user_id)`); err != nil {
		return fmt.Errorf("migrate index space_members: %w", err)
	}
	// R2（REQ-009）：对齐上游 schema v8「双索引」——补 space_id 单列索引（PRIMARY KEY 已含 UNIQUE 约束，
	// 但显式索引便于按 space_id 反查成员/空间级授权判定；IF NOT EXISTS 对存量库无副作用）。
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_space_members_space ON space_members(space_id)`); err != nil {
		return fmt.Errorf("migrate index space_members_space: %w", err)
	}
	// [family]（REQ-010/011 自上游移植，上游 schema v7）：家族传承记录模块三表——
	// 成员/typed 关系边/MM-DD 循环纪念日；独立表按 user_id 隔离，模块卸载零残留。
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS family_members (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL,
  name       TEXT NOT NULL,
  gender     TEXT NOT NULL DEFAULT '',
  birth_date TEXT NOT NULL DEFAULT '',
  note       TEXT NOT NULL DEFAULT '',
  avatar_id  TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
)`); err != nil {
		return fmt.Errorf("migrate create family_members: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS ix_family_members_user ON family_members(user_id)`); err != nil {
		return fmt.Errorf("migrate family_members index: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS family_relations (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL,
  from_id    TEXT NOT NULL,
  to_id      TEXT NOT NULL,
  relation   TEXT NOT NULL,
  note       TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE(user_id, from_id, to_id, relation)
)`); err != nil {
		return fmt.Errorf("migrate create family_relations: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS ix_family_relations_user ON family_relations(user_id)`); err != nil {
		return fmt.Errorf("migrate family_relations index: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS family_anniversaries (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL,
  title         TEXT NOT NULL,
  kind          TEXT NOT NULL DEFAULT 'custom',
  date          TEXT NOT NULL,
  ref_member_id TEXT NOT NULL DEFAULT '',
  remind_days   INTEGER NOT NULL DEFAULT 0,
  enabled       INTEGER NOT NULL DEFAULT 1,
  created_at    INTEGER NOT NULL
)`); err != nil {
		return fmt.Errorf("migrate create family_anniversaries: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS ix_family_anniv_user ON family_anniversaries(user_id)`); err != nil {
		return fmt.Errorf("migrate family_anniversaries index: %w", err)
	}
	// 外部 WebDAV 挂载（WebDAVStore；password_enc 为加密存储，密钥见 security 配置）
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS webdav_mounts (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL,
		url          TEXT NOT NULL,
		username     TEXT NOT NULL DEFAULT '',
		password_enc TEXT NOT NULL DEFAULT '',
		created_at   INTEGER NOT NULL,
		updated_at   INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("migrate create webdav_mounts: %w", err)
	}
	// 复习队列（ReviewStore，SM-2 间隔重复）
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS review_items (
		id            TEXT PRIMARY KEY,
		user_id       TEXT NOT NULL,
		file_id       TEXT NOT NULL,
		easiness      REAL NOT NULL DEFAULT 2.5,
		interval_days INTEGER NOT NULL DEFAULT 0,
		due_at        INTEGER NOT NULL,
		reps          INTEGER NOT NULL DEFAULT 0,
		difficulty    INTEGER NOT NULL DEFAULT 3,
		last_rating   INTEGER NOT NULL DEFAULT -1,
		status        TEXT NOT NULL DEFAULT 'active',
		created_at    INTEGER NOT NULL,
		updated_at    INTEGER NOT NULL,
		UNIQUE(user_id, file_id)
	)`); err != nil {
		return fmt.Errorf("migrate create review_items: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS review_logs (
		id            TEXT PRIMARY KEY,
		user_id       TEXT NOT NULL,
		file_id       TEXT NOT NULL,
		rating        INTEGER NOT NULL,
		prev_interval INTEGER NOT NULL DEFAULT 0,
		new_interval  INTEGER NOT NULL DEFAULT 0,
		created_at    INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("migrate create review_logs: %w", err)
	}
	// ---- M0 多站点数据层（SPEC-MS-001 v1.1.0）----
	// 身份/路由表：一个 site = 一个独立站点（自定义域名 / 子域名 / 子目录）。
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sites (
		id            TEXT PRIMARY KEY,
		slug          TEXT NOT NULL UNIQUE,
		domain        TEXT NOT NULL DEFAULT '',
		subdomain     TEXT NOT NULL DEFAULT '',
		path_prefix   TEXT NOT NULL DEFAULT '',
		owner_id      TEXT NOT NULL DEFAULT '',
		status        TEXT NOT NULL DEFAULT 'active',
		created_at    INTEGER NOT NULL DEFAULT 0,
		updated_at    INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("migrate create sites: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sites_domain ON sites(domain)`); err != nil {
		return fmt.Errorf("migrate index sites.domain: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sites_subdomain ON sites(subdomain)`); err != nil {
		return fmt.Errorf("migrate index sites.subdomain: %w", err)
	}
	// per-site 配置（结构化列；v1.1.0 定稿：不复用全局 settings 的 scope）。
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS site_settings (
		site_id                     TEXT PRIMARY KEY,
		default_theme               TEXT NOT NULL DEFAULT 'default',
		allow_visitor_theme_switch  INTEGER NOT NULL DEFAULT 1,
		title                       TEXT NOT NULL DEFAULT '',
		subtitle                    TEXT NOT NULL DEFAULT '',
		locale                      TEXT NOT NULL DEFAULT 'zh-CN',
		seo_title                   TEXT NOT NULL DEFAULT '',
		seo_description             TEXT NOT NULL DEFAULT '',
		seo_keywords                TEXT NOT NULL DEFAULT '',
		ext                         TEXT NOT NULL DEFAULT '{}'
	)`); err != nil {
		return fmt.Errorf("migrate create site_settings: %w", err)
	}
	// M2 站点数授权：site_licenses（独立成表，不复用 access_grants 的单篇付费语义；SPEC-MS-001 §5）。
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS site_licenses (
		id           TEXT PRIMARY KEY,
		edition      TEXT NOT NULL,
		seats        INTEGER NOT NULL,
		license_type TEXT NOT NULL,
		expires_at   INTEGER NOT NULL DEFAULT 0,
		status       TEXT NOT NULL DEFAULT 'active',
		granted_at   INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("migrate create site_licenses: %w", err)
	}

	// files/tags 加 site_id：内容隔离核心。NOT NULL DEFAULT 'default' 使存量行自动归默认站。
	if !hasColumn(db, "files", "site_id") {
		if _, err := db.Exec(`ALTER TABLE files ADD COLUMN site_id TEXT NOT NULL DEFAULT 'default'`); err != nil {
			return fmt.Errorf("migrate add files.site_id: %w", err)
		}
	}
	if !hasColumn(db, "tags", "site_id") {
		if _, err := db.Exec(`ALTER TABLE tags ADD COLUMN site_id TEXT NOT NULL DEFAULT 'default'`); err != nil {
			return fmt.Errorf("migrate add tags.site_id: %w", err)
		}
	}

	// B17 视频转码：file_media 加 6 个转码状态列（与 transcoded_ref 配套）。
	// 幂等：hasColumn 判定，已存在则跳过；SQLite 的 ADD COLUMN 不支持 IF NOT EXISTS。
	for _, c := range []struct {
		col string
		ddl string
	}{
		{"transcode_status", "TEXT NOT NULL DEFAULT ''"},
		{"transcode_msg", "TEXT NOT NULL DEFAULT ''"},
		{"transcode_signature", "TEXT NOT NULL DEFAULT ''"},
		{"transcode_bytes", "INTEGER NOT NULL DEFAULT 0"},
		{"transcode_height", "INTEGER NOT NULL DEFAULT 0"},
		{"transcode_updated_at", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if !hasColumn(db, "file_media", c.col) {
			if _, err := db.Exec(`ALTER TABLE file_media ADD COLUMN ` + c.col + " " + c.ddl); err != nil {
				return fmt.Errorf("migrate add file_media.%s: %w", c.col, err)
			}
		}
	}
	// 默认站 seed（幂等，id='default'，domain='*'）：现有单站数据全部归属默认站，行为零变化。
	now := time.Now().Unix()
	if _, err := db.Exec(`INSERT OR IGNORE INTO sites(id, slug, domain, subdomain, path_prefix, owner_id, status, created_at)
		VALUES('default','default','*','','','','active',?)`, now); err != nil {
		return fmt.Errorf("migrate seed default site: %w", err)
	}
	// 默认站配置：default_theme 取 config 默认 'aiklog'（M3 后台可由站长改；settings 表由 config 包创建，此处不跨包查询）。
	if _, err := db.Exec(`INSERT OR IGNORE INTO site_settings(site_id, default_theme, allow_visitor_theme_switch)
		VALUES('default','aiklog',1)`); err != nil {
		return fmt.Errorf("migrate seed default site_settings: %w", err)
	}
	// 存量兜底（NOT NULL DEFAULT 已使存量归 default，此处二次确保空值归位，幂等）。
	if _, err := db.Exec(`UPDATE files SET site_id='default' WHERE site_id IS NULL OR site_id=''`); err != nil {
		return fmt.Errorf("migrate normalize files.site_id: %w", err)
	}
	if _, err := db.Exec(`UPDATE tags SET site_id='default' WHERE site_id IS NULL OR site_id=''`); err != nil {
		return fmt.Errorf("migrate normalize tags.site_id: %w", err)
	}
	// 列迁移：B43 数字商品下载控制 —— 旧库 store_order_items 无下列三列时补充。
	//   download_token  ：条目级下载令牌（真正的高熵授权凭据；下单自填邮箱并非身份凭证）
	//   download_count  ：已下载次数
	//   max_downloads   ：次数上限（0 = 不限）
	for _, c := range []struct{ col, ddl string }{
		{"download_token", `download_token TEXT NOT NULL DEFAULT ''`},
		{"download_count", `download_count INTEGER NOT NULL DEFAULT 0`},
		{"max_downloads", `max_downloads INTEGER NOT NULL DEFAULT 0`},
	} {
		if !hasColumn(db, "store_order_items", c.col) {
			if _, err := db.Exec(`ALTER TABLE store_order_items ADD COLUMN ` + c.ddl); err != nil {
				return fmt.Errorf("migrate add store_order_items.%s: %w", c.col, err)
			}
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_store_items_token ON store_order_items(download_token)`); err != nil {
		return fmt.Errorf("migrate index store_order_items.download_token: %w", err)
	}
	// Digest / Inbox / Org 无独立表：Digest 读 files+settings 汇总写 notifications；
	// Inbox 复用 files.inbox_state；Org 树存 settings 键（见 ddl.sql 注释 §7）。
	return nil
}

// Now 统一时间戳源（Unix 秒）。
func Now() int64 { return time.Now().Unix() }
