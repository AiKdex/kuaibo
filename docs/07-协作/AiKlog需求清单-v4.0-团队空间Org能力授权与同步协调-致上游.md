# AiKlog 需求清单 v4.0 · 团队空间（Org）能力授权层级与同步协调

> 发起方：AiKlog（爱库录）｜收件方：上游（AiKmap 主系统）
> 日期：2026-09-29
> 前置：B31 批次已上线（生产 `aiklog.cn` 与 124 实例 health 200，serving `index-Dj4r2w1G.js`，bin md5 `c2003d5b…` 互等；代码 commit `52480237`、文档 commit `fff94343`）。当前远程分支头：main=`895fb013`、sync/ag-base=`fff94343`（GitHub ls-remote 核验，2026-09-29）。
> 本清单与 v1.0/v2.0/v3.0 互补：前序已覆盖底座对接、能力应用与采集商品化；本清单**只问两件事**——① 团队空间该不该/怎么跟付费授权绑定；② Org 在 sync/ag-base 的权威版本与协作移植机制。**不做代码重写**——Org 引擎本 fork 早已落地。

---

## 0. 缘起

产品侧出现明确诉求：用 AiKlog 搭「团队内部 Wiki / 多人在线协作」。经我方实测核查，Org 模块在代码侧**已经实装且接线完成**（service/org.go + handler/org.go 全套守卫、config 四开关、CapOrg 能力项、space_members 表、前端 OrgView）。因此本清单**不是要求上游重写 Org**，而是补齐两处上游拥有决策权、本 fork 不应擅自定档的事项：

1. **授权层级**：Org 当前与 Entitlement 完全解耦（社区版也能手动开），但上游把 Org 标注为「企业知识库组织模块 M0/M1/M2」，疑似企业/付费定位——需上游确认开闸模型。
2. **同步协调**：Org 服务端代码在 main 与 sync/ag-base 零差异，但历史上 sync/ag-base 曾 force-update、fork 按 SHA 移植（禁 merge）；需上游约定 Org 改动时的协作机制，避免覆盖 fork 自有前端/建表增量。

---

## 1. 我方已确认的技术结论（请上游核对）

### 1.1 Org 模块本 fork 已实装且已接线（非半成品）

证据链（均为本 fork `main` 实测）：

| 维度 | 落点 | 说明 |
|---|---|---|
| 服务层 | `server/internal/service/org.go` | `Enabled()`/`TreeEnabled()`/`TransferEnabled()` 三层开关联动；部门空间（`spaces.kind='team'` + 节点绑定）；岗位移交流全链路（created→executed→completed→cancel）+ 审计 |
| 守卫层 | `server/internal/handler/org.go` | `orgBlocked` / `orgTreeEnabled` / `orgDepartmentEnabled` / `orgTransferEnabled` 四重守卫，未开启返回 `403 ORG_*_DISABLED` |
| 配置层 | `server/internal/config/config.go:134-137` | `org.enabled`/`org.tree`/`org.department`/`org.transfer` 默认 `false` |
| 设置项 | `server/internal/handler/settings.go:212-225` | 四开关 desc 完整（含联动语义说明） |
| 能力项 | `server/internal/service/capabilities.go:33/43` + `settings.go:525` | `CapOrg="org"` 已列入 `CapabilityNames`，设置页「组织架构」卡片已挂 |
| 数据层 | `server/internal/repo/db.go:477` | `space_members` 表 `CREATE TABLE IF NOT EXISTS` + 索引（部门空间成员查询依赖） |
| 前端 | `web/src/views/OrgView.vue` | 组织树 / 部门空间 / 移交流 UI 已存在 |
| 单测 | `service/capabilities_test.go`、`handler/settings_org_test.go` | 覆盖 CapOrg 开关判定 + 四开关联动语义 |

实测结论：`org.enabled=false` 时所有组织写端点一律 `403 ORG_*_DISABLED`；开总开关后组织树写端点仍受 `org.tree` 正确拦截（预期行为）。**Org 是完整功能，不是占位 TODO。**

### 1.2 当前 Org 与授权/许可完全解耦（这是要请上游定档的核心）

`grep -rn "org\|CapOrg\|capability.org" server/internal/entitle/` → **零命中**。Entitlement 包（`entitle.go`）仅处理 `edition=community|pro` 与 `scopes`（现仅 `all` / `shell:aiklog` / `multisite`，见 `site_admin.go:388` 的 `MultisiteFeature`）。Org **不在**任何 license scope，也**不参与** edition 判定（`license.go:44-76`：key 有效→pro，否则 community，无任何 org 分支）。

