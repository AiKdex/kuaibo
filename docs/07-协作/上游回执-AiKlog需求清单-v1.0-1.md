# 上游回执：AiKlog 协作事项清单 v1.0

- 版本：v1.0（2026-09-18）
- 回应方：AiKmap 主系统（上游/应用中心）
- 对应文件：《上游配合事项清单-AiKlog致上游-v1.0.md》（AiKlog 方，2026-09-18 提交协作区）
- 状态：逐项已回执；A3 已完成上架；**A1 协议 v0.1 已交付（草案 + Ed25519 验签实现 + paid 门禁联调全绿）**

---

## A 类：需要上游提供能力或决策

### A1. Entitlement / License 商业化协议（优先级最高）—— ✅ 已交付（v0.1，2026-09-18）

**现状确认**：上游索引协议已定义 `tier / price / billing_period / free_until / trial_days / quota` 字段（blog_market.go：marketTool/marketPlugin），但服务端确未实现 license 签发/验签（license.go 为占位版，edition 恒 free）。**此项缺口属实，已补齐。**

**交付内容（v0.1）**：
1. **协议草案**：`docs/Entitlement-License协议-v0.1草案.md`（已入仓库）——key 格式、scopes[] 多壳授权模型、验签流程、计费归属决策点、演进路线。
2. **License key 格式**：`<base64url(payload)>.<base64url(signature)>`；payload = `{iss, sub, scopes[], edition, iat, exp, jti, tid}`，签名 **Ed25519**（签发私钥仅上游持有，公钥编译期内嵌各壳二进制，本地离线验签）。
3. **多壳互通**：同一 license 按 `scopes[]` 声明——`shell:<id>`（壳授权）/ `feature:<name>`（功能授权）/ `all`（全生态）；含本壳即生效。
4. **计费结算归属**：决策点已列两案（A 上游统一代收按约定分成——推荐；B 各壳自签发），v0.1 先统一验证能力，收款渠道就绪后落地，不影响协议本身。
5. **SDK**：Go 包 `server/internal/entitle`（Validate/Sign/GenerateKeyPair + HasScope/HasFeature/IsPro 门禁 API），壳端直接复用（internal 限制可经主仓库引用或拆独立 module）。
6. **paid 门禁联调（生产实测全绿）**：`tier=paid` 商品 `pro-advanced` 已上架——停用 license → 安装 `402 MARKET_LICENSE_REQUIRED`；激活 license → 安装 `200` 成功；卸载正常。

**后续**：v0.2 签发服务（主系统 license 端点/CLI）+ 实例强绑定 + 吊销黑名单；v0.3 计费订单/分成。

### A2. AgentTools 运行时规格 —— 规格已有，按 v1 先行

**现状确认**：AgentTools 运行时规格**已存在**：`AgentTools协议.md`、`AgentTools开发规范.md`、`Agent工具开发规范-v1.md`（目录结构 / manifest 字段 / 宿主钩子 / 权限申请 / 文件系统访问边界 / 沙箱）。官方 5 个工具（tool_im_upload / sendfile / share / asr / web_clip）已上架且 `target` 含全部四壳。

**逐项回应**：
1. 完整运行时规格见上述文档；宿主侧需实现的钩子清单（权限申请、文件系统边界、执行沙箱）已定义，如有出入以《AgentTools开发规范.md》为准。
2. **能力插件 v2（kind=ui|capacity）**：草案方向已定，但建议 AiKlog **按 v1 先行**（v1 工具协议已可跑通、跨壳 target 兼容），v2 作为升级路径同步演进，不阻塞。
3. web_clip / asr 等外部服务依赖：**配额与鉴权归属上游统一管理**（收费工具走 A1 Entitlement）；web_clip 已 `reg.Register(NewWebClipTool(...))` 注册进内核，asr 走本地 faster-whisper（隐私不出服务器）。

### A3. 测试货架与样例包 —— 已完成 ✅（2026-09-18 上架）

`https://aikmap.cn/market/index.json` 已生效，新增：

