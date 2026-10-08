# AiKlog 客服模块（联系人与会话）架构与实施方案

> 编号：**SPEC-CS-001** · 版本：**v1.0.0** · 2026-09-28 · 状态：**方向定稿，待排期实施**
> 适用：AiKlog 自部署发行版（`server/` + `web/`），GitHub `AiKlog`（main）为权威源。
> 读者：产品决策、后端开发、前端开发、发行版打包方。
> 品牌口径：对外一律「爱库录 / AiKlog」；技术标识（Go module、`aikmap_*` storage key）维持不变。

> **修订记录**
> | 版本 | 日期 | 变更 |
> |---|---|---|
> | **v1.0.0** | 2026-09-28 | 首版：定位与边界、三层架构、数据模型（客户身份图谱 / 会话 / 消息 / 出站队列 / 渠道账号）、渠道适配器契约（邮件线程归并、IM 幂等与客服消息窗口）、复用现有底座清单、授权门控、M0–M3 实施分期、验收口径、接线点清单。 |

---

## 0. 结论（先看这里）

**一句话**：客服能力 = 「客户身份图谱 + 会话线程 + 渠道适配」三件事。宿主**已经具备**其中的入站管道、会话两表范式、自动化引擎与知识库检索——**这不是另起炉灶，是在现有骨干上补三个模块，分 M0–M3 四期落地**。

**四点判断**：

1. **「通讯录」= 客户身份图谱，不是通讯录 App**。核心要回答的是「同一客户从网页、邮件、IM 分别来时，能否认成同一个人」。落到 `cs_contacts` + `cs_contact_identities` 两张表，且**必须与 `users`（登录用户）解耦**——绝大多数客户不会注册本站，硬塞一张表会让后续合并/去重/注销互相打架。
2. **邮件与 IM 是两种物种，不共用一个适配器**。邮件有标准协议（IMAP/SMTP），线程靠 `In-Reply-To` / `References` 头归并；IM 无统一协议，需逐渠道适配，入站是 webhook 回调（**必须幂等**），出站是平台 API（**有限速 + 「客服消息窗口」**）。
3. **宿主已有四块可直接复用**：入站管道（`im_bindings` / `im_ingest` 已验证的「渠道 → 归一入库」范式）、会话两表范式（`ai_conversations` / `ai_messages`）、自动化引擎（`workflow_defs` / `workflow_runs`）、知识与 AI（`index_chunks` / `collections` / `skill_packages`）。
4. **差异化在「会话 + 自家知识库」**。客户提问命中站内哪篇文章 → AI 起草回复 → 客服一键确认发送。通用客服 SaaS 没有这一层，这是 AiKlog 做客服的天然优势，也是 `skill_*` 技能包的变现落点。

**边界（本方案明确不做）**：
- 不做「给第三方开客服台」的多租户 SaaS（合规与数据隔离责任外溢）；
- 不做呼叫中心 / 语音 / 工单 SLA 合同管理；
- 不做销售漏斗式 CRM。

**范围**：仅面向**自部署实例的站长自己接待访客与客户**。

---

## 1. 背景与定位

AiKlog 现有能力集中在「内容 + 知识」侧：目录即站点、博客 SSR 轨 + SPA 轨、知识库检索、AI 问答与写作、应用中心（主题/插件/技能）分发。

客服是**同一个知识库的另一个出口**：内容站是「人找知识」，客服是「知识找人」。两者共用一套检索与 AI 能力，因此做客服的边际成本远低于从零起一个客服系统。

已有基础（本方案直接复用，见 §6）：
- `im_bindings` / `im_bind_attempts` / `im_ingest` —— **「外部渠道消息 → 归一落库」这条管道已经在生产跑通**（当前语义是「入站成内容」）；
- `ai_conversations` / `ai_messages` —— 多轮会话的「会话表 + 消息表」范式；
- `workflow_defs` / `workflow_runs` —— 规则化自动动作引擎；
- `notifications` / `mentions` —— 站内提醒；
- `collections` / `index_chunks` / `vectors` —— 知识库检索；
- `skill_packages` / `skill_grants` —— 可分发的能力包；
- `blog_webhooks` —— HMAC 签名的出站事件契约。