结论：当前团队空间 = 纯手动 config 开关 + CapOrg 能力开关，**任何 edition（含 community）均可手动开启，无付费门槛**。

### 1.3 本 fork 与 sync/ag-base 在 Org 服务端代码上零差异

`git diff 895fb013 fff94343 -- server/internal/service/org.go server/internal/handler/org.go server/internal/handler/settings.go server/cmd/aikmap/capabilities.go server/internal/repo/db.go` → **全域零 diff**。

即：Org 服务端实现已与同步基完全一致，不存在「fork 改了而上游没有」的 Org 服务端分叉。唯一 Org 相关分叉在**前端**（`git diff` 显示 `web/src/views/OrgView.vue` +2 行、`web/src/views/SettingsView.vue` 组织页签 +5 行），属本壳 B 批次美化增量，非功能差异。

---

## 2. 需上游配合事项（R1–R2）

### R1 · 团队空间（org.enabled / CapOrg）的授权层级确认【P0 · 阻塞上线决策】

**现状**：Org 不受 Entitlement 约束，community 也能开；但上游在 `cmd/aikmap/capabilities.go:22` 把 Org 标注为「企业知识库组织模块 M0/M1/M2」，疑似企业/付费定位。

**问题**：本 fork 不应单方面决定「团队空间免费送」或「团队空间收费」——这是上游的产品/授权决策。需上游明确：

- **(a)** 团队空间是否应纳入付费授权（新增 entitle `feature:org` scope，由 license 控制开闸），还是保持 community 可自由开启？
- **(b)** 若付费：落在哪个 tier（pro？），scope 命名是否与 `multisite` 同约定（`feature:org`），以及验签通过后本 fork 应在哪一层做闸门（拟在 `service/org.go Enabled()` 增加 license-scope 校验，与 community 回落逻辑一致）。
- **(c)** 若免费：请明确，我方保持现状（手动开关），并在**产品文档**标注「多空间数据模型已就绪，协作能力规划中」（不写成「已支持多人协作」）。

**请给**：授权模型**口径**即可（无需源码）；若付费，请给 `feature:org` 的 scope 命名 + 验签接入点（复用 `entitle.Validate(payload, feature)` 的 feature 形参，参考 `site_admin.go:388` 的 `MultisiteFeature` 范式）。

**我方拟实现（供参考，若主系统另有设计请直接给口径，避免单壳差异化造成合并冲突）**：`Enabled()` 内 `if edition==community && !licenseHas("feature:org") → 视作未开启`；前端 OrgView 在 community 无该 scope 时展示「需升级」而非裸 `403`。

**拿到后能开工什么**：1 个工作日内完成 org 授权闸门 + 前端提示 + 授权单测（扩 `capabilities_test.go` / `settings_org_test.go`）。

### R2 · Org 在 sync/ag-base 的权威版本与协作移植机制【P1 · 防覆盖】

**现状**：Org 服务端代码 main 与 sync/ag-base 零差异（§1.3）；但历史上 sync/ag-base 曾 force-update，fork 按 SHA 增量移植（禁 merge，见仓库 git-ref 纪律）。

**风险**：上游若改 Org（如重构 `space_members`、增列、重写 OrgView、调整 `CapOrg` 注册），本 fork 的前端增量（OrgView/SettingsView 微调）与建表迁移（`repo/db.go` 的 `space_members` 自建）可能被整文件覆盖或冲突。

**请给**：

- **(a)** Org 相关文件的权威 SHA 锚点（当前可锚 main=`895fb013` / sync/ag-base=`fff94343`，二者 Org 一致）；
- **(b)** 上游改动 Org 时的协作约定——是否可在提交信息打 `[org]` 标，或合并前在协作区发「Org 变更预告 + 影响文件清单 + 最小 diff」，便于我方按 SHA 增量移植，而非整文件拉取；
- **(c)** `space_members` 表是否纳入上游正式 schema（当前由 fork 在 `repo/db.go:477` 以 `CREATE TABLE IF NOT EXISTS` 自建；若上游后续也建同表，需约定权威 DDL 与迁移顺序，防重复建表/列冲突）。

