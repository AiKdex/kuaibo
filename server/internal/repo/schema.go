// Package repo 提供 SQLite 存储访问。
// schema.go 定义 AiKmap 的 SQLite 数据模型。
// 阶段 0 目标：数据层前置预留（owner_id/space_id/users/audit_log/settings/jobs），
// 功能层按实施落地文档 §7.10 的节奏逐步开启，绝不留"后期重写"的债。
package repo

// DDL 是全部表结构的唯一权威来源（迁移命令按需引用）。
// 说明：
//   - 所有实体表均带 owner_id / space_id 双维度定位（实施文档 §7.4），单用户时 owner_id 恒为系统 owner。
//   - audit_log 为追加写、带 prev_hash 哈希链（防篡改，商业化企业版钩子）。
//   - settings 表承载"配置即变量"（实施文档 §10.5）：scope = instance|space|user。
//   - jobs 表是持久任务队列（索引/转写/缩略图），与事件总线（进程内 pub/sub）边界：
//     "触发副作用"走 jobs（幂等、可重试、崩溃可恢复）；"通知"走事件总线（fire-and-forget）。
const DDL = `
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

-- 用户
CREATE TABLE IF NOT EXISTS users (
  id            TEXT PRIMARY KEY,              -- uuid
  username      TEXT NOT NULL UNIQUE,
  email         TEXT,
  pass_hash     TEXT NOT NULL,                 -- Argon2id
  display_name  TEXT,
  avatar        TEXT,
  role          TEXT NOT NULL DEFAULT 'owner', -- owner|admin|member|viewer
  status        TEXT NOT NULL DEFAULT 'active',-- active|disabled|invited
  preferences   TEXT NOT NULL DEFAULT '{}',    -- {readingSize, readingFont, readingBg, theme} JSON
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);

-- 空间/工作组（多用户载体，单用户时只有一个 home 空间）
CREATE TABLE IF NOT EXISTS spaces (
  id           TEXT PRIMARY KEY,
  owner_id     TEXT NOT NULL REFERENCES users(id),
  name         TEXT NOT NULL,
  kind         TEXT NOT NULL DEFAULT 'home',   -- home|shared|team
  quota_bytes  INTEGER,                        -- NULL = 不限
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);

-- 文件：file_id 与路径解耦；内容留在存储后端，系统只存元数据与索引（受管多后端）
CREATE TABLE IF NOT EXISTS files (
  id             TEXT PRIMARY KEY,             -- file_id（对外稳定引用）
  space_id       TEXT NOT NULL REFERENCES spaces(id),
  owner_id       TEXT NOT NULL REFERENCES users(id),
  parent_id      TEXT REFERENCES files(id),    -- 目录树（NULL=根）
  name           TEXT NOT NULL,
  kind           TEXT NOT NULL DEFAULT 'file', -- file|dir
  mime           TEXT,
  size           INTEGER NOT NULL DEFAULT 0,
  sha256         TEXT,
  storage_backend TEXT NOT NULL DEFAULT 'local', -- local|nas|s3|oss|webdav
  storage_ref    TEXT NOT NULL,                -- 后端内的路径/引用
  version        INTEGER NOT NULL DEFAULT 1,
  content_state  TEXT NOT NULL DEFAULT '{"visibility":"private","status":"published"}',
  slug           TEXT,                         -- 公开稳定链接（博客文章发布生成；文件名/分类改名不碎链）
  sort_order     INTEGER,                      -- 手动排序权重（目录/分类：NULL=动态排序；0 显式最前；相同权重→回退动态）
  pin_scope      TEXT NOT NULL DEFAULT 'none', -- 文章置顶范围（2.1 置顶底座）：none|category（分类置顶）|global（全站置顶）
  pin_order      INTEGER,                      -- 置顶排序权重（同 scope 内升序；NULL=未设，按更新时间）
  access_pwd     TEXT,                         -- 文章访问密码（sha256 哈希；NULL/空=无密码；解锁后会话级放行，2.2 密码保护）
  publish_at     INTEGER,                      -- 定时发布（毫秒时间戳；NULL=立即；到点由常驻调度置 NULL 自动公开，2.3 定时发布）
  view_count     INTEGER NOT NULL DEFAULT 0,   -- 阅读计数（PV 聚合到文章级；公开页 blog-stats 上报时累加）
  deleted_at     INTEGER,                      -- 软删除（回收站）
  created_at     INTEGER NOT NULL,
  updated_at     INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_files_space_path ON files(space_id, parent_id, name);
CREATE INDEX IF NOT EXISTS idx_files_deleted ON files(deleted_at);
-- slug 唯一索引见 Migrate（需先确保列存在；旧库列迁移后补建）

-- 索引（三类分开存储；向量部分走 VectorSink 抽象，SQLite 阶段用 sqlite-vec）
CREATE TABLE IF NOT EXISTS index_chunks (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  file_id    TEXT NOT NULL REFERENCES files(id),
  space_id   TEXT NOT NULL,
  chunk_idx  INTEGER NOT NULL,                 -- 块序号
  content    TEXT NOT NULL,                    -- 块文本（全文检索源）
  meta       TEXT NOT NULL DEFAULT '{}',       -- 段落锚点/页码/时间戳等定位信息 JSON
  model      TEXT,                             -- 生成向量的 embedding 模型名（向量升级要重建索引）
  status     TEXT NOT NULL DEFAULT 'pending',  -- pending|indexed|failed
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chunks_file ON index_chunks(file_id);

-- 标签：多值路径式（tech/distributed/raft）
CREATE TABLE IF NOT EXISTS tags (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  path       TEXT NOT NULL,                     -- 完整路径
  parent_id  TEXT REFERENCES tags(id),
  owner_id   TEXT NOT NULL,
  created_at INTEGER NOT NULL DEFAULT 0,
  UNIQUE(owner_id, path)
);

CREATE TABLE IF NOT EXISTS file_tags (
  file_id TEXT NOT NULL REFERENCES files(id),
  tag_id  TEXT NOT NULL REFERENCES tags(id),
  PRIMARY KEY(file_id, tag_id)
);

-- 集合：手动 + 智能（查询驱动自动更新）
CREATE TABLE IF NOT EXISTS collections (
  id         TEXT PRIMARY KEY,
  owner_id   TEXT NOT NULL,
  space_id   TEXT NOT NULL,
  name       TEXT NOT NULL,
  kind       TEXT NOT NULL DEFAULT 'manual',   -- manual|smart
  query      TEXT,                             -- 智能集合的查询定义 JSON
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS collection_files (
  collection_id TEXT NOT NULL REFERENCES collections(id),
  file_id       TEXT NOT NULL REFERENCES files(id),
  added_at      INTEGER NOT NULL,
  PRIMARY KEY(collection_id, file_id)
);

-- 分享令牌
CREATE TABLE IF NOT EXISTS shares (
  id         TEXT PRIMARY KEY,
  file_id    TEXT REFERENCES files(id),
  collection_id TEXT REFERENCES collections(id),
  owner_id   TEXT NOT NULL,
  token      TEXT NOT NULL UNIQUE,
  permission TEXT NOT NULL DEFAULT 'read',     -- read|write
  expires_at INTEGER,
  created_at INTEGER NOT NULL,
  revoked_at INTEGER
);

-- 评论（协作层，阶段 4 前置建表）
CREATE TABLE IF NOT EXISTS comments (
  id         TEXT PRIMARY KEY,
  file_id    TEXT NOT NULL REFERENCES files(id),
  user_id    TEXT REFERENCES users(id),        -- NULL=访客评论（guest_name 兜底展示）
  anchor     TEXT,                             -- 段落/chunk 锚点
  body       TEXT NOT NULL,
  parent_id  TEXT REFERENCES comments(id),
  status     TEXT NOT NULL DEFAULT 'approved', -- approved=已通过（公开可见）|pending=待审（访客评论）
  guest_name TEXT NOT NULL DEFAULT '',         -- 访客昵称（登录评论为空）
  created_at INTEGER NOT NULL
);

-- Webhook 投递端点（事件契约 v1 投递器；内核 topic 白名单见 engine/bus/topics.go）
CREATE TABLE IF NOT EXISTS blog_webhooks (
  id         TEXT PRIMARY KEY,
  url        TEXT NOT NULL,                    -- 投递目标（http/https）
  secret     TEXT NOT NULL DEFAULT '',         -- HMAC-SHA256 签名密钥（X-AiKlog-Signature）
  topics     TEXT NOT NULL DEFAULT '[]',       -- 订阅 topic JSON 数组（内核冻结清单内）
  enabled    INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

-- 通知与消息中心
CREATE TABLE IF NOT EXISTS notifications (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id),
  type       TEXT NOT NULL,                    -- mention|space|task|comment|index
  payload    TEXT NOT NULL DEFAULT '{}',
  read_at    INTEGER,
  created_at INTEGER NOT NULL
);

-- 持久任务队列：索引/转写/缩略图等副作用（幂等、可重试、死信）
CREATE TABLE IF NOT EXISTS jobs (
  id         TEXT PRIMARY KEY,
  type       TEXT NOT NULL,                    -- index_file|asr|thumbnail|rebuild|check
  payload    TEXT NOT NULL DEFAULT '{}',
  status     TEXT NOT NULL DEFAULT 'pending',  -- pending|running|done|failed|dead
  attempts   INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  run_after  INTEGER NOT NULL DEFAULT 0,       -- 延迟/重试时间戳
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_ready ON jobs(status, run_after);

-- 导入导出中心（R11/R12）：异步任务 + 进度可见 + 失败清单可重试。
-- 独立于 jobs 表：jobs 是索引/转写/缩略图等"幂等副作用"队列（无进度/失败明细字段），
-- 导入导出需 total/processed/percent/fail_list 等展示与重试语义，单列一表更清晰、免迁移。
-- kind: import|export；source: zip|clipboard|url|obsidian|folder|tag|collection；
-- status: pending|running|done|partial|failed；fail_list: JSON [{item,error,ref}]。
CREATE TABLE IF NOT EXISTS impex_jobs (
  id         TEXT PRIMARY KEY,
  kind       TEXT NOT NULL,
  source     TEXT NOT NULL,
  status     TEXT NOT NULL DEFAULT 'pending',
  percent    INTEGER NOT NULL DEFAULT 0,
  total      INTEGER NOT NULL DEFAULT 0,
  processed  INTEGER NOT NULL DEFAULT 0,
  payload    TEXT NOT NULL DEFAULT '{}',
  result     TEXT NOT NULL DEFAULT '{}',
  fail_list  TEXT NOT NULL DEFAULT '[]',
  last_error TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_impex_jobs_recent ON impex_jobs(kind, created_at);

-- 爱库录精简发行：已裁剪采集（sources/collect_runs/collect_tombstones）与
-- 商业化预留表（skill_packages/skill_grants/prompt_templates/workflow_defs）。

-- AI 入库解读（异步后台队列产物）：上传/索引后由 Summarizer 串行分批生成，
-- 对话精读优先复用，避免大文档多轮临时读取；tags 预留 JSON 数组（图谱/检索扩展）；
-- meta 为 JSON（author/source/keywords/doc_type/language，K19 入库元数据抽取）。
CREATE TABLE IF NOT EXISTS file_ai_summaries (
  file_id    TEXT PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  status     TEXT NOT NULL DEFAULT 'pending', -- pending|done|error
  summary    TEXT NOT NULL DEFAULT '',
  tags       TEXT NOT NULL DEFAULT '[]',      -- JSON 数组
  meta       TEXT NOT NULL DEFAULT '{}',      -- JSON 对象（K19 元数据）
  source     TEXT NOT NULL DEFAULT '',        -- head|full
  attempt    INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  updated_at INTEGER NOT NULL
);

-- 知识图谱边表（知识库聚合层 A06）：文件-文件/标签-文件/标签-标签 关系
-- relation: related | cites | references | tagged_as | parent_of；weight 供图谱布局/排序
CREATE TABLE IF NOT EXISTS knowledge_edges (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  space_id    TEXT NOT NULL,
  source_type TEXT NOT NULL,             -- file | tag
  source_id   TEXT NOT NULL,
  target_type TEXT NOT NULL,             -- file | tag
  target_id   TEXT NOT NULL,
  relation    TEXT NOT NULL DEFAULT 'related',
  weight      REAL NOT NULL DEFAULT 1,
  source      TEXT NOT NULL DEFAULT 'manual', -- manual | ai | auto
  anchor      TEXT,                      -- 段落级引用锚点（[[文件#标题]] 的标题，null=整文件）
  created_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_knowledge_edges_source ON knowledge_edges(source_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_edges_target ON knowledge_edges(target_id);

-- 配置中心：配置即变量（实施文档 §10.5）
CREATE TABLE IF NOT EXISTS settings (
  key          TEXT NOT NULL,
  scope        TEXT NOT NULL DEFAULT 'instance', -- instance|space|user
  scope_id     TEXT NOT NULL DEFAULT '',         -- space_id / user_id（scope=instance 时为空）
  value        TEXT NOT NULL DEFAULT '',         -- JSON 序列化值
  type         TEXT NOT NULL DEFAULT 'string',   -- string|int|bool|json|secret
  default_value TEXT,
  description  TEXT,
  encrypted    INTEGER NOT NULL DEFAULT 0,
  updated_by   TEXT,
  updated_at   INTEGER NOT NULL,
  PRIMARY KEY(key, scope, scope_id)
);

-- 审计日志：追加写 + prev_hash 哈希链（防篡改，企业版强制开启）
CREATE TABLE IF NOT EXISTS audit_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    TEXT,
  action     TEXT NOT NULL,
  target     TEXT,
  detail     TEXT NOT NULL DEFAULT '{}',
  prev_hash  TEXT NOT NULL DEFAULT '',
  hash       TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

-- 登录会话：Bearer token 鉴权（单机单用户；token 随机 32 字节 hex，可主动失效）
CREATE TABLE IF NOT EXISTS sessions (
  token      TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id),
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

-- 向量表（SQLite 起步阶段；升级 pgvector/Qdrant 时经 VectorSink 迁移，表结构由 sink 实现）
CREATE TABLE IF NOT EXISTS vectors (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  chunk_id   INTEGER NOT NULL REFERENCES index_chunks(id),
  file_id    TEXT NOT NULL,
  space_id   TEXT NOT NULL,
  model      TEXT NOT NULL,
  vec        BLOB NOT NULL,                    -- 序列化向量
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_vectors_model ON vectors(model);

-- AI 对话会话（多话题模式：每会话独立上下文；消息级联删除由应用层处理）
CREATE TABLE IF NOT EXISTS ai_conversations (
  id         TEXT PRIMARY KEY,
  owner_id   TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT '新对话',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ai_conv_owner ON ai_conversations(owner_id, updated_at);

-- AI 对话消息（只存 user/assistant 文本；工具调用过程不落库，重放时只回放文本）
CREATE TABLE IF NOT EXISTS ai_messages (
  id              TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  role            TEXT NOT NULL, -- user | assistant
  content         TEXT NOT NULL,
  created_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ai_msg_conv ON ai_messages(conversation_id, created_at);

-- 博客功能插件（协议：声明+启用状态；渲染由前端按 mount_points 加载组件）
CREATE TABLE IF NOT EXISTS blog_plugins (
  id              TEXT PRIMARY KEY,            -- 插件 id（如 blog-comments）
  name            TEXT NOT NULL,
  version         TEXT NOT NULL DEFAULT '0.1.0',
  description     TEXT NOT NULL DEFAULT '',
  author          TEXT NOT NULL DEFAULT '',
  mount_points    TEXT NOT NULL DEFAULT '[]',  -- JSON 数组：post_bottom|sidebar|list_item|head
  api_permissions TEXT NOT NULL DEFAULT '[]',  -- JSON 数组：blog.read / blog.write
  frontend_entry  TEXT NOT NULL DEFAULT '',    -- 前端组件注册键
  min_core_version TEXT NOT NULL DEFAULT '0',  -- 要求的最低主系统版本（语义化）
  enabled         INTEGER NOT NULL DEFAULT 1,
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL
);

-- AI 能力用量（配额分层通用规范 v1.0）：day=北京时间自然日；ident=u:<uid> / ip:<ip>；scope 能力名
CREATE TABLE IF NOT EXISTS ai_ask_usage (
  day             TEXT NOT NULL,
  ident           TEXT NOT NULL,
  scope           TEXT NOT NULL,
  hits            INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (day, ident, scope)
);

-- 博客插件数据（KV：第三方插件读写通道，替代自由 DDL；天然按插件隔离）
CREATE TABLE IF NOT EXISTS blog_plug_data (
  plugin_id  TEXT NOT NULL,
  key        TEXT NOT NULL,
  value      TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (plugin_id, key)
);

-- IM 网关幂等去重（webhook 重发/用户连发防护；发博客时 file_id 回填）
CREATE TABLE IF NOT EXISTS im_ingest (
  id         TEXT PRIMARY KEY,
  platform   TEXT NOT NULL,             -- telegram | wecom
  msg_id     TEXT NOT NULL,
  intent     TEXT NOT NULL,             -- 意图类型（blog_post / blog_draft / ...）
  file_id    TEXT,                      -- 入库动作产生的文件 id（发博客时填充）
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_im_ingest_dedup ON im_ingest(platform, msg_id);

-- 第三方开发者档案（审核制：pending 申请 → approved 通过；一人一档）
CREATE TABLE IF NOT EXISTS developers (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL REFERENCES users(id),
  display_name  TEXT NOT NULL,
  bio           TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'pending',  -- pending | approved | rejected
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL,
  UNIQUE (user_id)
);

-- 第三方市场条目（开发者自管内容：只能改自己的；平台审核闸门 status）
CREATE TABLE IF NOT EXISTS market_listings (
  id            TEXT PRIMARY KEY,
  developer_id  TEXT NOT NULL REFERENCES developers(id),
  kind          TEXT NOT NULL,                   -- plugin | theme | editor | ai
  name          TEXT NOT NULL,
  description   TEXT NOT NULL DEFAULT '',
  icon          TEXT NOT NULL DEFAULT '',         -- 相对 /market/uploads/ 路径或外链
  screenshots   TEXT NOT NULL DEFAULT '[]',       -- JSON 数组：路径/链接
  version       TEXT NOT NULL DEFAULT '0.1.0',
  download_url  TEXT NOT NULL DEFAULT '',         -- 相对 /market/uploads/ 或外链
  sha256        TEXT NOT NULL DEFAULT '',
  price_cents   INTEGER NOT NULL DEFAULT 0,       -- 0 = 免费
  tier          TEXT NOT NULL DEFAULT 'free',     -- free | paid
  status        TEXT NOT NULL DEFAULT 'draft',    -- draft | pending | approved | rejected
  review_note   TEXT NOT NULL DEFAULT '',
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_market_listings_dev ON market_listings(developer_id);
CREATE INDEX IF NOT EXISTS idx_market_listings_status ON market_listings(status);
`
