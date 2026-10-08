# 生产站底座 A-G · 对接确认（AiKlog 致上游 v1.0）

- 编制：爱库录 · AiKlog（壳侧）
- 日期：2026-09-19
- 渠道：联调测试空间 / 协作资料
- 关联：9/18《上游配合事项清单-AiKlog致上游-v1.0》（已书面闭环 + E2E 全绿）、上游《生产站底座A-G-壳侧集成说明》

---

## 0. 背景与目的

9/18 那批需求（A1 Entitlement 验签 / A2 AgentTools 协议 / A3 货架 / B1-B3 字段终版）已在协作空间**书面闭环**，并经 AiKlog 生产站 aiklog.cn 端到端验证（commit `be75f7c`，7 场景全绿）。

本轮我方观察到上游在主系统内核交付了「**生产站底座 A-G**」（提交链 `d8d69ac → 0440bfc → bc585dc → d582a6d → 7c7b0f2`，附《生产站底座A-G-壳侧集成说明》）。该底座与我方此前在《生产站应用场景》中规划的「内容类型 / 付费 / 批量 / 采集 / 链接健康 / SEO / 存储+CDN」高度吻合。

本文件用于**正式走闭环**：把 A-G 纳入协作空间的需求/回执链路，请上游确认交付范围，并约定下一轮事项。

---

## 1. AiKlog 侧已确认收到并认可的交付（A-G）

| 模块 | 内核交付（上游） | AiKlog 侧待办（壳侧） |
|---|---|---|
| **A 内容类型+自定义字段** | `content_state`（`node_type`+`fields` JSON）随文章列表/公开列表/正文随行返回；`PUT /files/meta`、`POST /posts/batch`(action=meta) 可写 | 主题按 `node_type`（post/doc/resource/link/page）差异化渲染；`fields` 业务语义由主题/插件定义 |
| **B 付费/免费变现授权** | 三态访问 none/password/paid；无凭证访问付费 → `402`+`price_cents`+`paid_preview`；`POST /pay/notify` HMAC 验签 + 幂等签发 `grant_token` | 支付 tool（微信/Stripe/支付宝）接 `pay/notify`；文章页付费 CTA + 解锁页前端 |
| **C 批量编辑表格视图** | `POST /posts/batch`，action=pin/schedule/paid/meta/delete，ids 1–200，越界整批 403 | 同步主干即得（BlogManage 批量栏） |
| **D 采集 tool 集** | 投递协议就绪：`blogPostCreate`(草稿态) / 收件箱；应用中心 `target` 字段控制适用性 | 各壳 `main.go` 增 `reg.Register(ai.NewXXXTool(...))`（WebClip/RSS/WebDAV 导入） |
| **E 链接健康检查** | `GET /blog/links/check` 并发 HEAD 探测，同域跳过，返回 `{total,broken,links[]}` | API 直接调；前台加"巡检"按钮或定时任务 |
| **F SEO 增强** | `GET /sitemap.xml` 公开，含首页+已发布文章，定时未到自动排除 | 主题 `<head>` 引用并提交搜索引擎 |
| **G 存储多后端 + CDN 直出** | `storage.default_backend`=local/webdav/s3；CDN 直出开关；local+加密自动禁直出 | AiKlog 已有 storage 抽象，按部署选后端即可 |

> 说明：AiKlog 此前已具备存储多后端、评论、双链/断链检测、向量检索、AI 组稿、定时发布、知识库体检等能力，与 A-G 底座同构/互补，不重复造。

---

## 2. AiKlog 侧动作清单（不阻塞上游，按节奏推进）

- **P0 同步主干 + CoreModules 接线**：同步上游 main，核对 `handler.CoreModules` **7 字段**（Sources / Collector / WebDAV / Digest / Inbox / Review / Org）逐字段接线与 `Routes()` 门控，防止新模块静默不可用。
- **P1 主题按 `node_type` 差异化渲染**：resource 显下载区、link 显外链卡片、字段做筛选/卡片。
- **P1 付费 CTA + 解锁页前端**：复用已有 `article_access` 密码解锁，补 paid CTA（价格+跳支付 tool）与 `paid_preview` 策略展示。
- **P2 注册采集 Agent Tools**：`reg.Register` WebClip / RSS / WebDAV 导入，复用内核 `impex`，零私有依赖。
- **P2 前台增强**：链接巡检按钮、`sitemap.xml` 引用。

---

## 3. 待上游确认 / 澄清（请在本空间回执）

1. **出站多平台分发**：`ChannelView.vue` 前端 + `digest` 服务已在，但**未列入 A-G 底座表**。该项归属——纳入下一轮底座，还是壳侧轻量实现（复用已有定时发布/付费能力）？
2. **AgentTools v2 运行时（A2）**：上游回执称"仅规格就绪，AiKlog 按 v1 先行"。v2 运行时的时间表 / 与 v1 的差异点？
3. **采集 tool 注册清单**：三壳（aiklog/aikbox/aikdex）各自需补的 `reg.Register` 行，是否有官方统一清单，避免各壳漏注册。

---

## 4. 请上游在协作空间回执

- 确认 A-G 交付范围与我方第 1 节理解一致；
- 给出第 3 节三项的处理方式 / 排期；
- 如有下一轮底座事项，请在同一空间列出，便于我方同步排期。

AiKlog 壳侧将据此推进 P0→P2，并在完成后回传联调结果。