**我方拟遵循**：收到上游 Org 变更预告后，仅对受影响文件做 plumbing 增量移植，绝不整文件覆盖；fork 自有增量（OrgView/SettingsView 前端微调、`space_members` 自建迁移）优先保留，冲突时回协作区开 `BUG` 单。

**拿到后能开工什么**：把本 fork 的 Org 前端增量与 `space_members` 迁移整理为「fork 自有补丁清单」归档，建立与 sync/ag-base 的**逐文件映射表**，后续每次上游 Org 变更可 10 分钟内定位与移植。

---

## 3. 我方自持事项（不需上游）

- **团队空间基础代码已实装**（§1.1），无需上游重写；多空间数据模型（`spaces.kind=home|shared|team`、`files.space_id`、`index_chunks.space_id`、`space_members`）已就绪。
- **单用户固定归属的收口**列为我方自持增量：`server/internal/ai/tool_search.go:17`、`tool_webclip.go:19` 的 `homeSpaceID`/`ownerID` TODO（注释明示「单用户阶段固定归属，多用户阶段按请求上下文解析」），与 R1/R2 互不阻塞。
- **产品文档口径**由我方按 R1(c) 统一措辞：「多空间数据模型已就绪，协作能力规划中」，不写成「已支持多人协作」。

---

## 4. 交付节奏建议

| 项 | 优先级 | 是否阻塞 | 给了什么即可开工 |
|---|---|---|---|
| R1 授权层级 | **P0** | 阻塞团队空间商业化定档 | 口径（免费 / 或 `feature:org` scope 命名 + 验签接入点） |
| R2 同步协调 | **P1** | 不阻塞，防未来覆盖 | 权威 SHA 锚点 + 变更预告约定 + space_members schema 归属 |

> 建议上游先回 R1 口径（决定团队空间是否收费），R2 可同期给机制约定。两项的「拿到后我方能做什么」见 §2 各条末。

---

## 5. 附：本 fork 实测依据（文件:行号 / grep 结果）

- **Entitlement 无 org**：`grep -rn "org\|CapOrg\|capability.org" server/internal/entitle/` → 零命中（`entitle.go` / `entitle_test.go` / `public_keys.go`）。
- **四开关默认关**：`config/config.go:134-137`（`org.enabled`/`org.tree`/`org.department`/`org.transfer` 均 `false`）。
- **设置项 desc**：`handler/settings.go:212-225`。
- **CapOrg 能力项**：`service/capabilities.go:33`（`CapOrg="org"`）、`:43`（`CapabilityNames`）、`handler/settings.go:525`（`{service.CapOrg,"组织架构",...}`）。
- **守卫层**：`handler/org.go:17`（`enabled`）、`:32`（`orgBlocked`）、`:42`（`orgTreeEnabled`）、`:47`（`orgDepartmentEnabled`）、`:432`（`orgTransferEnabled`）。
- **space_members 自建**：`repo/db.go:477`（`CREATE TABLE IF NOT EXISTS space_members`）、`service/org.go:431/440/469/532`、`handler/org.go:188`。
- **main vs sync/ag-base 零差异**：`git diff 895fb013 fff94343` 对 Org 服务端 5 文件零 diff；前端 `OrgView.vue` +2 / `SettingsView.vue` +5。
- **单测**：`service/capabilities_test.go`、`handler/settings_org_test.go`（覆盖 CapOrg 判定 + 四开关联动）。
- **license 判定**：`handler/license.go:44-76`（key 有效→pro，否则 community，无 org 分支）；`site_admin.go:388`（`entitle.ValidateProduct(key, entitle.MultisiteFeature)` 范式）。

> 将同步在协作区开 **REQ-008（R1 授权层级）** 与 **REQ-009（R2 同步协调）**，`doc_id` 关联本清单。

---

## 6. 上游已确认与移植记录（2026-09-30）

协作区 **REQ-008 / REQ-009 均已 `closed`（已处理）**，上游给了明确口径，本 fork 已据此完成移植。

### 6.1 上游口径（来自工单 resolution）

- **R1 授权层级（REQ-008）**：
  - 基础组织能力（M0 组织树/岗位快照/只读）= **community 免费**，`org.enabled` 保持可手动开启。
  - 付费能力（**M1 部门空间 / M2 移交流**）= **付费**，由 `orgPaidGate`（`users.tier=pro`，owner/admin 豁免）执行。
  - 授权 scope 统一 **`feature:org`**、**tier=pro**；license 门禁接入点 = 在 `orgPaidGate` 追加 `HasFeature(org)`。
  - **产品文档口径**：「多空间数据模型已就绪，协作/组织能力按付费分层开放」。
