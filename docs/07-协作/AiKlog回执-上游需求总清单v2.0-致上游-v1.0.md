# AiKlog 回执 · 答复上游《需求总清单 v2.0 回执》

- 回执方：爱库录 · AiKlog（AiKdex/AiKlog）
- 日期：2026-09-19
- 对应：上游《上游回执-AiKlog需求总清单-v2.0.md》（联调空间 doc `f9013048`）
- 渠道：联调测试空间「协作资料」
- 标记：`已启动/完成` / `需上游给源码` / `等契约·排期` / `我方自持`

---

## 一、总体

上游回执已收悉并逐项归档。契约与口径层面**已足够**，AiKlog 侧无异议。

但有一处结构性落差必须先讲清楚，否则下一轮仍会空转：**上游回执中多处「已实现」，指的是主系统已实现；AiKlog 是早期 fork（与主系统无共同 git 历史、无法 merge），这些模块的源码不在本仓库内。** 缺的是**源码**，不是契约。

下文 §二 列出 2 个「需上游给源码」的阻塞项；§三 为本轮已完成；§四 为等排期/契约项。

---

## 二、🔴 关键落差 · 需上游给源码（阻塞项）

### 2.1 CoreModules 接入（对应上游 §3.1「已实现」）

上游给出的**字段终版清单 + 门控矩阵 + 迁移指引**非常清晰，我方认可这套设计。但迁移指引的改后示例假设下列类型已存在：

```
service.SourceStore / WebDAVStore / DigestStore / InboxStore / ReviewStore / OrgStore / ChannelStore
```

**实测 AiKlog 现状**：
- `internal/handler/routes.go:38` 仍是位置参数注入、**无 `CoreModules` 参数**：
  `func New(db, cfg, b, aud, gate, files, tags, kb, agent, summarizer, impex, notify) *API`
- `internal/service` 中**不存在**上述任何 Store（仅有 FileStore / TagStore / KBStore / ImpexStore / NotifyStore / AuditStore / Bus 等）。

**影响**：上游 §2.2 Digest、§2.4 Inbox 虽标「已实现（后端）」，AiKlog **无法直接接入**（无 Store 类型 → 无 handler → 无路由可注册）。

**需要上游提供（任一形式即可）**：
1. **源码包**：上述 7 个 Store 的 `internal/service/*.go` ＋ 对应 handler（`digest.go` / `inbox.go` / `sources.go` / `webdav.go` / `review.go` / `org.go`）＋ 建表 DDL/迁移片段 ＋ 路由注册片段；**或**
2. **最小同步文件清单**：一份「从主系统同步到老 fork 需要移植哪些文件」的路径列表（我方按清单逐文件移植、去私有依赖）；**或**
3. 明确告知「这些模块不适合老 fork，需先整体同步主干」，并附同步路径指引。

> 补充确认：上游 §3.1 提到的「AiKlog 已自建 `ai.Registry` + `reg.Register`，与上游同形，直接对齐无需重复造」——**确认无误**。本轮已按此机制新增 `web_clip`（见 §3.1），工具数 6 → 7。

### 2.2 真实支付（对应上游 §四.5「pay/notify 已上线，可直接接」）

AiKlog 现有 `internal/handler/article_access.go` **仅实现密码保护 + 定时发布**（`access_pwd` / `publish_at`），**没有** A–G 的 B 项支付后端（无 `price_cents` / `pay/notify` / `grant_token`）。

因此「可直接接」的前提是拿到**支付后端源码**：
- `internal/handler`：下单、`pay/notify` 回调、`grant_token`（签发/校验）；
- `internal/service`：订单与授权存储、表结构 DDL（orders / grants 等）；
- 支付渠道适配层：网关选择、签名校验口径、金额与币种约定。

契约 A–G 已验，我方要的是**实现源码**。

---

## 三、✅ AiKlog 侧本轮已完成

### 3.1 web_clip 采集工具已注册（对应上游 §3.2「已实现（web_clip）」）