| 类型 | 包 | target | 用途 |
|---|---|---|---|
| theme | `com.aiklog.theme-paper`（纸墨主题） | `["aiklog"]` | 测试主题（beta） |
| plugin | `com.aikmap.rss-feed`（博客 RSS 输出） | 空（全兼容） | 测试插件（beta） |
| plugin | `aiklog-sidebar`（日志侧栏） | `["aiklog"]` | 测试插件（beta） |
| plugin | `bad-sha-plugin`（坏校验） | 空 | 负样本：sha256 故意错写，安装必报 `MARKET_BAD_PACKAGE` |

- `tier=paid` 测试商品：**待 A1 协议定稿后上架**（避免无 license 时 402 干扰联调）。
- AiKlog 拉索引后可先装 theme-paper / rss-feed 验证安装链路；用 bad-sha-plugin 验证错误路径。

---

## B 类：请上游确认协议口径

### B1. 版本兼容字段 —— 确认：字段为 `min_core_version`（无 min_core/max_core）

1. **字段名**：上游代码、`/market/index.json` 实际索引、全部规范文档（应用中心条目字段与上架规范 / AgentTools开发规范 / 应用中心壳端对接规范）统一为 **`min_core_version`**。`min_core / max_core` 非本协议字段（可能源于早期文档版本），请 AiKlog 解析器以 `min_core_version` 为准。
2. **语义**：**semver 下限比较**（`versionAtLeast(core, min)`，即 `core >= min` 才可装）；仅下限门槛，非白名单匹配。
3. **schema_version 变更机制**：协议变更遵循「新字段可选、旧字段不删」的向后兼容承诺；`schema_version` 递增时在发布说明中预告；建议壳端对未知字段宽容解析（忽略即可）。

### B2. blog 市场三端点与公开门户移植授权 —— 授权 ✅

- **授权 AiKlog 直接移植** `blog_market.go` 三端点（站长博客侧市场 `/api/v1/blog/market` + 公开 `/market` 门户），同构仓库口径。
- **公开门户品牌化**：允许各壳替换品牌三件套；**AiKlog 面向用户不得出现 AiKmap/Knowledge Map 字样 —— 确认遵守**。公开门户默认品牌文案为可配置项（壳端可设 `shell_brand`），上游侧引用各壳时也遵循各壳品牌口径。

### B3. 多壳规范 vs 单壳上架冲突 —— 口径确认

- `target` 空 = 全兼容、写死单壳 = 仅该壳可装：**确认**（规范原文）。
- 历史已上架的写死单壳包：**不强制作者改 target**；宿主侧在展示层做「该商品不兼容本壳」降级提示即可，不对整个市场 422。

---

## C 类：知会同步 —— 全部收悉，无需上游动作

1. AiKlog 已上线并持续部署（aiklog.cn，SPA `/app#/`、SSR `/blog`；默认市场源已指向 `aikmap.cn/market/index.json`）— 收悉。
2. AiKlog 品牌铁律（爱库录 / AiKlog）— 收悉并遵守（见 B2）。
3. 技术标识不改（Go module / storage key）— 认可，利于未来并入零成本。
4. AiKlog 主题声明式 zip（免编译投放，`data/themes/<id>` 白名单 + SSR 模板运行时覆盖）— 收悉；**上游推送主题类商品时附 `ssr.css`** 以覆盖机器轨，已记入上架规范。

---

## 协作节奏对齐

| 事项 | 我方承诺 | 状态 |
|---|---|---|
| A1 Entitlement 协议 | v0.1 草案 + Ed25519 验签实现 + paid 门禁联调 | ✅ 已完成（2026-09-18） |
| A3 测试货架 | 已上架（theme/plugin/坏包 + paid 商品 pro-advanced） | ✅ 已完成 |
| B1 版本字段口径 | 已书面确认（`min_core_version`） | ✅ 已完成 |
| B2 移植授权 | 已授权 + 品牌边界确认 | ✅ 已完成 |
| B3 单壳降级口径 | 已确认（展示层降级，不 422） | ✅ 已完成 |
| A2 AgentTools 规格 | 文档已存在，按 v1 先行 | ✅ 规格已就绪 |

> 联系方式：本回执同文提交 aikmap.cn 协作区；AiKlog 侧可继续在协作区评论/订阅本文件获取更新。
