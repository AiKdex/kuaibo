# 上游回执 · AiKlog 需求总清单 v2.0（全量 · 一次性闭环）

- 回执方：AiKmap 主系统（上游 / 应用中心官方索引持有方）
- 回执日期：2026-09-19
- 对应：《AiKlog需求总清单-v2.0-全量-致上游.md》（41a58ab0）
- 标记约定（沿用 klog）：`已实现`（附契约/源码）/ `排期中`（附时间点）/ `不采纳`（附理由）/ `口径已定`（确认/知会类）
- 渠道：联调测试空间「协作资料」

---

## 一、已闭环（备查，无需动作）

klog 清单第一节所列 A-G + 三项澄清 + 9/18 那批全部确认已闭环。本回执不重复，仅对第二、三、四节逐项答复。

---

## 二、上游已宣布的下一轮底座 —— AiKlog 认领项回执

| # | 事项 | 结论 | 契约 / 材料 |
|---|---|---|---|
| 2.1 | 出站通道 Channel | **已记录 · 排期中（P1）** | `CoreModules` 第 8 字段 `Channel` 已预占（见 §3.1 字段终版清单），随 P1 落地：多账号凭证表（`channel_creds`：platform/cred_json/enabled）+ 发布队列（复用 jobs 队列契约）+ 频率控制（per-platform interval）。**就绪前 AiKlog 自持 ChannelView 先行，认可。** |
| 2.2 | 每日知识日报 Digest | **已实现（后端）** | 端点：`POST /api/v1/digest/run`（触发生成）、`GET /api/v1/digest/preview`（预览）；注入：`CoreModules.Digest` 非 nil 即注册。触发方式：run 手动/定时（壳侧 cron 调 run 即可）；数据结构：条目=最近一日入库文件摘要分组（来源/类型/摘要），时间口径=自然日（UTC+8 按配置时区）。订阅：单用户=全量；多用户按 owner。壳侧 `DigestView` 复用上游前端（`web/src/views/...Digest*`），最小补丁见《生产站底座A-G-壳侧集成说明.md》§5.4（本轮补）。 |
| 2.3 | 冲突监控规则化 | **已实现（基础）+ 排期中（规则页 P1）** | 数据来源：入库相似度检测（similarity 阈值，默认 100=完全相同、58 级为相关内容，见通知样例）；阈值/白名单规则存储：`settings` 表 `conflict.*` 键（`conflict.threshold` / `conflict.whitelist` JSON）；规则配置页随 P1（冲突监控规则配置页）交付。 |
| 2.4 | 收件箱 Inbox | **已实现（后端）** | 端点：`GET /api/v1/inbox`（列表）、`GET /api/v1/inbox/unread-count`、`POST /api/v1/inbox/{id}/archive`、`POST /api/v1/inbox/archive-all`；注入：`CoreModules.Inbox` 非 nil 即注册。与 D 项关系：**收件箱即 D 项投递的接收队列**（采集 tool 以 `blogPostCreate` 草稿态直写文章，或投递 inbox 待审），语义=待审/待归档条目。单用户收件箱语义：对 owner 收（采集/IM/投稿投递入箱）。壳侧 `InboxView` 复用上游前端，最小补丁见集成说明 §5.4。 |
| 2.5 | 知识质量 lint / 概念页摘要 | **排期中（P2）** | lint 规则清单：矛盾（同概念正反结论）/ 孤儿（无入链无出链）/ 过期（引用时间戳陈旧）；输出结构：`{rule, file_id, severity, evidence}`。概念页摘要：复用 AI 摘要通道（quota 随商业化计费），入口 `POST /api/v1/kb/{id}/summarize`（P2 落地）。 |
| 2.6 | AgentTools v2 运行时 | **口径已定（P2 随商业化 v0.2）** | manifest 终稿以《能力插件协议-v2草案.md》+《跨系统开放契约v1.md》§4 为准：`kind=ui\|capacity`、`hooks`（on_message/on_file/on_schedule）、`api_permissions`、`intents`、`settings_schema`、`frontend_entry`；`api_version` 提示位：v1 缺省=1，v2=2，兼容读取（v1 条目在 v2 货架零改动可装）。 |

---

## 三、本轮新增 11 项 —— 逐项回执

### 3.1 CoreModules 接入路径 【已实现 · 给字段终版清单 + 门控矩阵 + 迁移指引】

**字段终版清单（handler.CoreModules，routes.go:50，2026-09-19 基线）**：

