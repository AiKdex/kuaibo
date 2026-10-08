# AiKlog 自部署多站点（站点数授权）架构与实施方案

> 编号：**SPEC-MS-001** · 版本：**v1.1.0** · 2026-09-20 · 状态：**评审定稿（两处实现细节已定）**
> 适用：AiKlog 自部署发行版（`server/` + `web/`），GitHub `AiKlog`（main）为权威源。
> 读者：架构评审、后端开发、发行版打包方。
> 品牌口径：对外一律「爱库录 / AiKlog」；技术标识（Go module、`aikmap_*` storage key、响应头 `X-AiKmap-File-Id`）维持不变。

> **修订记录**
> | 版本 | 日期 | 变更 |
> |---|---|---|
> | **v1.0.0** | 2026-09-20 | 首版草案：定位、目标架构、数据模型、路由、授权 cap、底座复用、实施分期。 |
> | **v1.1.0** | 2026-09-20 | 定稿两处实现细节：①per-site 配置采用**新增 `site_settings` 结构化列表**（非 `settings` 加 scope）；②SPA 主题 per-site **复用现有双轨 + 访客 localStorage 切换**，仅接 `default_theme` / `allow_visitor_theme_switch`，无需编译层。补充三种访问方式（自定义域名/子域名/子目录）的路由解析优先级与子目录模式 nginx 层注意点；`sites` 表收敛为身份路由表，`site_settings` 承载标题/默认主题/访客切换开关/SEO 等。 |

---

## 0. 结论（先看这里）

**一句话**：在现有单实例 AiKlog 之上加一层 `site` 维度（数据隔离 + Host/Path 路由），并把「站点数上限」做成一个受 `license` 约束的能力，像主题/插件一样走应用中心付费分发——免费自部署版 cap=1，付费解锁 3 / 10 / 累加 / 订阅档。

**三点判断**：
1. **放弃「第三方在我系统里建站」的多租户 SaaS**（合规雷区：内容审核责任、备案实名、第三方数据隔离），本方案**不涉任何客户建站流程、客户计费、客户配额**。
2. **只做「自己运营的多站点」**：一套自部署实例、自己开 N 个独立域名的内容站（矩阵站群），内容/主题/配置各自独立，全部在自己管控内。
3. **多站点 = 一个受 cap 限制的能力模块**，不是平台。授权售卖与主题/插件/agent tools/skill/工作流**共用同一条应用中心付费分发管线**，不新建计费体系。

**两个实现细节已于 v1.1.0 定稿**（详见 §3.2、§4、§9）：
- **per-site 配置** → 新增 `site_settings` 结构化列表（后台表单直接映射，站长零代码），不复用全局 `settings` 表的 `scope`。
- **SPA 主题 per-site** → 复用现有双轨主题 + 已存在的访客 localStorage 切换，只新增 `default_theme`（站长设默认主题）与 `allow_visitor_theme_switch`（站长可关闭访客切换），**无编译层新增**。

---

## 1. 背景与定位

| 维度 | 当前（v 现状） | 目标（本方案） |
|---|---|---|
| 站点模型 | 单实例单站点，全局唯一博客 | 单实例多站点，每 site 独立域名/内容/主题/配置 |
| 商业模型 | 免费自部署，应用中心卖主题/插件 | 自部署 + **站点数授权**作为付费分发项 |
| 合规边界 | —— | **不做**第三方建站 SaaS，规避客户内容合规责任 |
| 多站点授权 | 无 | 1（免费）/ 3 / 10 / 累加 / 订阅，应用中心分发 |

**差异化判断（与外部对照）**：开源自部署博客/论坛（WordPress 单站、Typecho、Ghost、Discuz! X、Flarum）基本「一份部署=一个站」，多站=多部署；WordPress Multisite 虽原生多站但共享用户库、隔离弱、配置重；商业建站 SaaS（Wix/Squarespace/凡科）多租户但用户不拥有系统。本方案的「自部署所有权 + 真隔离多站 + 灵活站点数授权」恰填补这一空白区间。

---

## 2. 目标架构总览

