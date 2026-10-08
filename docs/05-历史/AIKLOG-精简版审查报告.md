# AiKlog 精简版 0.1.0 审查报告

> 审查时间：2026-09-13 · 仓库：`AiKdex/AiKlog` @ `4845d94`（单 commit 精简发行快照）
> 审查方式：源码结构审查 + 全链路构建（npm build + go build）+ 本地冒烟（AIKMAP_ADMIN_PASSWORD 启动、health/登录/公开 API/已裁路由验证）+ 站点实访（aiklog.com）
> 状态：可编译、可运行、裁剪方向正确；发现 2 个产品级遗留 + 2 个体验级提示

## 一、总体结论

精简方向与《AIKLOG-精简发行方案》一致：采集（collector/source/tools_collect/probe）、IM（telegram）、情报频道（ChannelView）、ECharts、Webhook、OnlyOffice 已从代码与路由层摘除，鉴权公开面干净，"文件 → 知识库引擎 → 公开博客"链路成立。线上站点（品牌落地页 + 样板房博客 4 篇 + 爱库录主题 + RSS）完整可用。

## 二、发现的问题（按严重度）

### 🔴 P1 品牌替换不彻底（产品级）

| 位置 | 现状 | 影响 |
|---|---|---|
| `server/internal/config/config.go:28` | 默认 `site.name: "AiKmap"` | 新用户不手动改设置时，公开站/控制台显示 "AiKmap 博客" |
| `web/index.html` | `<title>AiKmap</title>` + description "AiKmap — 自部署、AI 原生的跨存储个人知识资产中枢" | SPA 壳仍主仓品牌 |
| `cmd/aikmap/main.go:194` | 启动日志 `AiKmap dev listening on …` | 运维视角品牌未换 |
| `web/public/favicon.svg` | 主仓图标 | 标签页仍是 AiKmap |
| `AppShell.vue:221/611` | 通知面板文案"采集完成 / 导入导出终态" | "不出现采集"主张的文案残留 |

说明：线上 aiklog.com 显示"爱库录"是因为部署时在 DB 里手动改了站点设置——代码默认值未换，**新部署会暴露主仓品牌**，与《精简发行方案》"默认站点名「爱库录」"不符。

### 🟠 P2 schema 裁剪不彻底（一致性）

- 仍在建表的孤儿表（全仓库仅 schema.go/db.go 引用，无业务代码）：
  - 采集三表：`sources` / `collect_runs` / `collect_tombstones`
  - 商业化四表：`skill_packages` / `skill_grants` / `prompt_templates` / `workflow_defs`
- db.go 仍执行对应迁移：`sources.channel_type/fetch_mode/target_dir`、`collect_runs.rejected/rejected_detail`
- 功能无害（建空表），但违背《裁剪清单》"采集表/schema 精简"；且从表反推会误以为采集能力仍在
- 对照：`webhook_subscriptions` 表随 webhook.go 一并裁掉、`blog_plugins` 无 settings_schema（配套市场裁剪）——裁剪逻辑是清楚的，**采集/商业化表属于漏删**

### 🟡 P3 仓库拉取后不能直接编译（体验）

- `handler/static.go` 用 `embed all:webdist`，webdist 不入库 → 直接 `go build` 报 `no matching files found`
- README 有完整构建步骤（npm build → cp dist → go build），属"源码快照不含前端产物"设计，但对"精简发行快照"的体验有门槛——建议发布包带 webdist，或提供 release 二进制

### 🟡 P4 版本号未注入

- `GET /api/v1/version` 返回 `{"schema":2,"version":"dev"}`（主仓同设计，CI 注入）
- README 声称 0.1.0，发布时需 `-ldflags "-X main.version=0.1.0"`，建议写入发布脚本

## 三、验证通过项

- 前端 `npm run build` 10.7s 通过（42 产物，无 echarts/ChannelView 残留）；后端 `-trimpath -ldflags "-s -w"` 编译通过
- 冒烟全过：`/api/v1/health`、登录（AIKMAP_ADMIN_PASSWORD 生效）、`/public/site`、`/blog/feed.xml`、`/blog/plugins`
- 已裁路由全部不可达（`/sources` `/admin/webhooks` `/admin/ai/pools` `/office/config` → SPA 兜底）
- 鉴权公开面干净：仅 health/version/login/public 读/shares GET/feed/plugins 读/KV 读
- 主题协议完整（aiklog 默认 + qiuzhi）；5 个内置博客插件完整；system.go 首启随机密码逻辑完整

## 四、建议

1. **P1**：品牌统一收口——config 默认 `site.name` 改「爱库录」、index.html title/description、启动日志、favicon、通知面板文案去"采集"
2. **P2**：删除 schema.go/db.go 中 7 张孤儿表声明与迁移（或保留但更新裁剪文档说明）
3. **P3/P4**：发布流程脚本化（`build.sh`：npm build → webdist 复制 → version 注入 → 打包）

---

## 五、成长空间分析（2026-09-13）

**总体判断：有空间，但空间不在"博客"本身，而在底座复用。**

### 1. 作为"博客系统"：天花板有限，是存量博弈

- 博客/CMS 大盘在萎缩，内容创作流向短视频和公众号；自部署博客更是细分中的细分（Typecho/emlog/Halo 的用户群就那么大）
- WordPress 生态碾压，Typecho/emlog 靠"轻"活下来——AiKlog 的"单二进制+知识底座"能抢这个存量，但抢不到增量
- 核心矛盾：**语义检索/AI 摘要对"读者"感知弱**（读者看的是内容，不是知识库管理），AI 底座的价值主要在"作者侧"（整理、发布、找回）。博客前台很难把底座价值显性化——这是它卖不出溢价的根源

### 2. 作为"知识发布形态"：有真实差异化，对手是笔记工具而不是博客

- 真正对位的竞品是 Obsidian Publish（$8/月/站点）、Notion 公开页、Flomo 会员——**付费 SaaS 且数据不在自己手里**
- AiKlog"目录即站点、文件即文章、数据主权、自部署免费"恰好是这群人的空白：已有 VPS 的笔记党、隐私敏感的作者、想从 Typecho 迁走的人
- 这个人群不大，但付费意愿和传播力都不差（自部署圈口碑传播强）

### 3. 真正的复利：底座样板房，不是独立产品

- 已在验证的模式：求职/培训补贴频道、探路工具、aiinwork 式信息聚合——**都是同一底座换壳**。AiKlog 是"知识库→公开站"这条链的最薄样板，它把"文件目录投影成站点"的心智立住了
- 成长路径如果只盯博客：天花板就在 Typecho 的量级；如果把它当"底座的第一个对外皮肤"：后续每个垂直样板房（求智、培训、招标）都是它的变体，成长空间取决于底座能孵化多少种"目录即站点"

### 结论

AiKlog 单独做不值得押注增长（别指望它跑出用户量），但值得做扎实——它是"自部署 AI 知识库"这个底座最轻、最好演示的门面。建议它的 KPI 不是"多少博客用户"，而是"多少个垂直场景愿意拿这个壳子去套"。等它的主题/插件协议稳定，配合已有插件市场，就是把壳子交给第三方去孵化的时机。