---

## 2. 目标架构总览

三层，自上而下为「渠道接入 → 会话与身份核心 → 客服工作台」。消息向下流入，回复向上流出。

| 层 | 模块 | 状态 | 落点 |
|---|---|---|---|
| 渠道接入 | 网页挂件（webchat） | **新增** | `widget.js` + `/api/v1/public/cs/*` |
| 渠道接入 | 邮件渠道（IMAP/SMTP） | **新增** | `cs_` 邮件适配器 + 定时/长连任务 |
| 渠道接入 | IM 适配器（企微/钉钉/飞书/TG/WhatsApp） | **新增** | `ChannelAdapter` 接口逐渠道实现 |
| 会话核心 | 联系人身份图谱 | **新增** | `cs_contacts` + `cs_contact_identities` |
| 会话核心 | 会话线程 | **复用范式** | 借 `ai_conversations` 结构，加 `channel` / `external_thread_id` |
| 会话核心 | 消息存储 | **复用管道** | 借 `im_ingest` 归一范式 + `ai_messages` 结构 |
| 会话核心 | 出站队列（重试/幂等/限速/窗口） | **新增** | `cs_outbound` |
| 工作台 | 统一收件箱 | **新增** | `web/src/views/CsInboxView.vue` |
| 工作台 | 分派与状态 | **复用引擎** | 挂 `workflow_defs` / `workflow_runs` |
| 工作台 | AI 起草回复 | **复用能力** | 接 `collections` 检索 + `skill_packages` |

---

## 3. 数据模型

约定：时间戳一律 **毫秒**（与 `files.*_at` 口径一致）；`TEXT PRIMARY KEY` 走 `newID()`；新表全部 `cs_` 前缀（fork 自建表命名纪律）。

### 3.1 `cs_contacts` —— 客户档案（与 `users` 解耦）

```sql
CREATE TABLE IF NOT EXISTS cs_contacts (
    id             TEXT PRIMARY KEY,
    display_name   TEXT NOT NULL DEFAULT '',      -- 展示名；空则回退首个身份标识
    linked_user_id TEXT NOT NULL DEFAULT '',      -- 可选：该客户恰是站内注册用户时关联 users.id
    email          TEXT NOT NULL DEFAULT '',      -- 归一等值匹配用（可空）
    phone          TEXT NOT NULL DEFAULT '',
    company        TEXT NOT NULL DEFAULT '',
    note           TEXT NOT NULL DEFAULT '',      -- 客服备注（内部可见，永不外发）
    tags           TEXT NOT NULL DEFAULT '[]',    -- JSON 数组
    owner_id       TEXT NOT NULL DEFAULT '',      -- 归属坐席；'' = 未分派
    status         TEXT NOT NULL DEFAULT 'active',-- active | blocked | merged
    merged_into    TEXT NOT NULL DEFAULT '',      -- 合并留痕，不物理删（见 §5）
    created_at     INTEGER NOT NULL DEFAULT 0,
    updated_at     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_cs_contacts_email ON cs_contacts(email);
CREATE INDEX IF NOT EXISTS idx_cs_contacts_owner ON cs_contacts(owner_id, updated_at);
```

### 3.2 `cs_contact_identities` —— 渠道身份映射（一个客户挂多条）

```sql
CREATE TABLE IF NOT EXISTS cs_contact_identities (
    id            TEXT PRIMARY KEY,
    contact_id    TEXT NOT NULL,
    channel       TEXT NOT NULL,              -- email | webchat | wecom | dingtalk | feishu | telegram | whatsapp
    external_id   TEXT NOT NULL,              -- 渠道侧唯一标识：邮箱地址 / openid / tg user id / 挂件 visitor_id
    display       TEXT NOT NULL DEFAULT '',   -- 渠道显示名（如微信昵称）
    verified      INTEGER NOT NULL DEFAULT 0,
    first_seen_at INTEGER NOT NULL DEFAULT 0,
    last_seen_at  INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_cs_ident_channel_ext
    ON cs_contact_identities(channel, external_id);
CREATE INDEX IF NOT EXISTS idx_cs_ident_contact ON cs_contact_identities(contact_id);
```