```
                        ┌──────────────────────────────────────────────┐
   自定义域名 ───────►  │  接入层：Host/Path→site 解析中间件（注入 site_id）│
  site1.aiklog.cn       │  优先级：自定义域名 > 子域名 > 子目录 > 默认站     │
  site2.aiklog.cn       └──────────────────────────────────────────────┘
  aiklog.cn/site2                    │ 按 site_id 分流
   ┌──────────────────┬──────────────┴───────────────┬──────────────────┐
   ▼                  ▼                              ▼                  ▼
 [Site A]          [Site B]        ...          [默认站]            [平台底座]
 内容(files)        内容(files)                  内容(files)      ┌─────────────────┐
 分类(tags)         分类(tags)                   分类(tags)        │ 应用中心/市场    │
 站点设置           站点设置                     站点设置           │ 支付后端(HMAC)  │
  (site_settings)   (site_settings)              (site_settings)   │ CoreModules     │
 主题(default_theme)主题(default_theme)         主题(default_theme)│ R4 平台令牌     │
                                                      │            │ settings(instance)│
                                                      ▼            └─────────────────┘
                                              [授权层] site_licenses
                                              cap = 1 + Σ永久 + Σ订阅临时
```

- **隔离边界**：内容（files）、分类标签（tags）、站点级设置（site_settings）按 site 归属；用户（users）**平台共享**（跨站作者），不强制 site-scoped，降低复杂度。
- **主题**：沿用现有双轨主题机制（SSR `data/themes/<id>/` + SPA 运行时选主题，访客已可 localStorage 自行切换）。每 site 在 `site_settings.default_theme` 设自己的默认主题；`allow_visitor_theme_switch=0` 时强制使用默认主题、隐藏切换器。**不新建主题体系、无编译层新增**。
- **授权层独立于内容付费**：站点数授权走新建 `site_licenses` 表（见 §5），不复用 `access_grants`（后者绑定 `file_id`，语义为单篇内容付费）。

---

## 3. 数据模型改造

### 3.1 新建 `sites` 表（身份 / 路由表）

只承载「这个站是什么、怎么访问、归谁、状态」，不混配置。

```sql
CREATE TABLE IF NOT EXISTS sites (
    id            TEXT PRIMARY KEY,
    slug          TEXT NOT NULL UNIQUE,   -- 内部标识，亦用于 /s/{slug} 回退路由
    domain        TEXT NOT NULL DEFAULT '', -- 自定义域名（精确匹配）；空或 '*' = 默认站
    subdomain     TEXT NOT NULL DEFAULT '', -- 子域名前缀（配合平台 base domain，如 site2.aiklog.cn → 'site2'）
    path_prefix   TEXT NOT NULL DEFAULT '', -- 同域名子目录（如 /site2，运维层 rewrite 注入，见 §4.4）
    owner_id      TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active', -- active | suspended | deleted
    created_at    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sites_domain ON sites(domain);
CREATE INDEX IF NOT EXISTS idx_sites_subdomain ON sites(subdomain);
```

### 3.1.1 新建 `site_settings` 表（per-site 配置，结构化列）

**v1.1.0 定稿**：per-site 配置采用本表（结构化列），**不复用全局 `settings` 表的 `scope`**。理由：
1. 全局 `settings` 是 KV 表，加载器只认 `scope='instance'`（插 `global` 历史上有静默不生效的坑），塞 `site:<id>` 会串站且需改加载器；
2. 站长「站点设置」后台要渲染表单（默认主题下拉、访客切换开关、标题/SEO 输入），结构化列直接映射字段、好校验；
3. 与全局平台配置（多站点开关）职责分明，不污染全局。

```sql
CREATE TABLE IF NOT EXISTS site_settings (
    site_id                     TEXT PRIMARY KEY,            -- 关联 sites.id
    default_theme               TEXT NOT NULL DEFAULT 'default', -- 站长设的默认主题
    allow_visitor_theme_switch  INTEGER NOT NULL DEFAULT 1,   -- 1=允许访客自由切换；0=关，强制 default_theme
    title                       TEXT NOT NULL DEFAULT '',    -- 站点名称（区别于博客目录名）
    subtitle                    TEXT NOT NULL DEFAULT '',
    locale                      TEXT NOT NULL DEFAULT 'zh-CN',
    seo_title                   TEXT NOT NULL DEFAULT '',
    seo_description             TEXT NOT NULL DEFAULT '',
    seo_keywords                TEXT NOT NULL DEFAULT '',
    ext                         TEXT NOT NULL DEFAULT '{}'   -- 站点级兜底(JSON: logo/favicon/备案号/自定义CSS等)
);
```

