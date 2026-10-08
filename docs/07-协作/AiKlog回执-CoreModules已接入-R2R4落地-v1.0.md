# AiKlog 回执 · CoreModules 已接入 + R2/R4 落地 + 两处契约落差已修
### 致上游 · 2026-09-19

> 收件方：上游（AiKmap 主系统）｜发起方：AiKlog（爱库录）
> 关联：《上游回执-AiKlog需求清单v3.0-致AiKlog-20260919.md》
> 关联工单：`REQ-001`（R1 校验层同步）· `REQ-002`（R2 能力开关）· `REQ-003`（R3 源包规范）· `BUG-001`（本轮新增：同步包缺件）
> 结论先行：**R2 已按回执口径实现并端到端验证；R4/R5/R6 契约已落地；阻塞 1 的 5 个可编译模块已接入；同时发现并修正 2 处双方契约落差（支付密钥键名、平台令牌缺失）。**

---

## 一、已落地（本壳实测通过）

### 1. R2 能力开关（capability gate）—— 按上游采纳口径实现 ✅

上游口径：「安装 capacity 应用写一条 `blog_plugins` 记录（kind=capacity, capabilities=[...], enabled=1）；
main 装配阶段查已启用记录决定是否构造 Store 注入 CoreModules；模块为 nil → 路由不注册（404 = 能力未启用）」。

本壳实现（`server/cmd/aikmap/capabilities.go` + `handler.CoreModules`）：

| 层 | 实现 |
|---|---|
| 注入面 | `handler.CoreModules{Sources,Collector,WebDAV,Digest,Inbox,Review,Org}`；`handler.New(..., mods CoreModules)`，新增内核模块只扩字段、签名不再变 |
| 门控判定 | ① 站长开关 `settings capability.<name>` → ② 应用中心记录 `blog_plugins(kind='capacity' AND id/name=<能力名>)` 的 `enabled` → ③ 内置默认 `true` |
| 停用效果 | 字段传 nil → 路由不注册；**并补了 `/api/v1/*` 兜底 404 JSON**（此前未注册 API 会落 SPA HTML 兜底返 200，客户端无法区分"能力未启用"） |
| 站长开关 | `capability.{collector,webdav,digest,inbox,review,org}` 六键已进设置白名单（中文可读、后台可编辑） |

> **默认值说明（需上游确认是否接受）**：第 ③ 级兜底本壳取 `true`（精简发行仍内置这些能力，装/卸载走应用中心）。
> 若上游要求 strict「必须装应用才可用」，改动点仅一处（`capabilities.go` 兜底返回 `false`），口径可一键对齐。
> 之所以先取 true：当前市场尚无官方 capacity 应用可装，strict 默认会让能力对站长完全不可见。

**实测**：`capability.collector=false` → `GET /api/v1/sources` 返回 **404 API_NOT_FOUND**，inbox/review/digest 仍 200；
改回 `true` → 恢复 200（重启生效，装配期判定）。

### 2. R4 平台令牌（service token）—— 本壳原缺失，已补齐 ✅

回执 §二 R4 称 `security.service_token`（config.go:127 + authmw.go:139/175 白名单）。
**实测本 fork 全域 `grep service_token` 零命中**——即该机制本壳从未移植，第三方采集器/连接器无法调内核。

已按上游 `serviceTokenPath` 原样移植（`handler/authmw.go`）：

- 三令牌：`security.service_token`（全量=read+ingest）/ `security.readonly_token`（只读）/ `security.ingest_token`（采集写），留空=停用
- 白名单 9 条：`GET /files`、`GET /files/{id}/content`、`GET /license`、`POST /files/doc`、`POST /files/mkdir`、`DELETE /files/{id}`、`PUT /files/{id}/content`、`PUT /files/{id}/status`、`POST /shares`
- 身份记为 `service.SystemOwnerID`；白名单外一律 401（防令牌泄露横向扩散）
- 三键均已进设置白名单（站长可自助配置/轮换）

**实测**：令牌 `GET /files` 200 → `GET /admin/settings` **401**；令牌 `POST /files/doc` 201；错误令牌 401。

### 3. 阻塞 1：CoreModules 接入（5/7 模块）✅ / WebDAV+Org 待补 ⏳

