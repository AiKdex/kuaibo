# 上游回复：AiKlog 配合事项清单 v1.0（2026-09-19）

- 回复方：AiKmap 主系统（上游 / 应用中心官方索引持有方）
- 对应文件：《上游配合事项清单-AiKlog致上游-v1.0.md》（AiKlog 方，2026-09-18）、《AiKlog联调跟进-20260919.md》
- 交付方式：本文件已存入联调测试空间「协作资料」目录；另附公开分享链接可转发存档

---

## A 类：需要上游提供能力或决策 —— 逐项交付

### A1. Entitlement / License 商业化协议 ✅ 已交付 v0.1（含联调料）

**协议草案**：`docs/Entitlement-License协议-v0.1草案.md`（GitHub 仓库 AiKmap/AiKmap.cn，与 AiKlog 仓库同构可直接参考）。

**Ed25519 验签公钥（iss=aikmap-license）**：
```
base64: 0aEys349vIDHxOc1D3naGulh3u9QFoWHHFOeyqCYO0Q=
PEM  : -----BEGIN PUBLIC KEY-----
       MCowBQYDK2VwAyEA0aEys349vIDHxOc1D3naGulh3u9QFoWHHFOeyqCYO0Q=
       -----END PUBLIC KEY-----
```

**测试 license key（全壳 pro，30 天，已生产环境验证）**：
```
eyJlZGl0aW9uIjoicHJvIiwiZXhwIjoxNzkyMzE4MzcyLCJpYXQiOjE3ODk3MjYzNzIsImlzcyI6ImFpa21hcC1saWNlbnNlIiwianRpIjoidGVzdC1hbGwtc2hlbGxzLTAwMSIsInNjb3BlcyI6WyJhbGwiXX0.95GCzxcHouUVGojwUPltWnXmO-Tum_sYTtq92SXfd9kH0Lcp3Jx-gECm7PhI1MgBDRPHuQGvcI9y0FUSGE14DQ
```
payload：`{edition:"pro", scopes:["all"], iss:"aikmap-license", jti:"test-all-shells-001", iat:2026-09-18, exp:2026-10-18}`；`scopes:["all"]` = 全壳（aikmap/aiklog/aikdex/aikbox）通用。

**Go SDK**：`server/internal/entitle/` —— `entitle.go`（Validate / GenerateKeyPair / Sign，公共 API：`Validate(key)`、`HasScope(scope)`、`HasFeature(name)`、`IsPro()`、`PublicKeyFromBase64`）、`entitle_test.go`（用例全绿）、`public_keys.go`（编译期内嵌公钥）。壳端可整体复用该目录或按草案自行实现。

**多壳互通（问题 2）**：v0.1 模型 = license 表达"授权事实"（edition + scopes），不绑定壳；`scopes:["all"]` 即跨壳激活。配额倍率由壳侧按 edition 映射套餐表（AiKlog 的 `ai_ask_*_quota` 接入点直接可用），上游只做验签与 tier 门禁。

**计费结算（问题 3）**：推荐口径 A —— 上游统一代收、按约定分成（v0.2 签发服务上线后落地）；当前联调期全部免费/测试 key，无结算动作。

**v0.1 边界（如实声明）**：无实例绑定（sub 弱绑定，留空跳过）、无吊销黑名单；测试 key 在 v0.2 引入绑定+黑名单后受限。正式 key 由签发服务（v0.2）签发。

### A2. AgentTools 运行时规格 ✅ 已有完整文档

- 协议：`docs/AgentTools协议.md`
- 开发规范：`docs/Agent工具开发规范-v1.md`（目录结构 / manifest 字段 / 权限申请 / 文件系统边界 / 执行沙箱）
- 能力插件 v2 草案：`docs/能力插件协议-v2草案.md`（kind=ui|capacity，不执行远程代码）——AiKlog 可直接按 v2 实现，v1 保留兼容
- 上游内核已注册工具：`tool_im_upload` / `tool_im_sendfile` / `tool_im_share` / `tool_im_asr` / `tool_web_clip`（/market/official/ 有包）
- 外部服务配额与鉴权（问题 3）：统一随 A1 Entitlement 管理 —— 市场条目 `quota` 字段声明配额，license edition/scopes 门禁执行；无独立鉴权通道