> `ux_cs_ident_channel_ext` 是**硬约束**：一个渠道身份只能属于一个联系人。入站时按 `(channel, external_id)` upsert，天然完成「同渠道同人复用档案」。

### 3.3 `cs_conversations` —— 会话

```sql
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
    first_reply_at     INTEGER NOT NULL DEFAULT 0,     -- 首次人工响应（SLA 计时起点）
    resolved_at        INTEGER NOT NULL DEFAULT 0,
    created_at         INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_cs_conv_thread
    ON cs_conversations(channel, external_thread_id) WHERE external_thread_id <> '';
CREATE INDEX IF NOT EXISTS idx_cs_conv_status ON cs_conversations(status, last_message_at);
CREATE INDEX IF NOT EXISTS idx_cs_conv_contact ON cs_conversations(contact_id, last_message_at);
```

> ⚠️ 唯一索引**必须带 `WHERE external_thread_id <> ''` 部分索引条件**：SQLite 的唯一索引对空串同样生效，不加条件会导致「第二个空 thread 的会话插入失败」这类难查的写入异常。

### 3.4 `cs_messages` —— 消息（幂等落点）

```sql
CREATE TABLE IF NOT EXISTS cs_messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    direction       TEXT NOT NULL,               -- in | out
    channel         TEXT NOT NULL,
    external_id     TEXT NOT NULL DEFAULT '',    -- 渠道侧消息 id（入站）/ 出站回执 id
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
```

### 3.5 `cs_outbound` —— 出站队列（幂等 + 重试 + 限速 + 窗口）

```sql
CREATE TABLE IF NOT EXISTS cs_outbound (
    id                TEXT PRIMARY KEY,
    conversation_id   TEXT NOT NULL,
    channel           TEXT NOT NULL,
    idempotency_key   TEXT NOT NULL,      -- 坐席端生成的 client_msg_id，防重复点击
    payload           TEXT NOT NULL,      -- JSON: {to, subject, text, html, attachments}
    status            TEXT NOT NULL DEFAULT 'queued', -- queued | sending | sent | failed | blocked
    attempts          INTEGER NOT NULL DEFAULT 0,
    next_attempt_at   INTEGER NOT NULL DEFAULT 0,     -- 指数退避
    window_expires_at INTEGER NOT NULL DEFAULT 0,     -- >0 且已过期 → blocked（须改模板消息）
    last_error        TEXT NOT NULL DEFAULT '',
    sent_external_id  TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_cs_out_idem ON cs_outbound(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_cs_out_due ON cs_outbound(status, next_attempt_at);
```

### 3.6 `cs_channels` —— 渠道账号（每渠道一套接入配置）

```sql
CREATE TABLE IF NOT EXISTS cs_channels (
    id            TEXT PRIMARY KEY,
    channel       TEXT NOT NULL,             -- email | webchat | wecom | ...
    name          TEXT NOT NULL DEFAULT '',
    enabled       INTEGER NOT NULL DEFAULT 0,
    inbound_token TEXT NOT NULL DEFAULT '',  -- webhook 路径 token
    config_enc    TEXT NOT NULL DEFAULT '',  -- AES-GCM 密文（IMAP/SMTP 账密 / AppSecret / Bot Token）
    status        TEXT NOT NULL DEFAULT 'idle', -- idle | ok | error
    last_error    TEXT NOT NULL DEFAULT '',
    last_sync_at  INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_cs_chan ON cs_channels(channel, name);
```

> 凭据一律**不落明文**：`config_enc` 用实例级密钥（落 `settings`，AES-GCM）加密；后台表单仅回显掩码，保存时才覆盖。密钥丢失需重录渠道配置（与 token 池同性质的风险，文档需明示）。