- 新增 `internal/ai/tool_webclip.go`，构造器签名与上游主系统 `cmd/aikmap/main.go:226` **完全一致**：
  `ai.NewWebClipTool(impex *service.ImpexStore, files *service.FileStore, ownerID, homeSpaceID string) *ai.Tool`
- 复用内核 `impex.CreateURLImport`（AiKlog 早已具备，零私有依赖）；参数 `{url, parent?, ua?}`，仅 http/https，目标目录可选。
- 已在 `cmd/aikmap/main.go` 注册 → **AiKlog 工具矩阵 6 → 7**：
  `search_files` / `read_file` / `dedup_files` / `batch_tag_files` / `batch_organize` / `batch_rename` / **`web_clip`**
- 编译验证：`go build ./...` 通过（`BUILD_EXIT=0`）。
- **仍欠**：WebDAV 导入 / RSS 导入构造器（上游 §3.2 列为「排期中」，等官方工具矩阵发布）。

### 3.2 paid 货架验证（对应上游 §3.8「已实现」）

已收到 `pro-advanced` 的 `target` 扩为四壳（含 `aiklog`）并生效。全链路验证（安装 → 402 门禁 → 放行 → 安装成功）安排在评审后的部署窗口执行——当前 `sync/ag-base` 按约定未部署生产，验证需可联网实例。

---

## 四、📋 等契约 / 排期（认领，无异议）

| 项 | 上游结论 | AiKlog 动作 |
|---|---|---|
| Channel 出站通道 | 排期中 P1（CoreModules 第 8 字段预占） | 自持 ChannelView 先行（已认可）；等 P1 凭证表 / 队列 / 频率控制契约 |
| lint / 概念页摘要 | 排期中 P2 | 等 `{rule,file_id,severity,evidence}` 规则落地 |
| AgentTools v2 运行时 | P2（随商业化 v0.2） | v1 兼容不返工；等 v2 运行时 |
| 主题宿主 options_schema / theme_settings / SSR 注入 | 口径已定，P1（主系统亦未实现） | 等宿主表单组件 + `GET /public/site` 增 `theme_settings` |
| 市场卡片图片化 | 排期中 P1 | 等 `cover` / `screenshots[]` / `demo_url` 字段 |
| 结构化分类树 | 排期中 P1 | 等 `GET /public/categories/tree` + `themeContext.categories` |
| Entitlement v0.2 | 随商业化 v0.2 | 等签发服务 diff 指引（公共 API 已一致） |
| 结算 v0.3 | 口径已定 | 等商业化方案 v0.2 的分成/周期/对账数字 |
| 投稿 UGC | 口径已定：Inbox 即通道 | 先用 Inbox 通道；等专用端点（draft→pending→published/rejected）P1 |
| 增量/混合渲染 | 口径已定，P2 | 千级站点可先自建增量导出；上游 P2 后对齐 |
| 自动内链 / 孤岛提醒 | 无统一契约 | 保留自建双链/断链检测；等主题可消费数据源 P2 |

---

## 五、我方自持（确认收到上游知会，无需动作）

1. 主题铺开（~18 套 SPA + SSR 角标）——已开工，按主题规范 §2.5 铺开；
2. ChannelView 自持先行——与 §2.1 一致；
3. 壳侧缺陷修复（install-zip 校验顺序、theme_options title）——知会收到；
4. 多用户前端 UI——知会收到；
5. 已交付分支 `sync/ag-base`（P0/P1/P2 + 批量编辑 C + 主题角标 + 本轮 web_clip）——欢迎评审反馈。

---

## 六、一句话结论

**契约已够，缺的是源码。** 请按 §二 提供「CoreModules 7 个 Store + handler + DDL」与「pay/notify 支付后端」的源码包或最小同步文件清单，AiKlog 即可立即开工接入 Digest / Inbox / 真实支付。

---

*回执方：爱库录 · AiKlog · 2026-09-19*
