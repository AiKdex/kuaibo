# AiKlog 需求清单 v3.0 · 能力应用（capacity）与采集商品化

> 发起方：AiKlog（爱库录）｜收件方：上游（AiKmap 主系统）
> 日期：2026-09-19
> 前置：AiKlog 已收《阻塞项解除包》并完成支付后端（A-G B 项）移植（commit `156e2d6`）。
> 本清单与《AiKlog需求总清单 v2.0（全量）》互补：v2.0 覆盖底座对接与既有缺口，本清单聚焦**新增产品方向**——把「采集」做成应用中心可安装、可售卖的**能力应用（capacity）**与**源包商品**。

---

## 0. 缘起

产品侧确认了一条结论：**采集是博客站的刚需**（帮站长补充内容，直接减少写作负担），且「采集引擎 + 采集源包」构成应用中心的商业卖点。

我方研读上游《能力插件协议 v2 草案》与《事件总线契约》后确认：**这件事在协议层已经铺好路**，不需要新造机制。本清单的作用是把我方想做的形态**对齐到上游协议**，并把协议里「已定义但未落地」和「尚未定义但落地必需」的缺口逐条列出，请上游一次补齐。

---

## 1. 我方已确认的技术结论（请上游核对）

### 1.1 三分类（关键：源包不是 AgentTools）

我方结论是**三层分离**，请上游确认是否与主系统一致：

| 层 | 内容 | 协议映射 | 分发方式 |
|---|---|---|---|
| 引擎 | 抓取/去重/入库能力本身 | `kind=capacity` + `capabilities:["collector"]` + `backend_entry` | 内核能力，L1 编译期内置 + **能力开关** |
| 源包 | 源模板 / 频道模板 / 解读模板（纯配置，无代码） | 协议 §6「配置包」 | L3 市场 zip（不允许任意代码执行） |
| AI 面 | 探路建源、网页剪藏等可被 AI 调用的工具 | `PluginRuntime.RegisterTools(reg *ai.Registry)` | 既有 AI 工具通道 |

也就是说：**「采集源包」的主体是配置包，不是 AgentTools**；只有其中「AI 主动调用」的部分（如 probe 探路、web_clip 剪藏）才落 `ai.Registry`。这个区分直接影响商品的形态、审核强度与计费方式，因此列为第一条请上游核对项。

### 1.2 引擎为什么必须内置

- Go 无法安全地在运行时加载任意代码（协议 §3 亦明确 L3「不允许任意代码执行」）；
- 协议 §6 的设计本意就是「**内核持有 collector，插件不重造**，差异化只在 UI 胶水 + 源模板 + 解读模板」；
- 故 AiKlog 的落点 = **CoreModules 全量编译期内置 + 由应用中心「安装/启用」开闸**。对站长体验等价：在应用中心点安装 → 配置 → 可用。

---

## 2. 需上游配合事项（R1–R11）

### R1 · v2 校验层最小同步清单【P0 · 阻塞开工】

上游已于提交 `0afb111` 实现 `validatePluginV2` + `blog_plugins` 六列迁移，但 AiKlog（老 fork）**完全没有这一层**。我方实测：

- `grep validatePluginV2 | capabilities | backend_entry | min_schema` 在 AiKlog `internal/` 全域**零命中**；
- `blog_plugins` 现有列仅 `min_core_version` / `settings_schema` / `kind`，**缺** capabilities / backend_entry / routes / hooks / min_schema / max_schema。

**请给**：(a) `validatePluginV2` 所在文件 + blog_plugins v2 六列迁移语句的**最小同步清单**；或 (b) 直接给源码包（同上次交付口径）。
没有这层，capacity 应用在 AiKlog 无法被识别与校验。

### R2 · 能力开关（capability gate）的宿主契约【P0 · 阻塞开工】

协议 §6 写明「内核只负责：签发令牌、**能力开关**」，但**未定义开关的形态**。

**请给**：开关落点（config 键 / DB 表 / 二者）、命名规范、以及「安装 capacity 应用 → 点亮对应 CoreModules 字段」的标准动作。

我方拟实现（供参考，若主系统另有设计请直接给口径，避免单壳差异化造成后续合并冲突）：
安装时写一条 capability 记录 → `main` 装配阶段按记录决定是否构造对应 Store 并注入 `CoreModules` 字段；停用时字段传 `nil`，路由不注册 → 404（符合现有门控语义）。

### R3 · 配置包（源模板 / 频道模板 / 解读模板）格式规范【P0 · 阻塞商品化】

请给完整 schema：字段定义、校验规则、版本字段、是否复用 `pluginManifest`、**落地位置**（写 `sources` 表还是落 `data/` 目录）、单包能否包含多个源模板、是否需要合规声明字段（目标站点 / 用途）。