| 模块 | service | handler | 路由 | 状态 |
|---|---|---|---|---|
| Sources + Collector | ✅ | ✅ | ✅ | **已接入**（`POST /collect/run` 202 + run_id；`GET /collect/runs`） |
| Inbox | ✅ | ✅ | ✅ | **已接入**（复用 `files.inbox_state`，无独立表） |
| Digest | ✅ | ✅ | ✅ | **已接入**（无独立表，读 files+settings 汇总） |
| Review | ✅ | ✅ | ✅ | **已接入**（review_items/review_logs 已建表） |
| WebDAV | ✅ service 编译通过 | ⏳ | ⏳ | **待接线**：见下 |
| Org | ✅ service 编译通过 | ⏳ | ⏳ | **待接线**：见下 |

DDL 已按 ddl.sql 落地（`repo/db.go` 迁移段）：`sources`、`collect_runs`、`webdav_mounts`、`review_items`、`review_logs` + 索引；
`files.inbox_state / access_mode / price_cents / paid_preview / custodian_user_id` 已在上一轮补齐。

第三方依赖：`collector.go` 需 `github.com/bogdanfinn/tls-client`(v1.16.0) + `fhttp`(v0.6.9)，
`webdav.go` 需 `golang.org/x/net/webdav`——已按上游 go.mod 版本接入（`golang.org/x/net` 升至 v0.59.0）。

**实测（本壳本地实例）**：`GET /sources` 200 → 建源 201 → `POST /collect/run` **202 + run_id** → `GET /collect/runs` 有记录
（`status=partial`，探针源指向本机 health 端点，SSRF 防护生效故 failed=1，符合预期）；
`/inbox`、`/inbox/unread-count`、`/digest/preview`、`/review/queue`、`/review/stats` 全 200。

### 4. WebDAV / Org 未接线的原因（**不是设计问题，是本 fork 与主系统在核心存储层的分歧**）

移植时实测到以下具体缺口，均需先补本壳核心层再注入（当前两模块保持 nil = 能力未启用，路由 404）：

| # | 缺口 | 用途 |
|---|---|---|
| 1 | `service.File` 缺 `ViewCount` 字段（列已存在，结构体未扫） | WebDAV 文件列表展示 |
| 2 | `service.FileStore` 缺 `ReplaceContentBinary` | WebDAV 写回 |
| 3 | `service.NotifyStore` 缺 `AddUser` | Org 成员变更通知 |
| 4 | handler 侧缺 `spaceRole` / `quotaUse` / `quotaUploadMB` 等共享辅助 | Org 权限、WebDAV 配额 |

上游 `handler/{webdav,org}.go` 已收到并归档在 `server/internal/handler/_pending_upstream/`（Go 忽略 `_` 前缀目录，不影响构建），
补齐上表 4 项即可接线。**如上游这 4 处有现成实现，直接给最小文件即可，我方接入。**

---

## 二、修正的两处契约落差（**建议上游确认，属跨壳一致性事项**）

### 落差 1 · 支付回调密钥键名不一致 ⚠️

| | 键名 |
|---|---|
| 上游回执 §一·阻塞 2 | `pay.notify_secret`（settings.go:138） |
| 本壳上一轮移植时（自定名） | ~~`pay.webhook_secret`~~ |

键名不一致会导致**支付对接插件按上游文档配置密钥、宿主却读另一个键**，回调恒返回 503。

- 已**以上游契约为准**：设置白名单注册 `pay.notify_secret`，`paid_access.go` 读 `pay.notify_secret`（`paidSecret()`），
  同时**兼容回退旧键 `pay.webhook_secret`**（已配置站点升级不失效）。
- 实测：`pay.notify_secret=TestPaySecret2026` → HMAC 回调签发 grant（200）→ 错签名 403 → 匿名读付费正文 **402** → 带 grant 200。

### 落差 2 · 平台令牌（R4）本壳完全缺失

见上文 §一·2。**建议上游在同步清单里把「服务令牌」列为独立项**——它不在阻塞项解除包内，
若不是本次逐条核对回执，本壳会一直以为"连接器协议已具备"。

---

## 三、本轮交付缺件（已开 `BUG-001`）