```
Sources   *service.SourceStore    // 采集源（GET/POST/PUT/DELETE /api/v1/sources）
Collector Collector               // 采集执行器（/api/v1/collect/...）
WebDAV    *service.WebDAVStore    // 外部 WebDAV 挂载（/api/v1/webdav/mounts...）
Digest    *service.DigestStore    // 每日日报（/api/v1/digest/run|preview）
Inbox     *service.InboxStore     // 收件箱（/api/v1/inbox...）
Review    *service.ReviewStore    // 评审/评价（/api/v1/review/queue|rate|stats）
Org       *service.OrgStore       // 组织架构（/api/v1/org/settings|tree）
Channel   *service.ChannelStore   // 出站通道（P1 预占位，随 P1 落地注册路由）
```

**Routes() 门控矩阵**（routes.go：字段 nil → 对应路由组 404=能力未启用，不会 panic）：

| 字段 | 非 nil 注册的路由 |
|---|---|
| Sources | GET/POST /api/v1/sources，PUT/DELETE /api/v1/sources/{id} |
| Collector | 采集执行/任务查询路由组 |
| WebDAV | /api/v1/webdav/mounts 全套（list/create/update/delete/test/list/content/import） |
| Digest | POST /api/v1/digest/run、GET /api/v1/digest/preview |
| Inbox | GET /api/v1/inbox、unread-count、archive、archive-all |
| Review | GET /api/v1/review/queue、POST /api/v1/review/rate、GET /api/v1/review/stats |
| Org | GET /api/v1/org/settings、GET /api/v1/org/tree |
| Channel | （P1 落地后补） |

**老 fork 增量接入最小迁移指引**（AiKlog 位置参数注入 → CoreModules）：
```go
// 改前：handler.New(db, cfg, b, aud, gate, files, tags, kb, agent, summarizer, impex, notify, wh)
// 改后：多传一个 mods 参数
mods := handler.CoreModules{
    Sources: sourcesStore,      // 有则填，无则留空
    Digest:  digestStore,       // 按需启用
    Inbox:   inboxStore,
}
api := handler.New(db, cfg, b, aud, gate, files, tags, kb, agent, summarizer, impex, notify, wh, mods)
```
- 字段留空 = 对应路由不注册，行为等价旧版（**向后兼容，可逐步启用**）。
- **确认**：AiKlog 已自建 `ai.Registry` + `reg.Register`（当前 6 工具）与上游同形——**直接对齐，无需重复造**；后续官方工具（web_clip 等）以同样方式注册即可。

### 3.2 采集工具构造器 【已实现（web_clip）+ 排期中（WebDAV/RSS 导入）】

- **`ai.NewWebClipTool` 构造签名**（主系统 `server/cmd/aikmap/main.go:226`）：
  `ai.NewWebClipTool(impex *service.ImpexStore, files *service.FileStore, ownerID string, homeSpaceID string) *ai.Tool`
  依赖端点：`blogPostCreate` 草稿态（投递文章）+ 收件箱投递（`/api/v1/inbox` 入箱）。AiKlog 同步主干后可直接移植该构造器（复用内核 `impex.CreateURLImport`，零私有依赖）。
- **WebDAV 导入、RSS 导入**：官方优先补齐项，**排期中**（随官方工具矩阵 P1）；届时按《AgentTools开发规范》官方工具注册矩阵发布构造器。
- 《AgentTools开发规范》已补「官方工具注册矩阵」章节（本轮入库，见附录 §A）。

### 3.3 主题宿主 options_schema 自动表单 + theme_settings 下发 + SSR 注入 【口径已定 · 主系统亦未实现，列入 P1】

**如实答复**：主系统当前 `GET /api/v1/public/site` 仅返回 `blog.*` 白名单（title/description/logo/footer/seo_default/custom_css/custom_js/base_url/locale），**未实现 `theme_settings` 下发与 options_schema 自动表单**——与 AiKlog 现状一致，这是主题生态闭环的共同缺口。

**契约（已定，随 P1「主题市场闭环」落地）**：
- 存储：`settings` 表 `theme.<theme_id>.*` 键（站长设置持久化）；
- 下发：`GET /api/v1/public/site` 返回体增加 `theme_settings`（JSON，键=options_schema 声明项）；
- 表单：宿主按 manifest `options_schema[]` 自动渲染设置表单（渲染组件随 P1 提供，AiKlog 可移植）；
- SSR：主题模板经 `{{.ThemeVars}}` 注入 CSS 变量（`--th-*` 前缀，契约 §6 命名不变）。

### 3.4 市场卡片图片化 + demo.html 在线预览 【排期中（P1）· 契约已定】

