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

-- 工作流编排定义（B2 内容工作流引擎）：nodes 存 Workflow JSON（triggers/steps/on_fail 模板）。
-- 注：原「精简发行」注释曾把本表一并裁剪，B2 批次按上游同构恢复（字段与上游一致，便于跨壳互通）。
CREATE TABLE IF NOT EXISTS workflow_defs (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  nodes       TEXT NOT NULL DEFAULT '[]',      -- JSON：{id,name,triggers,steps,enabled,...}
  status      TEXT NOT NULL DEFAULT 'draft',   -- draft|published|archived（仅 published 参与触发匹配）
  version     INTEGER NOT NULL DEFAULT 1,
  author_id   TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);

-- workflow 执行记录（幂等审计）：UNIQUE(wf_id,msg_key) 即幂等闸门本体——
-- IM 重复投递 / API 重放同一 msg_key 只会真正执行一次（INSERT OR IGNORE）。
CREATE TABLE IF NOT EXISTS workflow_runs (
  id         TEXT PRIMARY KEY,
  wf_id      TEXT NOT NULL,
  msg_key    TEXT NOT NULL,
  status     TEXT NOT NULL DEFAULT 'ok',       -- ok|failed
  detail     TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE (wf_id, msg_key)
);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_recent ON workflow_runs(created_at);