### A3. 测试货架与样例包 ✅ 已上架（9/17-9/18），含 paid 与坏包

索引地址：`https://aikmap.cn/market/index.json`（公开可拉，无需登录）

| 类型 | id | target | tier | 用途 |
|---|---|---|---|---|
| 插件 | com.aikmap.rss-feed | 空（全兼容） | free | 测试货架 beta |
| 插件 | aiklog-sidebar | [aiklog] | free | AiKlog 侧栏测试 |
| 插件 | bad-sha-plugin | 空 | free | **坏包负样本**（sha256 故意写错，装必报 MARKET_BAD_PACKAGE） |
| 插件 | pro-advanced | [aikmap] | **paid ¥99/年** | paid 门禁联调（停 key → 402；激活 → 200） |
| 主题 | com.aiklog.theme-paper | [aiklog] | free | AiKlog 专属测试主题 |

> ⚠️ AiKlog 若拉取显示 plugins/themes 为空：大概率是 5 分钟缓存或旧版本索引。请确认拉的是 `aikmap.cn/market/index.json` 且带缓存失效（或 ?t= 时间戳强制刷新）。当前索引 plugins=4、themes=1+、tools=5，实测 200。

---

## B 类：协议口径确认 —— 定案

### B1. 版本兼容字段 ✅ 以 `min_core_version` 为准

1. **字段名**：正式字段为 `min_core_version`（索引与博客插件表均此名）；`min_core/max_core` 未启用、不会更名。AiKlog 解析器已适配正确。
2. **语义**：语义化版本下限比较（semver 形态 "1.0.0"，字符串逐段比较；缺失视为 0 不拦截）。无"版本代号白名单"。
3. **schema_version 变更机制**：当前 "1"；协议变更遵循"只加字段不改旧字段语义"，且发版公告 + 兼容期（旧壳至少可解析忽略新字段）。

### B2. blog_market 三端点与公开门户移植 ✅ 授权

`blog_market.go`（GET/POST `/api/v1/blog/market[/install]` + 公开 `/market` 门户）与 AiKlog 仓库同构，**可直接移植**，无许可限制。
公开门户品牌化边界：允许各壳替换品牌三件套（站名 / logo / 页脚），AiKlog 面向用户呈现"爱库录 / AiKlog"，上游遵守该口径（引用 AiKlog 商品时不用 AiKmap 字样）。

### B3. 单壳上架冲突 ✅ 展示层降级 + 安装层拦截

- 规范维持：`target` 空 = 全兼容；写死单壳 = 该壳专用。
- 浏览不阻断：宿主侧对 target 不含本壳的商品标注"该商品不兼容本壳"（`applicable=false`），用户可见但可浏览详情。
- 安装才拦截：实际安装时报 422（MARKET_TARGET_MISMATCH）。
- 对历史已上架写死单壳的包：**不强制作者改 target**，按上述展示层降级即可；新上架包建议 target 空或如实声明。

---

## C 类：知会同步 —— 收悉并确认

1. AiKlog 已上线持续部署、默认市场源指向 aikmap.cn —— 确认，联调空间已互认。
2. 品牌铁律 —— 上游遵守；公开门户/市场卡片引用 AiKlog 时统一"爱库录 / AiKlog"口径。
3. 技术标识一致（Go module、storage key）—— 认可，利于未来并入零成本。
4. 主题 zip 免编译投放 + `ssr.css` —— 已记；上游后续推送主题类商品会附 `ssr.css`。

---

## 协作节奏确认

| 事项 | 上游侧状态 | 建议下一步 |
|---|---|---|
| A1 Entitlement 联调 | 公钥 + 测试 key + 草案已交付 | AiKlog 按 3 步接入（内嵌公钥 → Validate → 门禁查 IsPro），主系统 blog_market.go 为参考实现；本周内可跑通 paid 安装 |
| A3 测试货架 | 已上架 | AiKlog 装 rss-feed（全兼容）+ theme-paper（专属）验证安装链路；bad-sha-plugin 验证错误路径 |
| B1 字段口径 | 已定案 | 无需动作 |
| A2 AgentTools v2 | 草案已出 | 按 v2 实现，v1 保留兼容 |

> 上游侧对接人：admin（aikmap.cn 主系统）。后续联调事项继续在联调测试空间「协作资料」目录往返；本回复文档已同步一份至该目录。
