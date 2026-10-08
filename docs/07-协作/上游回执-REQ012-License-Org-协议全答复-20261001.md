# 上游回执 · REQ-012 License/Org/协议全答复（2026-10-01）

> 来源：协作区联调测试空间，REQ-012 工单 `resolution`（closed，2026-10-01 17:36，上游=AiKMap 主系统）。
> 归档时间：2026-10-02 07:20。**状态：答复全部收到 ✅；唯「正式 license key」实物缺位（详见文末核查），已另开 REQ-013 催补。**

## P0-1.1 公钥书面确认 ✅

内嵌公钥 `0aEys349vIDHxOc1D3naGulh3u9QFoWHHFOeyqCYO0Q=`（issuer=aikmap-license，entitle.PublicKeys 编译期内嵌）即上游正式签名公钥；与我方指纹一致，确认。签发私钥与公钥配对已自验（签发→验签闭环通过）。**v0.1 协议期内不轮换**；轮换机制已预留（public_keys.go 支持多条目 + iss 匹配平滑过渡，届时发变更公告）。

## P0-1.2 正式 license ⚠️ 已签发、key 实物缺位

上游称已签发并投递本空间根目录：`AiKMap-正式License-official-aiklog-2026-10-01.key`
- 载荷：edition=pro；scopes=[shell:aiklog, feature:family, feature:org]；features=[family, org]；exp=2027-06-30T23:59:59Z；jti=official-aiklog-001；sub 空（不绑定实例）
- 激活方式：设置页激活该 key（或 fork 激活流程回源 POST /api/v1/license/verify 验签；公开端点）
- test key（test-all-shells-001，exp 2026-10-18）可退役；到期后 fork 侧 licenseEdition 不再回落 community

**核查（2026-10-02 07:15）**：空间根目录 30 项 + 协作资料/ 12 项全量列出，无任何 `.key` 文件；分页参数（limit/page/page_size）与关键字过滤均不改变结果（该列表固定 30 条上限），已排除分页遗漏。→ **key 实物未投递到位**，另开 REQ-013 催补（要求三选一：重投根目录 / key 全文贴入工单 resolution / 在线验签端点直接签发）。

## P0-1.3 feature scope 语义定稿 ✅

license payload 双通道：scopes（`feature:<name>` / `shell:<id>` / `all`）+ features（裸名）；`HasFeature(name)`=scopes 含 feature:name 或 features 含 name；`IsPro(shell)`=edition=pro 且 HasScope(shell)。**family 与 org 是独立 feature（非同一 entitlement 子项）**，命名定稿 `feature:family` / `feature:org`；edition=pro 为全局档位。我方放行模型（isAdmin OR HasFeature OR edition=pro）与上游语义一致，**可锁定**。

## P1 Org 授权模型 ✅

- M1（部门空间）/ M2（移交流）license 层统一以 `feature:org` 授权（不再细分 M1/M2；产品分层由前端/权限展示承载，付费判定=HasFeature("org") 或 edition=pro）。
- 上游一期 orgPaidGate 为 users.tier=pro（DB 用户套餐模型，SaaS 多用户）；fork 单用户站点级模型下用 licenseEdition()+HasFeature("org") 放行属正确适配，语义对齐、允许实现差异。
- Org 锚点 `7bea3b9` 已合入 upstream main（上游实测为 main 祖先；sync/family 分支亦含），fork 可直接 fetch。

## P2 协议/工具 ✅（存档备查）

- AgentTools 规格：以 `docs/应用中心壳端对接规范 v1`（上游仓库 commit `3498cc6`）为准。
- 测试坏包三型：①zip 内 sha256 与 index.json 条目不符；②zip 缺 manifest.json 或 manifest 缺 id/version 必填字段；③manifest.id 与 index.json 条目 id 不一致。测试货架 A3 含 paid 与坏包。
- 版本字段口径：semver MAJOR.MINOR.PATCH，由 ldflags -X 注入（非 package.json）；market item.version 与 manifest.version 严格一致；预发布用 -rc.N。
- blog 市场公开门户合规口径：公开 /market 门户与 /market/index.json 仅承载内容合规的插件/工具/主题 manifest（作者署名+版本+sha256+target），不涉交易闭环；付费 tier 由 license 门禁在安装时判定，门户不展示密钥类信息。各壳对自身发布内容负合规责任。
- 单壳冲突处理：SPA 前端与公开路由共用同一 Go 入口；按路径/前缀分发（#/ 走 SPA，/p/:token、/market 等公开路由直出）。

## 四、勿覆盖确认 ✅

family.go/org.go 门禁、entitle 验签、family 前端模块、org 前端模块、blog_market 移植已在我方落地，上游不重复交付；如需修改上游会在我方工单/文档下留言对齐。

## 五、上游承诺与我方义务

- 收到即算闭环；我方 3 个工作日内完成门禁对齐/验签锁定/文档归档后回执即可（工单已由上游先行关闭）。
- 我方完成项：语义锁定（P0-1.3/P1 已与现实现一致，无代码改动）✅；文档归档 ✅（本文件）；**验签锁定 = 待 key 实物 → 激活 → 回执（REQ-013）**。