-- 评论收录（B3：内容引用与归因）——把评论区高价值内容带归因追加进正文。
-- 移植自上游 AiKmap.cn（同构字段便于跨壳互通）；本壳差异：
--   1) 评论素材取自本壳 comments 表（非上游 blog_comments），归因作者名取自 users.display_name；
--   2) actor_id 为本壳用户 id（newID()，32-hex）；
--   3) 站点归属不落列——组随文章走，跨站过滤经 ingest_groups.file_id → files.site_id。
-- 硬边界：收录只 append 不 mutate；撤销按锚点区块精确删除并置 status=reverted 留痕。
CREATE TABLE IF NOT EXISTS ingest_groups (
  id             TEXT PRIMARY KEY,             -- 收录组 id（同时是正文锚点 ingest-{id}）
  file_id        TEXT NOT NULL REFERENCES files(id),
  mode           TEXT NOT NULL DEFAULT 'quote', -- quote|fuse
  source         TEXT NOT NULL DEFAULT 'blog',  -- blog|doc|collect（评论/线索来源）
  status         TEXT NOT NULL DEFAULT 'draft', -- draft|accepted|reverted
  title          TEXT NOT NULL DEFAULT '',      -- 融合式小节标题（如"读者补充"）
  body           TEXT NOT NULL DEFAULT '',      -- 写入正文的区块（含归因；quote=原文照录；fuse=AI 融合+脚注归因）
  draft          TEXT NOT NULL DEFAULT '',      -- 融合式草稿原文（accept 后保留，可回查"谁说了什么"）
  version_before INTEGER NOT NULL DEFAULT 0,    -- 收录前的文件版本（撤销/审计用）
  version_after  INTEGER NOT NULL DEFAULT 0,    -- 收录后的文件版本
  actor_id       TEXT NOT NULL,                 -- 操作者（站长/授权作者）
  created_at     INTEGER NOT NULL,
  updated_at     INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_ingest_groups_file ON ingest_groups(file_id, status);
CREATE INDEX IF NOT EXISTS ix_ingest_groups_status ON ingest_groups(status, created_at);

-- 收录项：一组对应的一条评论（quote=1 条；fuse=N 条）。content=引用原文快照
-- （融合式防评论删除后失据；quote=原评论内容）。author_* 冗余供归因渲染与重投通知。
CREATE TABLE IF NOT EXISTS ingest_items (
  group_id    TEXT NOT NULL REFERENCES ingest_groups(id),
  comment_id  TEXT NOT NULL,
  author_id   TEXT NOT NULL DEFAULT '',
  author_name TEXT NOT NULL DEFAULT '',
  content     TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  PRIMARY KEY(group_id, comment_id)
);
CREATE INDEX IF NOT EXISTS ix_ingest_items_comment ON ingest_items(comment_id);
CREATE INDEX IF NOT EXISTS ix_ingest_items_author ON ingest_items(author_id);

-- 爱库录精简发行：已裁剪采集（sources/collect_runs/collect_tombstones）与
-- 商业化预留表（skill_packages/skill_grants/prompt_templates）；workflow_defs 已于 B2 恢复。

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

-- IM 绑定（B7）：站内用户 ↔ IM 平台身份映射，把「谁在哪个 IM 账号」接到「站内哪个身份」。
-- (platform, platform_user_id) 唯一：一个 IM 身份同时只对应一个站内用户，再次绑定＝改绑（用户显式操作）。
-- 时间戳一律**毫秒**（与本壳 files.*_at 口径一致；注意 doc_subscriptions 等早期表是秒）。
CREATE TABLE IF NOT EXISTS im_bindings (
  id               TEXT PRIMARY KEY,
  user_id          TEXT NOT NULL REFERENCES users(id),
  platform         TEXT NOT NULL,             -- telegram | wecom
  platform_user_id TEXT NOT NULL,             -- 平台侧身份（Telegram from.id / 企微 userid）
  platform_name    TEXT NOT NULL DEFAULT '',  -- 平台侧昵称（展示用）
  chat_id          TEXT NOT NULL DEFAULT '',  -- 常用会话 ID（主动推送目标，可空）
  bound_at         INTEGER NOT NULL,
  last_active_at   INTEGER NOT NULL DEFAULT 0,
  UNIQUE(platform, platform_user_id)
);
CREATE INDEX IF NOT EXISTS ix_im_bindings_user ON im_bindings(user_id);

-- IM 绑定码（B7）：Web 端（已登录）生成 → IM 端回复 /bind <码> 消费；一次性、短时效。
-- 免去在 IM 里输入账号密码；platform 为空表示该码可用于任意平台。
CREATE TABLE IF NOT EXISTS im_binding_codes (
  code       TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL,
  platform   TEXT NOT NULL DEFAULT '',
  expires_at INTEGER NOT NULL,
  used_at    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS ix_im_binding_codes_user ON im_binding_codes(user_id);

-- 绑定尝试失败计数（B10 限流）：绑定码是 6 位短码，若无失败计数，攻击者可用一个 IM 身份
-- 持续试码（31^6 空间虽然大，但试满 10 分钟窗口仍有可观命中率）。按
-- (platform, platform_user_id) 计数并在连续失败达到阈值后软锁定，锁定期内直接拒绝、
-- 不消耗任何码；绑定成功即删行。时间戳毫秒（与 im_bindings 口径一致）。
CREATE TABLE IF NOT EXISTS im_bind_attempts (
  platform         TEXT NOT NULL,
  platform_user_id TEXT NOT NULL,
  fail_count       INTEGER NOT NULL DEFAULT 0,
  locked_until     INTEGER NOT NULL DEFAULT 0,
  updated_at       INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (platform, platform_user_id)
);
CREATE INDEX IF NOT EXISTS ix_im_bind_attempts_locked ON im_bind_attempts(locked_until);

-- 媒体元信息（B8 媒体底座）：图片宽高 / 音视频时长与编码 / 缩略图与转码产物引用。
-- 本壳当前只做**本地可探测**的部分：图片宽高走 Go 标准库解码 header（不解全图，省内存）。
-- 音视频时长/编码/缩略图/转码依赖 ffmpeg —— 探测结果记为 probe_status='unsupported'，
-- 待外部二进制就位后由 worker 补齐。表结构按上游预留位口径，避免二次迁移。
CREATE TABLE IF NOT EXISTS file_media (
  file_id        TEXT PRIMARY KEY,
  duration_ms    INTEGER NOT NULL DEFAULT 0,
  width          INTEGER NOT NULL DEFAULT 0,
  height         INTEGER NOT NULL DEFAULT 0,
  codec          TEXT NOT NULL DEFAULT '',
  bitrate        INTEGER NOT NULL DEFAULT 0,
  thumbnail_ref  TEXT NOT NULL DEFAULT '',
  transcoded_ref TEXT NOT NULL DEFAULT '',
  -- B17 视频转码状态（与 transcoded_ref 配套）：status ∈ queued|running|done|failed|canceled；
  -- 空串 = 从未发起。产物命名带参数指纹（play-<sig>.mp4），参数一改即新对象、旧对象显式删除。
  transcode_status     TEXT    NOT NULL DEFAULT '',
  transcode_msg        TEXT    NOT NULL DEFAULT '',
  transcode_signature  TEXT    NOT NULL DEFAULT '',
  transcode_bytes      INTEGER NOT NULL DEFAULT 0,
  transcode_height     INTEGER NOT NULL DEFAULT 0,
  transcode_updated_at INTEGER NOT NULL DEFAULT 0,
  probe_status   TEXT NOT NULL DEFAULT 'pending',  -- pending | ok | unsupported | failed
  updated_at     INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_file_media_status ON file_media(probe_status);

-- 技能包商业化（B9，对应上游 E12「商业化框架预留」）：上架 → 授权 → 分发。
-- 与应用中心同源但维度不同：应用中心分发「站点能力」（主题/插件/站点授权），
-- 技能包分发「可授权的能力资产」（tool/skill/workflow/extension），两者共用 entitlement 思想、
-- 不共用表。上游明确「只留数据模型，UI 待验证后再做」，本壳同策略：先落模型与只读/自助领取链路。
CREATE TABLE IF NOT EXISTS skill_packages (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  version     TEXT NOT NULL DEFAULT '0.1.0',
  kind        TEXT NOT NULL,               -- tool | skill | workflow | extension
  title       TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  manifest    TEXT NOT NULL DEFAULT '{}',  -- JSON：入口/依赖/权限声明/模型需求
  author_id   TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'draft', -- draft | published | archived
  price_cents INTEGER NOT NULL DEFAULT 0,  -- 0=免费；付费分级点
  license     TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_skill_pkg_status ON skill_packages(status, kind);

-- 授权/分发记录（购买/免费领取/试用/手工发放；吊销与到期控制；grantee 维度唯一）
CREATE TABLE IF NOT EXISTS skill_grants (
  id           TEXT PRIMARY KEY,
  package_id   TEXT NOT NULL REFERENCES skill_packages(id),
  grantee_type TEXT NOT NULL,              -- user | space | instance
  grantee_id   TEXT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'active', -- pending | active | revoked | expired
  source       TEXT NOT NULL DEFAULT 'free',   -- purchase | free | trial | manual
  expires_at   INTEGER,                        -- NULL=永久
  created_at   INTEGER NOT NULL,
  UNIQUE(package_id, grantee_type, grantee_id)
);
CREATE INDEX IF NOT EXISTS idx_skill_grants_grantee ON skill_grants(grantee_type, grantee_id, status);

-- 阅读事件按天聚合（A5 内容数据中心）：单篇热力图 / 近 N 天趋势 / 站级看板共用。
-- 写入口唯一 = 公开页 PV 上报（publicBlogPV）；与 files.view_count 同口径（首访去重后累加）。
CREATE TABLE IF NOT EXISTS view_events (
  file_id TEXT NOT NULL,               -- 文章 file_id（slug/path 解析失败不上报）
  day     TEXT NOT NULL,               -- YYYY-MM-DD（本地时区）
  count   INTEGER NOT NULL DEFAULT 0,  -- 当日阅读次数
  PRIMARY KEY (file_id, day)
);
CREATE INDEX IF NOT EXISTS idx_view_events_day ON view_events(day);

-- 文章版本历史（B5 写作增强）：文本编辑 / 二进制覆盖前，把旧内容快照到 storage 的
-- versions/<file_id>/v<n>，并在此登记元数据；支持「列表 / 下载 / 恢复」。
-- 只登记元数据（sha256/size/note），正文留在 storage —— 不把大正文塞进 sqlite。
-- UNIQUE(file_id,version) 是幂等闸门：同一版本号重复登记会被拒，快照不会互相覆盖。
CREATE TABLE IF NOT EXISTS file_versions (
  id         TEXT PRIMARY KEY,
  file_id    TEXT NOT NULL,
  version    INTEGER NOT NULL,
  sha256     TEXT,
  size       INTEGER NOT NULL DEFAULT 0,
  note       TEXT,
  created_by TEXT,
  created_at INTEGER NOT NULL,
  UNIQUE(file_id, version)
);
CREATE INDEX IF NOT EXISTS idx_file_versions_file ON file_versions(file_id, version);

-- AI 写作提示词模板（B5 写作增强）：把提示词做成后台可配置项（参数化，不硬编码）。
-- variables = JSON 变量声明 [{name,label,default}]，驱动前端表单；写作页 / 工作流 llm 步骤可引用。
-- is_public=1 表示全站可用（0=仅作者本人可见）。
CREATE TABLE IF NOT EXISTS prompt_templates (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  category    TEXT NOT NULL DEFAULT '',
  content     TEXT NOT NULL,
  variables   TEXT NOT NULL DEFAULT '[]',
  is_public   INTEGER NOT NULL DEFAULT 1,
  author_id   TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_prompt_templates_cat ON prompt_templates(category, updated_at);

-- 内容订阅（B6 订阅与通知）：用户订阅某个文件或某个目录，内容更新时向其写通知。
-- target_type=file 时 target_id 是 file_id；=dir 时是目录 file_id（该目录下任意文件更新都命中）。
-- UNIQUE(user_id,target_type,target_id) 是幂等闸门：重复订阅不产生重复行。
CREATE TABLE IF NOT EXISTS doc_subscriptions (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL,
  target_type TEXT NOT NULL,              -- file | dir
  target_id   TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  UNIQUE(user_id, target_type, target_id)
);
CREATE INDEX IF NOT EXISTS idx_doc_subscriptions_target ON doc_subscriptions(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_doc_subscriptions_user ON doc_subscriptions(user_id, created_at);

-- @提及（B6）：正文/评论里的 @用户 解析后落库，并向被提及者写 notifications(type='mention')。
-- source_type=comment|post；source_id = 评论 id 或 file_id；mentioned_user_id = 被提及者。
-- UNIQUE(source_type,source_id,mentioned_user_id) 防重复：同一条内容重复解析只留一条。
CREATE TABLE IF NOT EXISTS mentions (
  id                TEXT PRIMARY KEY,
  source_type       TEXT NOT NULL,        -- comment | post
  source_id         TEXT NOT NULL,
  mentioned_user_id TEXT NOT NULL,
  mentioner_id      TEXT NOT NULL DEFAULT '',
  read_at           INTEGER,
  created_at        INTEGER NOT NULL,
  UNIQUE(source_type, source_id, mentioned_user_id)
);
CREATE INDEX IF NOT EXISTS idx_mentions_user ON mentions(mentioned_user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_mentions_source ON mentions(source_type, source_id);
-- FAQ 知识库类型（WeKnora 借鉴 A2，B27 自上游 bbd6764 移植）：标准问/相似问/反例问/答案，问题即检索单元。
-- 与文档检索并列：SearchFAQ 独立命中（标准问/相似问 LIKE），Agent 经 faq_search 工具注入回答。
-- similar_qs / counter_qs / tags 为 JSON 数组字符串。
CREATE TABLE IF NOT EXISTS kb_faq (
  id           TEXT PRIMARY KEY,
  space_id     TEXT NOT NULL,
  standard_q   TEXT NOT NULL,              -- 标准问（主检索键）
  similar_qs   TEXT NOT NULL DEFAULT '[]', -- 相似问（扩充命中面）
  counter_qs   TEXT NOT NULL DEFAULT '[]', -- 反例问（排歧/负样本）
  answer       TEXT NOT NULL DEFAULT '',   -- 答案正文（Markdown）
  tags         TEXT NOT NULL DEFAULT '[]', -- 标签
  source       TEXT NOT NULL DEFAULT '',   -- 来源（import:{file_id}=A4 预生成问题索引）
  created_by   TEXT NOT NULL,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_kb_faq_space ON kb_faq(space_id, updated_at);
CREATE INDEX IF NOT EXISTS ix_kb_faq_source ON kb_faq(source);
-- 空间级长期记忆（WeKnora 借鉴 A3 + 云雇工叙事）：按 space 存资料/偏好/事实/事项/兴趣。
-- Agent 问答与云雇工任务可读写：写=沉淀上下文，读=回答带空间记忆。
-- kind: profile(空间画像)|fact(事实)|preference(偏好)|todo(事项)|interest(兴趣)|note(自由备注)
CREATE TABLE IF NOT EXISTS space_memory (
  id         TEXT PRIMARY KEY,
  space_id   TEXT NOT NULL,
  kind       TEXT NOT NULL DEFAULT 'note',
  key        TEXT NOT NULL,
  content    TEXT NOT NULL,
  source     TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE(space_id, key)
);
CREATE INDEX IF NOT EXISTS ix_space_memory_space ON space_memory(space_id, kind, updated_at);
-- 概念页 AI 摘要缓存（A09：标签概念页独立生成，避免每次调模型；sources=摘要依据的来源文档引用链 A5）
CREATE TABLE IF NOT EXISTS kb_concept_summaries (
  tag_id     TEXT PRIMARY KEY,
  summary    TEXT NOT NULL,
  sources    TEXT NOT NULL DEFAULT '[]', -- JSON：[{"file_id":"...","name":"..."}]
  updated_at INTEGER NOT NULL
);

-- ============================================================
-- 客服模块（SPEC-CS-001）：客户身份图谱 + 会话线程 + 渠道适配
-- 与 users 解耦；一个渠道身份只能属于一个联系人（ux_cs_ident_channel_ext 硬约束）。
-- ⚠️ 唯一索引必须带 WHERE external_id/thread_id <> '' 部分条件：SQLite 唯一索引对空串
--    同样生效，不加条件会导致「第二条空 thread/空 ext id 的记录插入失败」这类难查写入异常。
-- ============================================================
CREATE TABLE IF NOT EXISTS cs_contacts (
  id             TEXT PRIMARY KEY,
  display_name   TEXT NOT NULL DEFAULT '',      -- 展示名；空则回退首个身份标识
  linked_user_id TEXT NOT NULL DEFAULT '',      -- 该客户恰是站内注册用户时关联 users.id
  email          TEXT NOT NULL DEFAULT '',      -- 归一等值匹配用（可空）
  phone          TEXT NOT NULL DEFAULT '',
  company        TEXT NOT NULL DEFAULT '',
  note           TEXT NOT NULL DEFAULT '',      -- 客服备注（内部可见，永不外发）
  tags           TEXT NOT NULL DEFAULT '[]',    -- JSON 数组
  owner_id       TEXT NOT NULL DEFAULT '',      -- 归属坐席；'' = 未分派
  status         TEXT NOT NULL DEFAULT 'active',-- active | blocked | merged
  merged_into    TEXT NOT NULL DEFAULT '',      -- 合并留痕，不物理删
  created_at     INTEGER NOT NULL DEFAULT 0,
  updated_at     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_cs_contacts_email ON cs_contacts(email);
CREATE INDEX IF NOT EXISTS idx_cs_contacts_owner ON cs_contacts(owner_id, updated_at);

CREATE TABLE IF NOT EXISTS cs_contact_identities (
  id            TEXT PRIMARY KEY,
  contact_id    TEXT NOT NULL,
  channel       TEXT NOT NULL,              -- email | webchat | wecom | dingtalk | feishu | telegram | whatsapp
  external_id   TEXT NOT NULL,              -- 渠道侧唯一标识：邮箱/openid/tg id/挂件 visitor_id
  display       TEXT NOT NULL DEFAULT '',
  verified      INTEGER NOT NULL DEFAULT 0,
  first_seen_at INTEGER NOT NULL DEFAULT 0,
  last_seen_at  INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_cs_ident_channel_ext
  ON cs_contact_identities(channel, external_id);
CREATE INDEX IF NOT EXISTS idx_cs_ident_contact ON cs_contact_identities(contact_id);

CREATE TABLE IF NOT EXISTS cs_conversations (
  id                 TEXT PRIMARY KEY,
  contact_id         TEXT NOT NULL,
  channel            TEXT NOT NULL,
  external_thread_id TEXT NOT NULL DEFAULT '',  -- 邮件=线程根 Message-ID；IM=平台会话 id；挂件=visitor session
  subject            TEXT NOT NULL DEFAULT '',
  status             TEXT NOT NULL DEFAULT 'open',   -- open | pending | resolved | closed
  priority           TEXT NOT NULL DEFAULT 'normal', -- low | normal | high | urgent
  assignee_id        TEXT NOT NULL DEFAULT '',
  unread_count       INTEGER NOT NULL DEFAULT 0,
  last_message_at    INTEGER NOT NULL DEFAULT 0,
  first_reply_at     INTEGER NOT NULL DEFAULT 0,     -- 首次人工响应（SLA 起点）
  resolved_at        INTEGER NOT NULL DEFAULT 0,
  created_at         INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_cs_conv_thread
  ON cs_conversations(channel, external_thread_id) WHERE external_thread_id <> '';
CREATE INDEX IF NOT EXISTS idx_cs_conv_status ON cs_conversations(status, last_message_at);
CREATE INDEX IF NOT EXISTS idx_cs_conv_contact ON cs_conversations(contact_id, last_message_at);

CREATE TABLE IF NOT EXISTS cs_messages (
  id              TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  direction       TEXT NOT NULL,               -- in | out
  channel         TEXT NOT NULL,
  external_id     TEXT NOT NULL DEFAULT '',    -- 渠道侧消息 id（入站幂等键）/ 出站回执 id
  in_reply_to     TEXT NOT NULL DEFAULT '',    -- 邮件 In-Reply-To / References 链
  author_type     TEXT NOT NULL DEFAULT 'contact', -- contact | agent | ai | system
  author_id       TEXT NOT NULL DEFAULT '',
  body_text       TEXT NOT NULL DEFAULT '',
  body_html       TEXT NOT NULL DEFAULT '',
  attachments     TEXT NOT NULL DEFAULT '[]',  -- JSON: [{name,mime,size,fid}]
  meta            TEXT NOT NULL DEFAULT '{}',  -- 原报文/头部（排障用，不出前端）
  created_at      INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_cs_msg_ext
  ON cs_messages(channel, external_id) WHERE external_id <> '';
CREATE INDEX IF NOT EXISTS idx_cs_msg_conv ON cs_messages(conversation_id, created_at);

CREATE TABLE IF NOT EXISTS cs_outbound (
  id              TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  channel         TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,      -- 坐席端 client_msg_id，防重复点击
  payload         TEXT NOT NULL,      -- JSON: {to,subject,text,html,attachments}
  status          TEXT NOT NULL DEFAULT 'queued', -- queued|sending|sent|failed|blocked
  attempts        INTEGER NOT NULL DEFAULT 0,
  last_error      TEXT NOT NULL DEFAULT '',
  next_attempt_at INTEGER NOT NULL DEFAULT 0,
  sent_at         INTEGER NOT NULL DEFAULT 0,
  created_at      INTEGER NOT NULL DEFAULT 0,
  updated_at      INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_cs_outbound_idem ON cs_outbound(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_cs_outbound_status ON cs_outbound(status, next_attempt_at);

CREATE TABLE IF NOT EXISTS cs_channels (
  id          TEXT PRIMARY KEY,
  channel     TEXT NOT NULL,
  name        TEXT NOT NULL DEFAULT '',
  enabled     INTEGER NOT NULL DEFAULT 0,
  config_enc  TEXT NOT NULL DEFAULT '',   -- 渠道接入配置（密钥，加密落库）
  window_secs INTEGER NOT NULL DEFAULT 0, -- 客服消息窗口（秒）；0=不限
  qps         REAL NOT NULL DEFAULT 1,    -- 限速（令牌桶速率）
  created_at  INTEGER NOT NULL DEFAULT 0,
  updated_at  INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_cs_channels_channel ON cs_channels(channel);

-- ============================================================
-- 多租户 SaaS 计费（SPEC-BILLING）：套餐 → 站点订阅 → 额度计量 → 超限拒绝
-- 与 site_licenses（站点数授权）分层：license 管「能开几个站」，subscription 管
-- 「这个站是什么档、额度多少、到什么时候」。二者独立，缺一不影响另一。
-- ============================================================
CREATE TABLE IF NOT EXISTS billing_plans (
  id            TEXT PRIMARY KEY,          -- free | community | pro | team
  name          TEXT NOT NULL DEFAULT '',
  description   TEXT NOT NULL DEFAULT '',
  edition       TEXT NOT NULL DEFAULT 'community', -- 映射到站点 license edition
  price_cents   INTEGER NOT NULL DEFAULT 0,        -- 标价（分）；0=免费
  billing_period TEXT NOT NULL DEFAULT 'free',      -- free | monthly | yearly
  max_sites     INTEGER NOT NULL DEFAULT 1,        -- 该档可用站点数
  max_members   INTEGER NOT NULL DEFAULT 1,        -- 每站成员数（0=不限）
  max_storage_mb INTEGER NOT NULL DEFAULT 1024,     -- 每站存储（MB）
  max_ai_calls_daily INTEGER NOT NULL DEFAULT 0,   -- 每站每日 AI 调用（0=不限）
  features      TEXT NOT NULL DEFAULT '[]',         -- JSON 特性开关清单
  sort          INTEGER NOT NULL DEFAULT 0,
  enabled       INTEGER NOT NULL DEFAULT 1,
  created_at    INTEGER NOT NULL DEFAULT 0,
  updated_at    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS site_subscriptions (
  id             TEXT PRIMARY KEY,
  site_id        TEXT NOT NULL,
  plan_id        TEXT NOT NULL,
  status         TEXT NOT NULL DEFAULT 'active',  -- active | past_due | canceled | expired
  billing_period TEXT NOT NULL DEFAULT 'free',
  price_cents    INTEGER NOT NULL DEFAULT 0,
  started_at     INTEGER NOT NULL DEFAULT 0,
  current_period_end INTEGER NOT NULL DEFAULT 0,   -- 订阅期到期（毫秒）；0=永久
  cancel_at_period_end INTEGER NOT NULL DEFAULT 0, -- 到期后取消（续费则自动降档）
  order_id       TEXT NOT NULL DEFAULT '',          -- 关联支付订单
  created_at     INTEGER NOT NULL DEFAULT 0,
  updated_at     INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_site_sub ON site_subscriptions(site_id);
CREATE INDEX IF NOT EXISTS idx_site_sub_plan ON site_subscriptions(plan_id, status);

-- 每日用量计量（按站点×日 聚合，超限拒绝读这里）
CREATE TABLE IF NOT EXISTS site_usage_daily (
  site_id    TEXT NOT NULL,
  day        TEXT NOT NULL,              -- YYYY-MM-DD
  ai_calls   INTEGER NOT NULL DEFAULT 0,
  storage_bytes INTEGER NOT NULL DEFAULT 0,
  files      INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (site_id, day)
);

-- ============================================================
-- 模型提供方持久化（「可任意添加/配置大模型」能力底座）
-- providers.json 的内置提供方首次启动种子写入（builtin=1）；用户在后台新增/编辑的
-- 提供方 builtin=0。网关注册表从该表全量加载，增删改即时热更新，无需改代码/重启。
-- keys 以明文存储（与 secrets/providers.json 一致，该文件同样明文存密钥且 gitignore）；
-- 后续如需加密可对接 config 加密通道，本表结构已预留。
-- ============================================================
CREATE TABLE IF NOT EXISTS ai_providers (
  name         TEXT PRIMARY KEY,         -- 提供方标识（defs map 的 key）
  endpoint     TEXT NOT NULL DEFAULT '',
  model        TEXT NOT NULL DEFAULT '', -- 默认模型（cap 未单独指定时用）
  models       TEXT NOT NULL DEFAULT '{}',   -- JSON: {cap: model}
  model_cands  TEXT NOT NULL DEFAULT '{}',   -- JSON: {cap: [model,...]}
  keys         TEXT NOT NULL DEFAULT '[]',   -- JSON: [apiKey,...]（token 池轮替）
  caps         TEXT NOT NULL DEFAULT '[]',   -- JSON: ["llm","embedding",...]
  voice_cands  TEXT NOT NULL DEFAULT '[]',   -- JSON: [音色名,...]
  note         TEXT NOT NULL DEFAULT '',
  builtin      INTEGER NOT NULL DEFAULT 0,   -- 1=内置（来自 providers.json，禁止删除）
  updated_at   INTEGER NOT NULL DEFAULT 0
);

-- ============================================================
-- 商城（WooCommerce 式）：商品 / 购物车 / 订单 / 订单项 / 优惠券
--   - 商品分 digital（数字，库存可为 NULL=不限）与 physical（实物，需库存与收货地址）
--   - 购物车以匿名 cookie（token）承载；登录后按 user_id 归属；登录态合并匿名车
--   - 金额单位统一「分」（price_cents / subtotal_cents ...），与 files.price_cents 同口径
--   - 支付网关（mock/wechat/alipay/stripe）仅在内核留适配层；收款由网关回调
--     POST /api/v1/store/notify/{gateway} 幂等标记订单已付（与 pay/notify 同模型）
-- ============================================================
CREATE TABLE IF NOT EXISTS store_products (
  id            TEXT PRIMARY KEY,
  kind          TEXT NOT NULL DEFAULT 'digital',  -- digital | physical
  slug          TEXT NOT NULL DEFAULT '',
  title         TEXT NOT NULL DEFAULT '',
  summary       TEXT NOT NULL DEFAULT '',
  body          TEXT NOT NULL DEFAULT '',         -- 详情（Markdown）
  price_cents   INTEGER NOT NULL DEFAULT 0,
  currency      TEXT NOT NULL DEFAULT 'CNY',
  stock         INTEGER,                          -- NULL=不限（数字商品）；>=0=实物库存
  sku           TEXT NOT NULL DEFAULT '',
  cover         TEXT NOT NULL DEFAULT '',         -- 封面图（站内 file id 或外链）
  meta          TEXT NOT NULL DEFAULT '{}',       -- JSON：下载链接/物流模板等扩展
  status        TEXT NOT NULL DEFAULT 'draft',    -- draft | published
  sort          INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL DEFAULT 0,
  updated_at    INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_store_product_slug ON store_products(slug);
CREATE INDEX IF NOT EXISTS idx_store_products_status ON store_products(status, sort);

-- 购物车（匿名 cookie 承载；items 以 JSON 存 [{product_id, qty}]，避免再建一张表）
CREATE TABLE IF NOT EXISTS store_carts (
  token       TEXT PRIMARY KEY,                 -- 匿名购物车令牌（种在 cookie）
  user_id     TEXT NOT NULL DEFAULT '',         -- 登录后回填（合并用）
  items       TEXT NOT NULL DEFAULT '[]',       -- JSON: [{product_id, qty}]
  created_at  INTEGER NOT NULL DEFAULT 0,
  updated_at  INTEGER NOT NULL DEFAULT 0
);

-- 订单主表
CREATE TABLE IF NOT EXISTS store_orders (
  id             TEXT PRIMARY KEY,
  order_no       TEXT NOT NULL DEFAULT '',
  user_id        TEXT NOT NULL DEFAULT '',
  session_token  TEXT NOT NULL DEFAULT '',
  status         TEXT NOT NULL DEFAULT 'pending', -- pending|paid|fulfilled|refunded|cancelled|expired
  subtotal_cents INTEGER NOT NULL DEFAULT 0,
  discount_cents INTEGER NOT NULL DEFAULT 0,
  total_cents    INTEGER NOT NULL DEFAULT 0,
  currency       TEXT NOT NULL DEFAULT 'CNY',
  coupon_code    TEXT NOT NULL DEFAULT '',
  gateway        TEXT NOT NULL DEFAULT '',        -- wechat | alipay | stripe | mock
  gateway_data   TEXT NOT NULL DEFAULT '{}',      -- 网关返回（code_url / pay_url / checkout_id 等）
  paid_at        INTEGER NOT NULL DEFAULT 0,
  contact_email  TEXT NOT NULL DEFAULT '',
  contact_name   TEXT NOT NULL DEFAULT '',
  address        TEXT NOT NULL DEFAULT '{}',      -- 实物收货地址 JSON
  remark         TEXT NOT NULL DEFAULT '',
  expires_at     INTEGER NOT NULL DEFAULT 0,       -- 0=不过期
  created_at     INTEGER NOT NULL DEFAULT 0,
  updated_at     INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_store_order_no ON store_orders(order_no);
CREATE INDEX IF NOT EXISTS idx_store_orders_status ON store_orders(status, created_at);
CREATE INDEX IF NOT EXISTS idx_store_orders_user ON store_orders(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_store_orders_session ON store_orders(session_token, created_at);

-- 订单项（快照商品标题/单价，避免商品改价后历史订单失真）
CREATE TABLE IF NOT EXISTS store_order_items (
  id              TEXT PRIMARY KEY,
  order_id        TEXT NOT NULL DEFAULT '',
  product_id      TEXT NOT NULL DEFAULT '',
  title           TEXT NOT NULL DEFAULT '',
  kind            TEXT NOT NULL DEFAULT 'digital',
  unit_price_cents INTEGER NOT NULL DEFAULT 0,
  qty             INTEGER NOT NULL DEFAULT 1,
  subtotal_cents  INTEGER NOT NULL DEFAULT 0,
  meta            TEXT NOT NULL DEFAULT '{}',
  download_token  TEXT NOT NULL DEFAULT '',       -- B43：条目级下载令牌（高熵凭据，支付时签发）
  download_count  INTEGER NOT NULL DEFAULT 0,     -- B43：已下载次数
  max_downloads   INTEGER NOT NULL DEFAULT 0,     -- B43：次数上限（0=不限）
  created_at      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_store_order_items_order ON store_order_items(order_id);
-- 🔴 download_token 的索引**故意不写在这里**：Migrate 会把整段 DDL 对**存量库**也执行一遍，
-- 而存量库的 store_order_items 尚无 download_token 列（要靠 db.go 里的 ALTER 补），
-- CREATE INDEX IF NOT EXISTS 只挡「索引同名」、挡不住「列不存在」→ 整段 DDL 报
-- no such column。索引统一放在 db.go 的 ALTER 之后建。

-- 退款流水（B45：原路退款审计 + 部分退款；先落流水再改订单状态，便于对账）
CREATE TABLE IF NOT EXISTS store_refunds (
  id           TEXT PRIMARY KEY,
  order_no     TEXT NOT NULL DEFAULT '',
  amount_cents INTEGER NOT NULL DEFAULT 0,      -- 本次退款金额（分）
  total_cents  INTEGER NOT NULL DEFAULT 0,      -- 订单原实付金额（分）
  gateway      TEXT NOT NULL DEFAULT '',        -- 退款走的通道
  refund_no    TEXT NOT NULL DEFAULT '',        -- 网关退款单号（对账用）
  reason       TEXT NOT NULL DEFAULT '',
  operator     TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'succeeded',
  created_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_store_refunds_order ON store_refunds(order_no, created_at);

-- B47 应用中心回源统计（官方侧专用表；免费版实例不建也不需要）
--
-- 只存**不可逆派生的匿名 install_id**（HMAC 前 16 字节，128 位），
-- 不存域名、不存 IP、不存用户输入、不存任何可反推密钥的材料。
-- 官方侧因此只能回答「有多少台机器、分别什么版本」，
-- **回答不了「是谁、装在哪、装了什么」** —— 这是本设计的硬边界。
--
-- 一台安装 = 一行（install_id UNIQUE），重复回源只更新 last_seen/hits：
-- 这样「装机数」= 行数，不受用户刷页面的影响（否则统计会被行为噪声放大）。
CREATE TABLE IF NOT EXISTS market_instances (
  install_id  TEXT PRIMARY KEY,                -- 匿名派生 ID（32 位小写 hex），非用户可控
  version     TEXT NOT NULL DEFAULT 'unknown', -- 回源时上报的版本（字符白名单过滤后）
  shell       TEXT NOT NULL DEFAULT 'aiklog',  -- 壳标识（多壳模型）
  hits        INTEGER NOT NULL DEFAULT 0,      -- 回源次数（仅作活跃度参考，不作装机口径）
  first_seen  INTEGER NOT NULL DEFAULT 0,      -- 首次回源（毫秒）
  last_seen   INTEGER NOT NULL DEFAULT 0       -- 最近回源（毫秒）
);
-- 索引一律建在本表自有列上（引用不存在的新列会让整段 DDL 在存量库上报 no such column）
CREATE INDEX IF NOT EXISTS idx_market_instances_last_seen ON market_instances(last_seen);
CREATE INDEX IF NOT EXISTS idx_market_instances_version ON market_instances(version);

-- 退货申请（B46：买家发起 → 管理员审核 → 通过则走原路退款）
-- 一单至多一条「待审」申请（partial unique index 兜底并发），已终结的记录保留作审计。
CREATE TABLE IF NOT EXISTS store_returns (
  id           TEXT PRIMARY KEY,
  order_no     TEXT NOT NULL DEFAULT '',
  contact_email TEXT NOT NULL DEFAULT '',
  reason       TEXT NOT NULL DEFAULT '',
  amount_cents INTEGER NOT NULL DEFAULT 0,   -- 申请退款金额（分）；0=申请全额
  status       TEXT NOT NULL DEFAULT 'pending', -- pending|approved|rejected|refunded
  admin_note   TEXT NOT NULL DEFAULT '',      -- 商家答复（驳回原因 / 处理说明）
  operator     TEXT NOT NULL DEFAULT '',      -- 审核人
  created_at   INTEGER NOT NULL DEFAULT 0,
  updated_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_store_returns_order ON store_returns(order_no, created_at);
CREATE INDEX IF NOT EXISTS idx_store_returns_status ON store_returns(status, created_at);
-- 同单只允许一条待审：并发下双击也只会成功一条
CREATE UNIQUE INDEX IF NOT EXISTS ux_store_returns_pending ON store_returns(order_no) WHERE status='pending';

-- 优惠券 / 促销
CREATE TABLE IF NOT EXISTS store_coupons (
  id                TEXT PRIMARY KEY,
  code              TEXT NOT NULL DEFAULT '',
  kind              TEXT NOT NULL DEFAULT 'percent', -- percent | fixed | free_shipping
  value             REAL NOT NULL DEFAULT 0,         -- percent: 0-100；fixed: 金额（元，REAL）
  min_subtotal_cents INTEGER NOT NULL DEFAULT 0,     -- 起用门槛（分）；0=无门槛
  max_discount_cents INTEGER NOT NULL DEFAULT 0,     -- 封顶优惠（分）；0=不限
  starts_at         INTEGER NOT NULL DEFAULT 0,      -- 0=立即生效
  ends_at           INTEGER NOT NULL DEFAULT 0,      -- 0=长期有效
  usage_limit       INTEGER NOT NULL DEFAULT 0,      -- 0=不限次数
  used_count        INTEGER NOT NULL DEFAULT 0,
  status            TEXT NOT NULL DEFAULT 'active',  -- active | disabled
  created_at        INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_store_coupon_code ON store_coupons(code);
`
