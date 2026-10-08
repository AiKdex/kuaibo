# AiKlog ⇄ 上游（AiKmap 应用中心）协作事项清单

- 版本：v1.0（2026-09-18）
- 发起方：AiKlog（爱库录，aiklog.cn）
- 对接对象：AiKmap 应用中心（独立化后的官方市场，`https://aikmap.cn/market/index.json`）
- 适用范围：应用中心协议、市场货架、商业化（Entitlement/License）、Agent 工具运行时

## 背景

AiKlog 已完成应用中心主链路对接：默认市场源已切换为 `https://aikmap.cn/market/index.json`（实测 200 可达），支持索引拉取（`GET /admin/apps/market`）、在线安装（zip-slip 防护 + sha256 + manifest + 多壳 `target[]` 匹配）与本地直装。三类同构索引 `plugins[] / themes[] / tools[]` 均可解析。

当前上游索引实际内容：`plugins: []`、`themes: []`、`tools: [tool_im_upload]`。以下事项按「需上游动作」程度分为 A（必须上游提供/决策）、B（请上游确认协议口径）、C（知会同步）三档。

---

## A 类：需要上游提供能力或决策

### A1. Entitlement / License 商业化协议（优先级最高）

**现状**：AiKlog 已移植 `license.go` 占位版（edition 恒 `free/community`），导致上游市场 `tier=paid` 的商品一律 402 拒装。AI 问答配额已按「游客/用户/管理员」三档分层实现，并预留了套餐倍率接入点（`ai_ask_*_quota` 设置组）。

**需要上游**：
1. 提供 Entitlement 校验协议：license key 的签发格式、验签方式（本地公钥验签 or 服务端校验端点）、续期/吊销机制
2. 明确多壳（aikmap / aiklog / aikbox / aikdex）下 Entitlement 是否互通：同一 license 能否跨壳激活、配额倍率如何随壳/套餐生效
3. 付费商品的计费与结算归属口径（上游统一代收 → 分成，还是各壳自收）
4. 若有现成 SDK/Go 包，提供引用方式

**没有这一项，上游市场的付费货架对 AiKlog 用户形同虚设。**

### A2. AgentTools 运行时规格（tools[] 透出的前置）

**现状**：AiKlog 无 Agent 工具运行时，索引中 `tools[]` 不透出（当前上游货架也仅有 `tool_im_upload` 一个工具）。AiKbox 主打网盘/IM/语音视觉技能，AgentTools（upload/sendfile/share/asr/web_clip）协议 v1 已定义。

**需要上游**：
1. AgentTools v1 的完整运行时规格：工具包的目录结构、manifest 字段、宿主侧需要实现的钩子（权限申请、文件系统访问边界、执行沙箱）
2. 明确「能力插件 v2（kind=ui|capacity，不执行远程代码）」草案的时间表，AiKlog 是否可以直接按 v2 实现、跳过 v1
3. web_clip / asr 等依赖外部服务的工具，其服务端配额与鉴权归属

### A3. 测试货架与样例包（联调刚需）

**现状**：上游索引当前 plugins/themes 均为空，AiKlog 对接侧缺少可验证的真实样本，只能靠自造 zip 自测。

**需要上游**：
1. 在官方货架上架 2–3 个 `target` 含 `aiklog`（或留空全兼容）的**测试用**主题/插件包（可标注 beta）
2. 提供 1 个 `tier=paid` 的测试商品（配合 A1 联调 Entitlement）
3. 提供 1 个故意写错 sha256 / manifest 的坏包样本，用于双向校验错误路径

---

## B 类：请上游确认协议口径

### B1. 版本兼容字段的最终口径

**现状**：上游索引用 `min_core / max_core`，AiKlog 解析器当前读的是 `min_core_version`，版本闸门形同虚设。AiKlog 会补齐适配，但需要上游确认：

1. 字段名以哪个为准（`min_core/max_core`？后续会否更名？）
2. 语义：是 semver 区间比较，还是「核心版本代号」白名单匹配？
3. `schema_version` 变更（当前 "1"）的通知机制：上游改协议时是否有预告期/兼容期承诺

### B2. blog 市场三端点与公开门户的移植授权

**现状**：上游 `blog_market.go` 的三端点（站长博客侧市场 `/api/v1/blog/market` + 公开 `/market` 门户）未移植到 AiKlog。

**需要上游**：确认这两块代码可否直接移植（同构仓库口径），以及公开门户的多壳品牌化边界——门户页面是否允许各壳替换品牌三件套（AiKlog 面向用户不得出现 AiKmap/Knowledge Map 字样，属 AiKlog 品牌铁律）。

### B3. 多壳规范 v.s. 单壳上架的冲突处理

**现状**：多壳规范（三壳知会版）约定 `target` 空即全兼容、写死单壳安装 422。请上游确认：对历史已上架的写死单壳的包，是要求作者改 target，还是宿主侧在展示层做「该商品不兼容本壳」的降级提示即可。

---

## C 类：知会同步（无需上游动作）

1. **AiKlog 已上线并持续部署**：aiklog.cn，SPA 走 `/app#/`，SSR 博客走 `/blog`；默认市场源已指向 `aikmap.cn/market/index.json`
2. **AiKlog 品牌铁律**：所有面向用户的 UI/SEO/导出物以「爱库录 / AiKlog」呈现；上游在公开门户/市场卡片引用 AiKlog 壳时请遵循同一口径
3. **技术标识不改**：Go module `github.com/AiKMAP/AiKmap/...`、storage key `aikmap_*` 等保持与上游一致，未来并入零成本
4. **AiKlog 自有交付链路**：主题声明式 zip（免编译投放）已支持，`data/themes/<id>` 白名单 + 运行时覆盖 SSR 模板；上游若推送主题类商品建议附 `ssr.css` 以覆盖机器轨

---

## 建议的协作节奏

| 事项 | 建议时限 |
|---|---|
| A1 Entitlement 协议 | 双方各 1 周内出接口草案，对齐后联调 |
| A3 测试货架 | 上游上架后 AiKlog 一周内完成安装链路联调 |
| B1 版本字段口径 | 书面确认即可，AiKlog 当周适配 |
| A2 AgentTools 规格 | 可后置至能力插件 v2 定稿 |

> 联系方式：AiKlog 侧对接人可通过 aikmap.cn 协作空间联系（本清单同文提交协作区），或邮件沟通。