> 访客切换现状（已核实）：`web/src/stores/theme.js` 的 `useThemeStore` 将访客选择写入 `localStorage` 并 `apply()`，访客各自独立。`allow_visitor_theme_switch=0` 时前端隐藏/禁用切换器、忽略访客 localStorage、统一用 `default_theme`。

### 3.2 现有表加 `site_id`（迁移策略）
- **`files`**：`ADD COLUMN site_id TEXT NOT NULL DEFAULT '<默认站id>'`。博客文章即 files 行，内容隔离的核心。
- **`tags`**：`ADD COLUMN site_id TEXT NOT NULL DEFAULT '<默认站id>'`。`ctx.tags`（带 count 的分类云）按站渲染，必须 per-site。
- **`settings`**：**不动其 scope 语义**。全局平台级配置（如「多站点是否启用」「`multisite.base_domain`」）保留在现有 `scope='instance'` 机制内；站点级配置全部走新 `site_settings` 表（见 §3.1.1）。——v1.1.0 已定，不再二选一。
- **`users`**：**不加** site_id，平台共享；站点归属通过内容行（files.site_id）体现，作者跨站可用。

### 3.3 默认站约定（升级兼容）
- 实例首次启动（或迁移时）自动建一条 `sites` 记录作为**默认站**（domain='*' 或空，subdomain/path_prefix 均空），现有全部 files/tags 的 `site_id` 指向它；同步建一条 `site_settings`（默认主题沿用现状 `blog.theme`、允许访客切换=1）。
- **单站点（未购授权）场景**：只存在默认站，路由/数据完全等价于现状，零行为变化。
- 升级脚本：迁移中 `UPDATE files SET site_id='<默认站id>' WHERE site_id=''` 等，幂等。

---

## 4. 域名与路由改造

### 4.1 Host/Path→site 解析中间件
- 在 `mux` 最外层（鉴权之前）新增中间件：综合 `Host` 头 + 请求 `Path`（及 §4.4 的 nginx 注入头）解析 site，注入 `context.Context`。
- 解析优先级（从高到低）：**自定义域名精确匹配 → 子域名 → 子目录 → 默认站**。
  1. `domain` 精确匹配 `Host`（非空且 != '*'）→ 命中。
  2. 取 `Host` 第一段前缀（去掉全局配置 `multisite.base_domain`，如 `aiklog.cn`），与 `subdomain` 匹配 → 命中。
  3. 请求 `Path` 或注入头 `X-Aiklog-Site-Path` 以 `/{path_prefix}` 开头 → 命中。
  4. 均未命中 → 默认站（fallback）。
- 全站数据访问从 ctx 取 `site_id` 作为查询条件（repo 层统一加 `WHERE site_id=?`）。

### 4.2 路由适配
| 入口 | 改造 |
|---|---|
| SSR `/blog`、`/{slug}` | 按 ctx.site_id 拉该站内容/主题/SEO 渲染；支持 `Path` 前缀（子目录模式） |
| SPA `/app#/blog` | 前端已支持运行时选主题 + 访客 localStorage 切换；API 由后端按 Host/Path 自动识别 site，前端无需改路由；`allow_visitor_theme_switch=0` 时隐藏切换器 |
| 管理后台 `/admin` | 超管加「站点切换器」；单站用户只看本站；开通/停用走 §7 |

### 4.3 未匹配域名
- 配置项 `multisite.fallback`（`scope='instance'`）：`default`（走默认站）或 `404`。默认 `default`，保证现状兼容。

