-- ============================================================
-- AiKlog 同步源码包 DDL（由主系统生产库提取 + schema.go 汇编）
-- 适用：AiKlog 早期 fork 迁移 CoreModules 模块 + 支付后端
-- 日期：2026-09-19
-- ============================================================

-- 1) 采集源
CREATE TABLE IF NOT EXISTS sources (
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
);

-- 2) 采集运行记录
CREATE TABLE IF NOT EXISTS collect_runs (
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
);

-- 3) 外部 WebDAV 挂载
CREATE TABLE IF NOT EXISTS webdav_mounts (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  url         TEXT NOT NULL,
  username    TEXT NOT NULL DEFAULT '',
  password_enc TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);

-- 4) Review 复习队列（SM-2）
CREATE TABLE IF NOT EXISTS review_items (
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
);
CREATE TABLE IF NOT EXISTS review_logs (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL,
  file_id       TEXT NOT NULL,
  rating        INTEGER NOT NULL,
  prev_interval INTEGER NOT NULL DEFAULT 0,
  new_interval  INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL
);

-- 5) 付费访问凭证（支付后端）
CREATE TABLE IF NOT EXISTS access_grants (
  id         TEXT PRIMARY KEY,
  file_id    TEXT NOT NULL,
  grantee    TEXT NOT NULL DEFAULT '',
  grant_token TEXT NOT NULL,
  source     TEXT NOT NULL DEFAULT '',
  expires_at INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL DEFAULT 0
);

-- 6) files 扩展列（旧库迁移；新库直接在 CREATE TABLE 中带全）
-- 已含（schema.go 版本）：inbox_state/sort_order/pin_scope/pin_order/access_pwd/publish_at
-- 需 ALTER 补充（生产库后加）：
ALTER TABLE files ADD COLUMN access_mode TEXT NOT NULL DEFAULT 'none';
ALTER TABLE files ADD COLUMN price_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE files ADD COLUMN paid_preview TEXT NOT NULL DEFAULT '';
ALTER TABLE files ADD COLUMN custodian_user_id TEXT;
-- 注意：老库若已执行过某条 ALTER 会报 duplicate column，忽略即可（幂等迁移用 try/ignore）。

-- 7) Digest / Inbox / Org 无独立表：
--    Digest   -> 读 files + settings 汇总生成，写 notifications
--    Inbox    -> 复用 files.inbox_state（0=未处理 1=已归档）
--    Org      -> 复用 users + settings（org 树存 settings 键）