参照主系统 sources 表的实战字段（`template.list.urls` / `direct_items` / `kind` / `target_dir` / `CityResolver` 等），请确认我方理解——「**源包 = 一个可导入的 sources 模板集合**」——是否正确。

### R4 · 平台令牌（service token）签发契约

连接器模式靠 service token 调用内核已有 API。**请给**：签发端点、scopes 枚举、`authmw` 白名单接入方式、有效期与轮换策略，以及一个最小对接样例。

### R5 · collect API 面 + `collector.finished` 配套

《事件总线契约》已有 `collector.finished {run_id, status, created, failed}`，但连接器要**驱动**采集还需要 API 面。**请给**：触发 run / 查询状态 / 获取结果的端点与参数。

### R6 · 事件契约 v1 落地（订阅白名单 + publish 权限）

协议 §8 排期第 2 项。AiKlog 有 `bus.Bus`，但**无 topic 白名单校验器**。请给 v1 实现或最小同步清单。

### R7 · 采集商品的计费与授权口径

协议 §6 写明「SaaS 计费在 SaaS 侧解耦」，但 AiKlog 是 **freemium 直营**（不走 SaaS 分租）。**请明确**：采集能力开关是否纳入 Entitlement（scopes 是否扩 `capability:*`）；源包作为商品走哪条结算（买断 / 订阅 / 源包持续更新订阅）；是否落在结算 v0.3 范围。

### R8 · 采集类上架审核与合规边界【重要 · 涉及代收分成责任】

采集会外网抓取并可能转载他人内容；**平台代收分成，意味着平台承担部分审核责任**。请明确：严审层审核项（SSRF 白名单、抓取频率上限、robots 遵从、版权/来源声明字段），以及**责任边界**（内容合规由源包作者承担还是平台承担）。

### R9 · L2 / L2.5 第三方采集器的部署形态

协议 §3 说明本期不启用 Go plugin，以「外部服务 + 内核 API」（L2.5）替代。**请给**具体形态：外部服务如何启动、如何注册、如何鉴权——以便支持**第三方采集引擎**上架，而非只有官方引擎一种供给。

### R10 · 官方源包供给与分成

请明确上游是否提供「官方源包」（例如 qiuzhi 招聘/培训源沉淀的经验：一源多城、kind 分流、探路建源），以及壳侧代销的**分成口径**。

### R11 · capacity 应用的 UI 承载方式（姊妹项）

协议 §7 明确 `frontend_entry` 白名单不变（未命中内置清单 → `422 PLUGIN_COMPONENT_NOT_REGISTERED`）。但 capability 应用通常需要管理界面（配源、查看 run 记录）；若每个都要求编译期内置组件，**第三方就无法自助上架**。

**请确认**：是否由内核提供**通用能力模块页壳**（按 `capabilities` 渲染标准表单）。我方愿意实现并回贡上游，但需先对齐口径，避免出现两套实现。

---

## 3. 我方自持事项（不需上游）

- CoreModules 全量编译期内置（Sources / Collector / WebDAV / Digest / Inbox / Review / Org；`files.inbox_state` 已预铺）
- 应用中心 capacity 卡片 UI（安装 / 启用 / 停用 / 配置）
- 通用「能力模块」页面壳（对应 R11，待口径确认后落地）
- 采集管理页（源列表 / 探路建源 / 手动触发 run）
- 源包安装后写入 sources 表

---

## 4. 交付节奏建议

| 优先级 | 事项 | 说明 |
|---|---|---|
| **P0** | R1、R2、R3 | 给了这三项，AiKlog 即可把 capacity 应用链路端到端跑通 |
| **P1** | R4、R5、R6、R11 | 连接器闭环 + 通用页壳 |
| **P2** | R7、R8、R9、R10 | 商业化与生态（源包市场、分成、第三方引擎） |

---

## 5. 附：AiKlog 侧实测依据（供上游核对）

1. `handler/apps_market.go:70` 注释已写「上游 v2 语义兼容：市场包 manifest kind 可能是 **ui / capacity**」——说明本 fork 早期已按 v2 预留判据，但当前仅用于「主题 / 插件」归一，**未实现 capacity 分支**。
2. `handler.CoreModules`（上游 `routes.go:47`）注释：字段可为 nil，对应路由不注册（404 = 能力未启用），不会因缺模块 panic；`New` 注释称 `collector` 为「**能力插件 seam**」——与本清单第 1 节结论一致。
3. AiKlog `blog_plugins` 现有列：`min_core_version` / `settings_schema` / `kind`（**缺** v2 六列）。
4. AiKlog 已具备的前置能力：`bus.Bus`（无 topic 白名单）、`ai.Registry`（已注册 7 个工具，含 `web_clip`）、`impex.CreateURLImport`、`ssrf.go` 同类防护思路可参照采集模块自带实现。