回执 §四列了 6 项「本轮可同步的源码包清单」，但**协作资料/ 内实际包仍是 09-19 18:48 那一版（25 文件）**，
23:12 的回执未附新包。逐项核对结果：

| 回执 §四清单 | 包内是否已含 | 说明 |
|---|---|---|
| ① `handler/routes.go` | ✅（作参考） | 已据其 CoreModules 定义适配 |
| ② service 7 Store | ✅ | 全部收到并编译通过 |
| ③ handler 7 个 | ✅ | source/digest/inbox/review 已接线；webdav/org 归入待补 |
| ④ `engine/bus/topics.go`（R6） | ❌ 未含 | **本壳已有**（`internal/engine/bus/topics.go` 含 `KernelTopics`/`ValidateTopic`），无需重复给 |
| ⑤ `repo/schema.go` + `repo/db.go` 迁移 | ❌ 未含 | 已用包内 `ddl.sql` 替代完成建表/迁移 |
| ⑥ `entitle/entitle.go`（R7） | ❌ 未含 | **本壳版本更新**（已接 Entitlement v0.1，含 ed25519），勿覆盖 |
| **R1 · `handler/blog_plugins.go`（v2 校验层）** | ❌ **未含** | **真缺**：`validatePluginV2` + manifest v2 六列（capabilities/backend_entry/routes/min_schema/max_schema）——本壳 `grep` 零命中 |
| **R3 · 《采集源包格式规范 v1》** | ❌ 未交付 | 回执称"本期先给规范文档"，文档未到 |

**请上游给**：① `handler/blog_plugins.go`（R1，含 manifest v2 结构与校验）+ `db.go` 六列迁移片段；② 《采集源包格式规范 v1》文档。

> 另：`engine/bus/topics.go` 与 `entitle/entitle.go` 我方已具备（后者更新），**不必随包给**，避免我们误覆盖回退。

---

## 四、R1–R11 状态对照（本壳视角）

| 项 | 上游结论 | 本壳动作 |
|---|---|---|
| R1 v2 校验层 | 已实现，给清单 | ⏳ 等 `blog_plugins.go` 文件（清单已收到，代码未随包） |
| R2 能力开关 | 已对齐（采纳拟实现） | ✅ **已实现并实测**（默认值口径见 §一·1 说明） |
| R3 源包格式 | 上游出规范 v1 | ⏳ 等文档 |
| R4 平台令牌 | 已实现，给契约 | ✅ **已移植并实测**（本壳原缺失） |
| R5 collect API | 已实现 | ✅ 已接入并实测（202+run_id / runs 查询） |
| R6 事件契约 v1 | 已实现 | ✅ 本壳已有 `KernelTopics`/`ValidateTopic` + hooks 校验 |
| R7 计费授权 | 机制就绪，扩 scope | ✅ 本壳已接 Entitlement v0.1；`capability:*` scope 可直纳 |
| R8 采集合规边界 | 需出边界文档 | ⏳ 等《采集商品上架审核规范 v1》 |
| R9 第三方采集器形态 | = service token + collect API | ✅ 两半均已具备（R4+R5），可开工 |
| R10 官方源包/分成 | 商业化排期 | ⏳ 等结算 v0.3 排期 |
| R11 能力页壳 | 采纳 AiKlog 回贡 | ⏳ 待 R1 到位后实现通用能力模块页壳并回贡 |

---

## 五、附：本壳本轮额外修正的缺陷（自测发现，非上游责任）

1. **`/api/v1/*` 未注册路径落 SPA 兜底返 200** —— 导致"能力未启用"无法与"接口写错"区分；已加 `/api/v1/` 兜底 404 JSON。
2. **早期提交半成品**：`link_check.go`、`blogPostsBatch` 的 handler 已提交，但路由注册未进 `routes.go`（等效于功能未上线）；
   本轮已补注册并实测 200。（web_clip 也曾在工作区丢失，已从 git 树版恢复。）
3. 工作区曾整体落后于 git 历史（web_clip/链接体检/批量编辑/前端 contentState 组件仅存在于对象库），已从树版补齐盘面，避免后续提交回退。

---

*AiKlog（爱库录）· 2026-09-19*
