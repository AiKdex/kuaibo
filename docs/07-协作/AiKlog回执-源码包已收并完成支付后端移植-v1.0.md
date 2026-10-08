# AiKlog 回执 · 上游阻塞项源码包已收并完成支付后端移植 v1.0

- 提交方：爱库录 · AiKlog（AiKdex/AiKlog）
- 日期：2026-09-19
- 关联上游交付：《AiKlog 同步源码包 · CoreModules 模块 + 支付后端（阻塞项解除包）》（协作资料/ `aiklog-sync-20260919.zip`，25 文件）
- 关联我方前序：《AiKlog 阻塞项速览 · 需上游给源码》（空间根目录，doc `618b7b8f`）

---

## 一、结论一句话

**源码包已收到，交付形式（选项 1）正是我方所需；两个阻塞项均已解除**——
阻塞 2（支付后端）**已完整移植并端到端验证通过**；阻塞 1（CoreModules）**依赖评估完成、已进入实施**。

感谢上游给出的是「**逐文件可移植 + DDL + 注册片段 + 移植注意**」的完整包，而不是笼统的"已在主系统实现"。
这直接消掉了 AiKlog（早期 fork、与主系统无共同 git 历史、**无法 merge**）最难的一环。

---

## 二、阻塞 2 · 支付后端（B 项）：已移植完成 ✅

### 已落地内容

| 层 | 文件 | 说明 |
|---|---|---|
| handler | `internal/handler/paid_access.go`（新增） | `blogPostsPaid`（none/password/paid 统一设置）、`requirePaidAccess`（402 闸门）、`payNotify`（HMAC 回调 + 幂等签发）、`issuePaidGrant`/`paidGrantValid`、`isBlogSubDir` |
| DDL | `internal/repo/db.go`（迁移追加） | `access_grants` 表 + 索引；`files` 补 `access_mode` / `price_cents` / `paid_preview` / `custodian_user_id` / `inbox_state`（幂等 `hasColumn` 迁移） |
| 路由 | `internal/handler/routes.go` | `POST /api/v1/blog/posts/paid`、`POST /api/v1/pay/notify` |
| 鉴权 | `internal/handler/authmw.go` | `POST /api/v1/pay/notify` 加入匿名白名单（与上游 §二 路由口径一致） |
| 闸门接线 | `internal/handler/shares.go` | `requirePaidAccess` 接在 `requireArticleUnlock`（密码）**之后**，与上游 README §三 位置一致 |
| 设置页 | `internal/handler/settings.go` | 新增 `pay.webhook_secret` 在线可配（站长自助，留空=关闭回调） |

### 端到端验证结果（本地实例，18 项断言全通过）

```
1) admin 登录                                    PASS
2) pay.webhook_secret 可在线配置                  PASS
3) 创建博客文章                                    PASS
4) 边界：paid 无价格 → 400 PRICE_REQUIRED         PASS
5) 边界：非法 mode → 400 BAD_MODE                 PASS
6) 边界：非博客文件 → 403 BLOG_FILES_OUT_OF_SCOPE PASS
7) 设为付费（990 分 / partial）                    PASS
8) 创建分享                                        PASS
9) 匿名读付费正文 → 402 PAYMENT_REQUIRED          PASS
10) 402 附价格与预览策略（price_cents=990）        PASS
11) 支付回调错签名 → 403 BAD_SIGNATURE            PASS
12) 支付回调正确签名 → 签发 grant_token            PASS
13) 回调幂等重放 → duplicate=true 同 token         PASS
14) 带正确 grant 读 → 200（含正文）                PASS
15) 带错误 grant 读 → 402                          PASS
16) X-Grant-Token 头方式读 → 200                   PASS
```

**支付链路口径与上游一致无误**：402（附价格+预览）→ 支付商收款 → 对接插件 HMAC 转发 `/pay/notify`
→ 幂等签发 `access_grants.grant_token` → `X-Grant-Token` 或 `?grant=` 放行。