---

## 4. 渠道适配器契约

### 4.1 统一入站入口

```
POST /api/v1/cs/hooks/{channel}/{token}
```

- **匿名可达**（外部平台回调）→ **必须加入 `authmw.go` 白名单**（与 `pay/notify` 同一处理；这是本项目铁律，漏了会 401 静默丢消息）。
- `token` 不匹配 → 直接 404（不泄露端点存在性）。
- 平台签名/secret 校验失败 → 403（企微/飞书/TG 各有一套签名或 secret token 机制）。
- **尽快返回 200**：解析 → 归一 → 幂等 → 落库即回；重活（AI、通知、外推）丢 `jobs` 异步。

### 4.2 幂等（硬要求）

| 方向 | 机制 |
|---|---|
| 入站 | `cs_messages(channel, external_id)` 唯一索引；重复投递走 `INSERT OR IGNORE`，命中直接 200 |
| 出站 | `cs_outbound.idempotency_key` 唯一；坐席端每次发送带 `client_msg_id`（同一草稿重复点击只发一条） |

### 4.3 出站：限速 / 窗口 / 模板

- **限速**：每渠道一个令牌桶（QPS 配置化）。
- **窗口**：IM 平台普遍有「用户先开口后 N 小时内才可主动回复」的客服消息窗口。超期 → `status=blocked`，工作台展示倒计时并在超期时提示「改用模板消息」。
  - 📌 具体小时数**以各平台当期文档为准**，实施时逐渠道核对，勿照抄历史口径。
- **重试**：`next_attempt_at = now + min(2^attempts, 3600) * 1000`，达上限转 `failed`。
- **可见性**：`failed` / `blocked` 不静默——写一条 `author_type='system'` 的 `cs_messages`，工作台可见，避免「以为发出去了」。

### 4.4 邮件渠道

- **收**：IMAP（IDLE 优先，回退轮询，间隔可配），按 UID 去重。
- **线程归并**：
  1. `In-Reply-To` 命中已有 `cs_messages.external_id` → 归入该会话；
  2. 否则取 `References` 链表首个命中项；
  3. 都不命中 → 新建会话，`external_thread_id = 本条 Message-ID`。
  - 🔴 **绝不自编 thread id**：不按 `In-Reply-To` 归并会导致同一串对话被切成多个会话，客户侧看到「断掉的对话」。
- **发**：SMTP，必须带 `In-Reply-To` + `References`；`Re:` 前缀做幂等处理（避免叠加 `Re: Re: Re:`）。
- **认证**：SPF / DKIM / DMARC 属**域名侧运维前置**（不配置则大概率进垃圾箱）。本方案只提供清单，不代管 DNS。
- **退信与自动回复**：识别 `multipart/report`（bounce）、`Auto-Submitted` / `X-Autoreply`（OOO）→ 标 `meta.bounce` / `meta.autoreply`，工作台提示，**不建档为新联系人**。

### 4.5 IM 渠道适配器

统一接口，逐渠道实现：

```go
type ChannelAdapter interface {
    Name() string
    Verify(r *http.Request, body []byte) error          // 签名/secret 校验
    ParseInbound(body []byte) ([]InboundMsg, error)     // 平台报文 → 归一消息
    Send(ctx context.Context, m OutboundMsg) (externalID string, err error)
    WindowTTL() time.Duration                           // 客服消息窗口；0 = 无限制
}
```

渠道差异要点：
- **企业微信 / 微信客服**：需企业主体认证（前置）；存在客服消息窗口与模板消息两条通道。
- **钉钉 / 飞书**：应用机器人 + 事件订阅回调。
- **Telegram**：Bot Token，无窗口限制但有限速。
- **WhatsApp**：需 Business 账号与模板消息审核。

### 4.6 网页挂件（M0 起点，无外部依赖）