### 4.4 子目录（path_prefix）模式注意点（运维层）
- SPA 是 hash 路由（`/app#/blog`），**纯应用层按 Path 前缀区分站对 SPA 不友好**（hash 在浏览器侧，path 仅 `/app`）。推荐运维层（nginx）把 `/site2/app` 反代到 `/app` 并注入请求头 `X-Aiklog-Site-Path: /site2`，应用层优先读该头解析 site，避免 hash 冲突。
- SSR（`/blog`、`/{slug}`）天然支持 Path 前缀：nginx 把 `/site2/blog` rewrite 到 `/blog` 并注入同款头即可。
- 也支持「纯 SSR 子目录」（不挂 SPA），最省事。
- 子目录方式与自定义域名/子域名**可并存**（不同站用不同方式），解析按 §4.1 优先级互不干扰。

---

## 5. 站点数授权模型（cap）

### 5.1 计算规则
```
cap = 1（基础免费） + Σ(生效中的 permanent.seats) + Σ(生效中的 subscription.seats)
```
- `permanent`：永久额度（档位包 / 累加购买）。
- `subscription`：临时额度（订阅期内有效，过期回落）。
- 已建站数 `used = COUNT(sites WHERE status!='deleted')`；新建第 `used+1` 个站时若 `used >= cap` → 拒绝（返回 `402` + 引导购买）。

### 5.2 新建 `site_licenses` 表
```sql
CREATE TABLE IF NOT EXISTS site_licenses (
    id           TEXT PRIMARY KEY,
    edition      TEXT NOT NULL,       -- 'tier3' | 'tier10' | 'addon1' | 'sub_year' ...
    seats        INTEGER NOT NULL,    -- 该 license 提供的站点数
    license_type TEXT NOT NULL,       -- 'permanent' | 'subscription'
    expires_at   INTEGER NOT NULL DEFAULT 0, -- subscription 用；permanent=0
    status       TEXT NOT NULL DEFAULT 'active',
    granted_at   INTEGER NOT NULL DEFAULT 0
);
```
> 不复用 `access_grants`（其 `file_id`/`grant_token` 语义为单篇内容付费），站点数授权是「能力/license」类，独立成表更清晰，且可并行服务于 CoreModules 门控。

### 5.3 三种购买形态并存（对应不同需求）
| 形态 | edition | license_type | 说明 |
|---|---|---|---|
| 档位包（一次性 5 站） | `tier5` 等 | permanent | 买断，cap += 5 |
| 订阅（一年尝试） | `sub_year` | subscription | 一年内 cap += N，过期回落基础档 |
| 累加（买 1 站试试） | `addon1` | permanent | cap += 1，可多次叠加 |

三者可叠加（例如已有 tier3 + 再买 addon1 → cap=4），cap 公式天然支持。

### 5.4 闸门与回落
- 新建站点、`/admin` 站点管理操作受 cap 约束。
- 订阅过期：临时额度移除，`used` 若已超过新 cap，**已建站保留但禁止新建/提示续费**（不强制删站，避免数据损失）。

---

## 6. 复用现有底座（不新建体系）

| 现有能力 | 文件/机制 | 本方案复用方式 |
|---|---|---|
| 应用中心分发 | `apps_market.go`（`install`/`market/install` 端点，支持 theme/plugin/source-pack 三类） | 新增 `kind=license` 商品，复用上架/付费/分发管线；install 端点增加 license 类型处理（签发 `site_licenses`） |
| 支付后端 | `pay/notify`（HMAC 验签，已在 authmw 匿名白名单）+ 402 闸门 | 复用现有 HMAC 验签与 402 闸门，**不新建支付**；notify 成功后幂等签发 `site_licenses` |
| 能力门控 | `cmd/aikmap/capabilities.go` 的 `buildCoreModules`（现有 7 模块门控） | 多站点注册为第 8 个 CoreModule（`Multisite`）；免费版表现为 cap 受限，付费解锁提升 cap |
| 平台令牌 | R4 平台令牌 + 白名单 | 授权包分发/验证复用现有令牌体系，不另起 |
| 双轨主题 | SPA 运行时选主题（访客 localStorage 切换，已存在）+ SSR `data/themes/<id>/` | 每 site 设 `default_theme`；`allow_visitor_theme_switch` 控制访客切换，零新增编译层 |

---

## 7. 站点管理后台