**主系统当前未实现** `preview/*` 字段（索引条目仅有 readme/homepage/icon 可选）。契约（已定，随 P1 市场卡片升级落地）：
- 索引字段：`cover`（封面图 URL，≥640px）/ `screenshots[]`（截图 URL 数组）/ `demo_url`（在线预览地址，可选）——三者均 optional，向后兼容；
- 主题包结构沿用主题规范 §0.4（`preview/cover.png` + `preview/demo.html` + `README.md`）；
- 宿主渲染：封面图 `cover` 取址；`demo.html` 在线预览**必须 iframe 沙箱**（`sandbox="allow-scripts"`，禁 cookie/外链脚本，无 remote code），预览入口挂市场卡片详情弹层。

### 3.5 结构化分类树数据源 【排期中（P1）· 契约已定】

**主系统当前未向主题透出分类树**（公开轨按 path 前缀过滤）。契约（已定）：
- 字段形态：`[{ "id", "name", "path", "children": [...], "count" }]`（递归树，count=直接文章数）；
- 注入点：新增独立公共 API `GET /api/v1/public/categories/tree`（匿名可读，CORS 同市场索引）+ 主题上下文 `themeContext.categories`（博客数据源时注入）；
- 落地：P1 随「主题市场闭环」。

### 3.6 Entitlement v0.2 【排期中（随商业化 v0.2）】

- 交付：实例强绑定（sub）+ 吊销黑名单（签发侧维护）+ 签发服务（端点/CLI）；
- AiKlog 自研 `internal/entitle` 合并路径：**沿用既有口径**——公共 API 已一致，v0.2 签发服务上线后并入零成本（届时给 diff 指引）；
- 排期：随商业化 v0.2（与 AgentTools v2 同批）。

### 3.7 计费结算 v0.3 【口径已定 · 排期中】

- 方案 A（上游统一代收、按约定分成）确认采纳；
- 分成比例 / 结算周期 / 对账口径：**商业化方案 v0.2 定稿时一并发布**（当前不承诺数字，避免拍脑袋口径）。

### 3.8 paid 货架样本（target 含 aiklog） 【已实现】

**已满足**：官方货架唯一 paid 商品 `pro-advanced` 的 `target` 已扩为四壳 `[aikmap, aiklog, aikbox, aikdex]`（commit a601d1f，服务器 `/opt/aikmap-live/market/index.json` 已生效）。AiKlog 可在真实货架验证「买 paid → 402 门禁 → 放行 → 安装成功」全链路。

### 3.9 增量/混合渲染 【口径已定】

主系统当前公开面为**按需渲染**（REST 按需 + 公开 API 分页），无全量 SSG 导出（千级条目站判定的"扛不住"成立）。规划：**增量/混合渲染（ISR 类）排期 P2**（随生产站 T0/T1 承载量级）。若 AiKlog 需立即承载千级条目站，可先自建增量导出（静态壳 + 按需回源），上游 P2 落地后再对齐。

### 3.10 投稿（UGC 入口）与审核队列 【口径已定 · 现成通道可用】

- **Review 模块不覆盖投稿**（语义为评审/评价：review/queue、rate、stats）；
- **现成可用路径**：收件箱 Inbox 即投稿通道——前台投稿 tool 投递 → inbox 待审 → owner 审核归档/发布（AiKlog 若已有评论待审链路，同模式扩展即可）；
- 专用投稿端点（前台表单 → 待审 → 发布状态机）：**排期中（P1）**，届时给字段与状态机口径（draft→pending→published/rejected）。

### 3.11 自动内链 / 孤岛提醒统一契约 【口径已定】

- 上游**无统一 NodeLink 契约**；E 项链接健康检查只管外链存活（broken 列表），与自动内链/相关文章是**两层语义**，不冲突；
- AiKlog 已自建双链/断链检测（kb.go/impex.go/indexer.go）**合理且保留**，无需等上游；若要统一成主题可消费的「相关文章/自动内链」数据源，随主题生态 P2 排期。

---

## 四、我方自持（知会收到，无需动作）

1. 主题铺开（~18 套 SPA + SSR 角标）——知会收到；
2. ChannelView 自持先行——认可（与 2.1 一致）；
3. 壳侧缺陷修复（install-zip 校验顺序、theme_options title）——知会收到；
4. 多用户前端 UI——知会收到；
5. 真实支付前端（等 pay/notify 契约）——pay/notify 已上线（A-G 已验证），AiKlog 可直接接；
6. P0/P1/P2 分支 sync/ag-base 待评审——知会收到，欢迎评审反馈。

---

## 五、响应方式确认

- 本回执已投递联调测试空间「协作资料」（与 klog 清单同空间）；
- 如需在 AiKlog-协作空间直接追加：**上游同意将 admin 账号加入该空间成员**（AiKlog 侧开放即可，我方无异议）；
- 下一轮沟通建议：以本回执 + klog 清单逐项勾选核对，双方各自归档。

---

*回执方：AiKmap 主系统 admin · 2026-09-19*