- 独立可嵌入脚本（`widget.js`），可挂到任意站点；走宿主 `/api/v1/public/cs/*`。
- 访客**免登录**：首次生成 `visitor_id` 存 localStorage，后端在 `cs_contact_identities(channel='webchat')` 建身份（与现有 `GuestEntry.vue` 的访客入口范式一致）。
- 实时：**SSE 优先**，回退轮询。
- 选它做 M0 的理由：不需要任何平台审核、不依赖域名认证、复用现有 SPA 与文件中心，能在最短路径上跑通「收 → 回」闭环。

---

## 5. 联系人身份归一规则

1. **精确匹配优先**：入站先按 `(channel, external_id)` 查 `cs_contact_identities`，命中即归属该联系人。
2. **跨渠道合并需人工确认**：入站带 email / phone 且命中另一 `cs_contacts` 时，**只生成「待确认合并」提示**交给坐席，**不自动合并**（自动合并一旦误判，会把两个客户的历史串在一起，后果比漏合并严重得多）。
3. **手动合并留痕**：合并后旧档案 `status='merged'` + `merged_into=<新id>`，不物理删除；查询时跟随 `merged_into` 跳转。
4. **渐进补全**：挂件访客后续留邮箱 → 同一浏览器内自动把新身份挂到同一 contact（同渠道同 visitor_id 已归一）。

---

## 6. 复用现有底座（不新建体系）

| 能力 | 现有实现 | 客服侧怎么用 |
|---|---|---|
| 入站管道 | `im_bindings` / `im_ingest` | 复用「渠道 → 归一 → 落库」的解析与幂等范式 |
| 会话两表 | `ai_conversations` / `ai_messages` | 借结构（会话 + 消息），加 `channel` / `external_thread_id` |
| 自动化引擎 | `workflow_defs` / `workflow_runs` | 自动分派、首次应答、超时升级 |
| 站内通知 | `notifications` / `mentions` | 「你被分派了一个会话」 |
| 知识检索 | `collections` / `index_chunks` / `vectors` | AI 起草的检索源 |
| 技能包 | `skill_packages` / `skill_grants` | 客服 AI 能力打包分发 |
| Webhook 出站 | `blog_webhooks`（HMAC 签名） | 会话事件外推（对接第三方系统） |
| 附件与媒体 | `files` / `file_media` | 会话附件复用文件中心（含缩略图/转码链路） |
| 授权门控 | `service/capabilities.go` | 新增 `cs.*` 能力位（§8） |
| 访客入口 | `GuestEntry.vue` | 挂件免登录会话的 UI 范式 |

---

## 7. 客服工作台（前端）

- 路由与页面：`web/src/views/CsInboxView.vue`，三栏布局（会话列表 / 会话详情 / 客户档案侧栏）。
- **属后台页，不进主题体系**（与 `/blog` SSR 轨解耦，避免主题白名单与 SSR 渲染牵连）。
- i18n 按现有约定：中文原文作键、`$t` 仅 `<template>` 可用、纯 `.js` 用 `t()` 且须 `import`；新键双语文档等长。
- 关键交互：未读计数、认领/转派、状态流转（open → pending → resolved → closed）、快捷回复、**AI 起草（一键插入草稿框）**、窗口倒计时提示。

---

## 8. 授权与门控

在 `server/internal/service/capabilities.go` 新增能力位：

| 能力位 | 含义 |
|---|---|
| `cs.inbox` | 统一收件箱与会话读写 |
| `cs.contacts` | 客户档案与身份归一 |
| `cs.channels` | 渠道接入配置与管理 |
| `cs.ai_draft` | AI 起草回复（消耗 AI 配额，接现有配额闸门） |

- 判定沿用现有机制：**`ORDER BY enabled DESC LIMIT 1`**，不可用 `enabled=1` 过滤（既有铁律）。
- 社区/免费版不含客服模块，付费版解锁——与 `org.*`、主题、插件共用同一条应用中心分发管线，不新建计费体系。
- 改门控后必须**关闭态逐端点断言**（既有铁律）。

---

## 9. 实施分期（M0–M3）