- **平台超管视图**：站点列表（域名/slug/子目录/状态/已用配额）、开通、停用（`suspended`）、软删（`deleted`）、查看每站 `site_settings`。
- **站点级视图**：复用现有 `/admin` 后台，限定 ctx.site_id 上下文（只能管本站内容/主题/设置）；「站点设置」表单直接映射 `site_settings` 结构化列（默认主题下拉、访客切换开关、标题/SEO 输入）。
- **开通流程**：后台「新建站点」→ 填域名/slug/子域名/子目录 + 选默认主题 → 落 `sites` 行 + 初始化 `site_settings`（继承默认站 or 平台模板）→ 配置 DNS/证书/reverse-proxy（运维侧，平台提供指引）。

---

## 8. 实施分期（里程碑）

| 阶段 | 内容 | 关键文件 |
|---|---|---|
| **M0 数据层** | `sites` 表 + `site_settings` 表 + `files`/`tags` 加 `site_id` + 默认站升级迁移 | `repo/db.go` |
| **M1 路由层** | Host/Path→site 中间件 + ctx.site_id 注入 + repo 层 `WHERE site_id` + SSR/SPA 渲染适配 + `default_theme`/`allow_visitor_theme_switch` 接线 | `routes.go`、`handler/*`、`web/src/stores/theme.js` |
| **M2 授权层** | `site_licenses` 表 + cap 计算 + 402 闸门 + `pay/notify` 签发 | `repo/db.go`、`pay handler`、`capabilities.go` |
| **M3 后台** | 站点管理 + 开通流程 + 站点切换器 + 站点设置表单 | `handler/admin*.go`、`web` |
| **M4 分发** | 应用中心 `kind=license` 商品上架 + 三种形态配置 + install 端点扩展 | `apps_market.go` |

> 依赖顺序：M0→M1（无数据隔离，路由无意义）→M2（授权依赖 site 存在）→M3→M4。M0 最硬，先做且必须幂等兼容单站现状。

---

## 9. 风险与遗留（简述，非阻塞）

- **数据迁移**：现有单站 files/tags 必须归属默认站，迁移脚本幂等、可回滚。
- **SPA 主题 per-site（v1.1.0 已定，非阻塞）**：双轨主题 + 访客 localStorage 切换已存在，per-site 仅接 `default_theme` 与 `allow_visitor_theme_switch`，**无需编译层新增**。唯一注意：SPA 主题仍是「编译后随发行版上线」机制（主题规范附录 F.3），per-site 只是「选哪个已上线主题」，不涉及编译差异。
- **SEO/搜索引擎**：多域名需 per-site sitemap 与 canonical，避免重复内容。
- **合规**：本方案不涉及第三方建站，无客户内容审核/备案责任；自运营多站的内容合规由运营方自行负责（与单站同责）。
- **备份/恢复**：多站共享一个库，备份粒度需支持按 site 导出（增量项，非阻塞）。

---

## 10. 验收口径

1. 单一部署起 ≥2 个 site（含自定义域名 + 子目录两种方式），内容/分类/站点设置互不可见（隔离通过）。
2. 默认 cap=1，购买 `tier3` license 后 cap=4，尝试建第 5 站被 `402` 拦截。
3. `sub_year` 订阅期内 cap 提升，模拟过期后临时额度回落、已建站保留但禁新建。
4. 升级脚本在现有单站数据上幂等执行，`used` 计数为 1 且全部归属默认站，行为零变化。
5. 授权包经应用中心上架→付费→`pay/notify`→`site_licenses` 签发→cap 提升，全链路打通。
6. 某站 `allow_visitor_theme_switch=0` 时，访客无法切换主题、统一显示该站 `default_theme`；另一站允许切换时访客可自由选且各自 localStorage 持久化。

---

## 11. 实施进度（M0–M4）

> 基线：`main@0655044`。以下均为**代码落地**状态；生产部署为独立运维动作（按约定不在未获授权时推送生产）。