- **R2 同步协调（REQ-009）**：
  - 上游 Org 锚点 **HEAD=`7bea3b9`（2026-09-30）**。
  - 自本单起上游 Org 变更 commit 强制加 **`[org]` 标 + 协作区预告 + 最小 diff**。
  - `space_members` 已纳入**上游 schema v8**（`UNIQUE(space_id,user_id)` + 双索引，迁移在 org 三表之后）；fork 自建 DDL 与上游不一致处**以上游 DDL 为准**。

### 6.2 本 fork 移植动作

- **R1（commit 待 plumbing 推 main）**：`handler/org.go` 的 `orgPaidGate` 在 `isAdmin` 豁免之外，新增
  站点级 `feature:org` 授权（`currentLicense()` + `HasFeature("org")`）+ 站点级 pro 授权（`licenseEdition()=="pro"`）。
  **同时修复潜伏 bug**：原 `SELECT tier FROM users` 因 `users` 表无 per-user `tier` 列会报错；
  本 fork 为站点级 license 模型，上游「`users.tier=pro`」落地为站点级 pro 授权。新增单测 `TestOrgPaidGateSiteLicense`。
  > ⚠️ 模型差异：上游是 per-user 套餐，本 fork 是站点级 license，故 org 付费能力以「站点授权」为粒度开放给全体登录用户。
- **R2（同一 commit）**：`repo/db.go` 的 `space_members` 迁移补显式 `idx_space_members_space` 索引，对齐上游 schema v8「双索引」（PRIMARY KEY 已含 UNIQUE，`IF NOT EXISTS` 对存量库无副作用）。

### 6.3 待取回 / 后续

- **锚点 `7bea3b9` 尚未进入本 fork 镜像的 `sync/ag-base`（当前 `fff94343`）**：ls-remote 未列出、fetch 后 `cat-file` 找不到。
  待上游把该 Org 提交推到可追踪分支后，再做 Org 逐文件映射与「按 SHA 增量移植」核验（禁 merge）。
- **`[org]` 流程**：今后上游 Org 变更须带 `[org]` 标 + 协作区预告 + 最小 diff；本 fork 改 Org 相关文件（`service/org.go` / `handler/org.go` / `cmd/aikmap/capabilities.go` / `handler/settings.go` / `OrgView.vue` / `SettingsView.vue`）前，先与 `sync/ag-base` 最新 SHA 比对，避免被 force-update 覆盖。

### 6.4 部署状态（2026-09-30 已上线两实例）

- **Git 链**：R1/R2 移植提交 `a28bc65`（parent `3232b18`）与 B31 主线（`fff9434`）为平行分支 → 已做**双父合并提交 `f52673d`**（parent `fff9434` + `a28bc65`，两链均为本 fork 自有提交，不违反「禁 merge 上游」），main ff 推送，`ls-remote` 核验。
- **构建（124 服务器）**：源码 tar 解至 `~/aiklog_build` 根 → `vite build`（entry **`index-Dj4r2w1G.js` 510998 B，与 B31 同名同 hash——前端零改动）→ 覆盖两份 webdist → `go build`**；bin md5 **`913f4bbc95bc58257370e2fb130aca24`**（54679643 B）。
- **124 实例**：回滚点 `aiklog.bak.20260930124939`（bin+DB 三件套）→ systemd `aiklog.service` 重启 → **health 200**、PID 1456806；entry md5 dist/线上互等 `909a2d30162f0b4176d545bb3ed123ea`；**R2 索引 `idx_space_members_space` 已随启动迁移落库**；二进制含 `PAYMENT_REQUIRED`（R1 门禁）。
- **生产实例（逃生通道：124 构建产物直推）**：bin 经 124→本地→生产三方 md5 互等 → 回滚点 `aiklog.bak.20260930125438`（bin+DB 三件套）→ `pkill -x aiklog` → 换 bin → `sh start_aiklog.sh` → **health 200**、PID 3068761；R2 索引同样落库。
- **外网复验**：`/api/v1/health` `/app` `/blog` `/public/site` 全 200；entry 资产 `200 text/javascript / 510998 B / md5 909a2d30…` 与构建树、两实例**三方互等**。
- 两实例临时部署脚本/源码包/构建产物均已清理（124 `~/aiklog_build/_b31b*`、生产 `/root/aiklog.b31b`）。