| 期 | 建议批次 | 内容 | 前置依赖 | 交付判据 |
|---|---|---|---|---|
| **M0** | B27a | 网页挂件 + `cs_contacts`/`cs_contact_identities` + `cs_conversations`/`cs_messages` + 统一收件箱 | 无外部依赖 | 访客发消息 → 收件箱出现 → 客服回复 → 访客收到（含断线重连） |
| **M1** | B27b | AI 起草（接知识库）+ 分派/状态/SLA（挂 `workflow_defs`） | M0 | 命中站内文章 + 一键插入草稿 + 自动分派生效 |
| **M2** | B27c | 邮件渠道（IMAP 收 / SMTP 发 / 线程归并 / 退信识别） | M0 + 域名 SPF/DKIM 就绪 | 邮件进入同一收件箱，回复落在同一线程 |
| **M3** | B27d | IM 渠道适配器（按客户所在渠道选，先 TG 或企微） | M0 + 平台资质 | webhook 幂等 + 窗口校验 + 模板消息回退 |

**M0 之所以先行**：它不依赖任何外部平台资质与域名配置，且能把数据模型、幂等、收件箱 UI 三件事一次做对——后续 M2/M3 只是往同一模型里加适配器。

---

## 10. 验收口径

- **端到端**：挂件发消息 → 收件箱出现会话 → 客服回复 → 访客收到；刷新/断线后仍能拉全历史。
- **幂等**：同一 `external_id` 重投 3 次 → 消息条数仍为 1。
- **归一**：同一邮箱先经挂件后经邮件 → 归到同一 `contact`（或按 §5 生成待确认合并）。
- **窗口**：构造一个超期会话 → 出站 `blocked` + 工作台提示可见。
- **门控**：关闭态逐端点断言（含匿名 webhook 端点与后台端点两类）。
- **隔离**：客服模块上线不影响现有 `/blog` SSR 轨、主题渲染与既有 API。

---

## 11. 风险与遗留（非阻塞）

| 项 | 说明 | 处置 |
|---|---|---|
| 平台政策时效性 | IM 客服消息窗口、模板消息审核规则会变 | 实施时逐渠道核对当期文档，不照抄 |
| 邮件送达率 | 依赖域名 SPF/DKIM/DMARC，属运维前置 | 在 M2 开工前完成域名侧配置 |
| 附件安全 | 挂件上传不可信 | 复用 `files` 现有类型/大小限制与安全策略 |
| 密钥管理 | `cs_channels.config_enc` 依赖实例级密钥 | 密钥落 `settings`；丢失需重录渠道配置 |
| AI 成本 | `cs.ai_draft` 消耗模型额度 | 接现有 AI 配额三闸门（突发/日配额/全站日预算） |

---

## 附录 A：与现有模块接线点清单

| 位置 | 改动 |
|---|---|
| `server/internal/repo/schema.go` | 新增 6 张 `cs_*` 表 DDL（全部 `CREATE TABLE IF NOT EXISTS`，幂等） |
| `server/internal/handler/routes.go` | 注册 `/api/v1/cs/*`（后台）与 `/api/v1/public/cs/*`、`/api/v1/cs/hooks/{channel}/{token}`（匿名） |
| `server/internal/handler/authmw.go` | 白名单追加 `/api/v1/cs/hooks/`（**必做，否则回调 401**） |
| `server/internal/service/capabilities.go` | 新增 `cs.inbox` / `cs.contacts` / `cs.channels` / `cs.ai_draft` |
| `server/internal/handler/`（新增文件） | `cs_contacts.go` / `cs_conversations.go` / `cs_channels.go` / `cs_hooks.go` / `cs_outbound.go` |
| `server/internal/service/`（新增文件） | `cs_adapter.go`（`ChannelAdapter` 接口与注册表）、`cs_identity.go`（归一规则） |
| `web/src/router` | 新增 `/cs` 后台路由（挂在后台壳内） |
| `web/src/views/CsInboxView.vue` | 三栏工作台 |
| `web/public/widget.js`（或独立构建入口） | 可嵌入挂件脚本 |
| `web/src/i18n/messages.js` | 新增键，中文原文作键、双语等长 |