| 里程碑 | 内容 | 状态 | 关键落点 |
|---|---|---|---|
| **M0** 数据层 | `sites` / `site_settings` 表；`files` / `tags` 加 `site_id`（默认站升级 + seed）；`SiteStore`（含 `ResolveSite` 解析优先级） | ✅ 完成 | `repo/db.go`、`service/site.go`（含单测） |
| **M1** 路由层 | `siteMiddleware` 注入 `ctx.site_id`；Host/Path→site 解析；公开面（`/public/site`、SSR 页）读 `site_settings` | ✅ 完成 | `handler/site_mw.go`、`handler/public_site.go`、`handler/routes.go`（中间件链） |
| **M1** 前端 | 访客切换开关接线（`allow_visitor_theme_switch`）；关闭时强制站点默认主题并隐藏切换器；落地页去掉「即将推出」角标 | ✅ 完成 | `themes/index.js`、`ThemeSwitch.vue`、`BlogView.vue`、`BlogPostView.vue`、`LandingView.vue` |
| **M2** 授权层 | 新建 `site_licenses` 表；`SiteCap = 1 + Σpermanent + Σsubscription(仅生效中)`；建站超额 `402 SITE_LIMIT_REACHED` | ✅ 完成 | `repo/db.go`、`service/site.go`、`handler/site_admin.go` |
| **M3** 内容隔离 | `CreateDir` / `Upload` / `CreateDoc` / `ListDir` 加 `siteID`（读过滤 + 写归属）；`EnsurePath` 保持默认站 | ✅ 完成 | `service/file.go` 及各 caller |
| **M3** 后台 | 站点 CRUD + 站点设置 + 授权管理 9 端点；管理员 `?site=<id>` 单域名下管理任意站 | ✅ 完成 | `handler/site_admin.go`、`handler/routes.go`、前端 `views/SitesManage.vue`（菜单「系统 → 多站点」） |
| **M4** 分发 | entitle 新增 `ValidateProduct(key, feature)`（不要求壳 scope）；内嵌自签公钥 `aiklog-multisite`；市场索引新增 `licenses[]` 与 `kind=license` 安装分支（下载 + SHA256 + 验签 → 写 `site_licenses`） | ✅ 完成 | `entitle/entitle.go`、`entitle/public_keys.go`、`handler/apps_market_catalog.go`、`handler/site_admin.go` |

**验证**：`go build ./internal/...` 与 `go test ./internal/{handler,service,entitle}/...` 全绿；前端 `vite build` 通过（`SitesManage` 分包产出）。

**未做（增量项，非阻塞）**：
- `pay/notify` 的商品识别直签（当前 M4 走「市场 `kind=license` 安装」与「后台手录 key」两条已打通路径；支付回调直签可后续接同一 `grantSiteLicenseKey`）。
- `cmd/aikmap/capabilities.go` 的 `Multisite` 模块门控（cap>1 才装配；当前站点后台随 CoreModules 常驻，不影响单站行为）。
- per-site sitemap / canonical（SEO 去重）与按 site 粒度的备份导出。

---

## 附录 A：与现有模块接线点清单

| 改造点 | 现有位置 | 动作 |
|---|---|---|
| 能力注册 | `cmd/aikmap/capabilities.go` `buildCoreModules` | 新增 `Multisite` 模块装配（门控 cap 是否 >1） |
| 数据迁移 | `server/internal/repo/db.go` `migrate()` | 新增 `sites` 表 + `site_settings` 表 + `files`/`tags` `site_id` 列 + 默认站初始化 |
| 路由中间件 | `server/internal/handler/routes.go` | 新增 Host/Path→site 解析中间件（mux 外层，鉴权前；含 §4.4 注入头） |
| 主题接线 | `web/src/stores/theme.js` + SSR 渲染 | 接入 `default_theme` 与 `allow_visitor_theme_switch`（v1.1.0 已定，无需编译层） |
| 分发管线 | `server/internal/handler/apps_market.go` | `kind=license` 支持 + install 端点签发 `site_licenses` |
| 支付签发 | `pay/notify` handler | 识别 license 商品 → 幂等写 `site_licenses` |
| 配额闸门 | 站点管理 / 建站接口 | 读 `site_licenses` 算 cap，超额 `402` |

> 本方案 v1.1.0 已将两处实现细节定稿：①per-site 配置采用**新增 `site_settings` 结构化列表**（非 `settings` 加 scope）；②SPA 主题 per-site **复用现有双轨 + 访客 localStorage 切换**，仅接 `default_theme` / `allow_visitor_theme_switch`，无需编译层。架构成立性不受影响。