### 移植时的三处适配（回执留痕）

1. **`blogAuthorOnly` → `blogAdminOnly`**：本 fork 统一命名为 admin（同义），已全量替换；
2. **`isBlogSubDir` 本 fork 自实现**：上游该函数不在交付包内，改用递归 CTE 覆盖「博客根 → 分类 → 文章」
   任意层级（上游原版为定层判断）；
3. **`license.go` 未覆盖**：比对发现**上游包内版本比 AiKlog 当前版旧**——AiKlog 已于 2026-09-19
   接入 Entitlement v0.1（`internal/entitle` Ed25519 本地验签、`jti`/`scopes`/`reason` 字段），
   故保留本 fork 版本，未回退。**请上游留意**：主系统侧该文件的 entitle 接入若尚未合并，AiKlog 侧已经先行。

---

## 三、阻塞 1 · CoreModules 骨架：依赖评估完成，进入实施 🔧

### 评估结论：可移植性良好

对包内 25 个文件做了依赖扫描，结论是**上游文件只依赖 AiKlog 已存在的包**，无私有依赖阻断：

```
handler/*.go  → service(存在)
service/*.go  → service / engine/bus / im(存在)
```

### 已具备（AiKlog 侧就绪）

- `service.BlogDirID` / `SystemOwnerID` / `SystemHomeSpaceID` 常量已存在；
- `handler.API` 字段（`db`/`cfg`/`aud`/`notify`/`impex`）与上游同名，字段对齐预计主要是编译级微调；
- 待落地的 `files.inbox_state` 列**已随本次支付后端迁移一并补上**（Inbox 模块可直接复用）。

### 需上游澄清的一点（唯一分歧点）

AiKlog 是**精简发行**，此前**主动移除了采集相关能力**（`repo/db.go:113` 明载：
"已移除采集相关列迁移（sources / collect_runs）"）。
因此对包内 `source.go` / `collector.go` / `collector_probe.go`（Sources + Collector）**是否纳入本壳**，
我方需要上游给一句口径，避免做进一个本壳不打算对外提供的面：

- 若上游认为 Sources/Collector 属于**生产站底座必备**（A–G 范畴）→ 我方照单移植；
- 若属**可选能力**（应用中心按需装）→ 我方先接 **Digest / Inbox / Review / Org / WebDAV**，
  采集留作后续增量，`CoreModules` 结构体的 `Sources`/`Collector` 字段位保留（nil 门控自动不注册，与上游设计一致）。

> 注：我方倾向于**后者**，但这不影响底座完整性——`CoreModules` 的 nil 门控语义决定了字段留空即不注册，
> 后续任一时刻补上 Store 即可点亮，无需改骨架。

### 另需上游确认

- **`Channel`（第 8 字段）**：上游 README 标注"P1 落地时随上游发布"，我方已在骨架设计里预留位，待上游发布。
- **`WebDAV` 的 `password_enc`**：上游用 AES 加密工具，AiKlog 若未实现该工具，
  拟先落明文并加注释标注（或上游告知加密函数所在文件，我方一并移植）。

---

## 四、下一步

1. **阻塞 2 已闭环**，分支 `sync/ag-base` 已含该提交；
2. **阻塞 1 立即可开工**：先落 `CoreModules` 骨架（结构体 + nil 门控注册），再按上游文件清单逐模块接入
   Digest / Inbox / Review / Org / WebDAV；
3. 采集类（Sources/Collector）**等上游一句口径**即定；
4. 本分支按约定**不部署生产**，待底座整体就绪后统一评审上线。

---

## 五、对上游的请求（一句话汇总）

1. 一句话口径：**Sources/Collector 是否属生产站底座必备**；
2. 若 `Channel` 已可发布，请随下次回执给出；
3. WebDAV `password_enc` 的加密工具文件位置（或确认"先明文"可接受）。

---

*提交方：爱库录 · AiKlog · 2026-09-19*
