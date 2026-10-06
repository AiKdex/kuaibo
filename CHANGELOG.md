# 更新日志 · 爱库录 AiKlog

本项目从 AiKmap 主系统精简发行而来。版本号从 **0.1.0** 起独立演进。

---

## [0.1.1-family.4] - 2026-10-01

### 修复

- **family.3 回归修复**：family.3 由源码受损的本地树构建（269 文件缺失 + 249 文件陈旧，含 entitle 公钥、B30 安全加固、Org 授权、REQ-007 在线验签接线），静默剥掉了 family.2 已有能力。本版以 `c378ca8`（family.2）完整树为底，仅叠加侧栏优化三文件
- `/api/v1/license/verify`（REQ-007 在线验签）恢复可用

### 侧栏

- 移除冗余「检索」导航项（顶栏全局搜索同路由 `/search`）
- 图标去撞车：家族传承 `clock→heart`、草稿箱 `edit→fileText`、静态页 `book→globe`
- 修复 AikIcon：`database` 图标 `ellipse` 类型不被模板支持导致空白（改纯 path）、重复 `edit` 键去重、补 `heart`/`layers`/`globe`

### 部署

- bin md5 `653e1076b891c4ef2834f043c61afe9e`（36,110,496 B），124 与生产互等；回滚点 `aiklog.bak.20261001132454`（124）/ `aiklog.bak.20261001132529`（生产）

---

## [0.1.23] - 2026-09-13

### 链接

- 公开文章主链改为 **`https://站点/{slug}`**（中文根路径，对齐 emlog/Typecho 习惯）
- 保留 `/blog/{slug}` 兼容；列表仍为 `/blog`
- 样板房主题改回 **爱库录（aiklog）**

---

## [0.1.22] - 2026-09-13

### 主题

- 新增 **暖纸情报**（`paper`）：暖纸底 + 青绿渐变顶栏 + 珊瑚强调（对齐求智情报站视觉）
- 站点设置可选；SSR `/blog` 同步该视觉

## [0.1.21] - 2026-09-13

### 检索

- 语义向量召回增加相关度门槛（`index.vector_min_score`，默认 0.45），避免弱相关「吐全库」

---

## [0.1.20] - 2026-09-13

### 修复

- 侧栏「对话 Agent」：点击打开右侧 **AI 助手**（原先为占位、无响应）
- 品牌改为「爱库录 / AiKlog」
- 「共享」标为规划中；「应用中心」改为跳转设置（应用/插件）
- 「智能检索」保留 `/search` 路由

---

## [0.1.19] - 2026-09-13

### 架构

- **博客目录对文件管理可见**（精简发行）：可在文件库拖拽 md 到「博客」目录
- 开启 `blog.auto_publish_on_upload` 后，上传到博客子树的 md **自动 published**
- 仍保留：博客根目录禁止删除/移动/改名（防误删整站）

---

## [0.1.18] - 2026-09-13

### 主题化 SSR

- `/blog` 与 `/blog/{slug}` 的 HTML **跟随 `blog.theme`**（aiklog / minimal / docs / default）
- 后台改主题后，静态阅读页视觉同步变化（仍为服务端 HTML，SEO 不变）

---

## [0.1.17] - 2026-09-13

### 架构

- **双轨定案**：读者主链 `/blog`（SSR）；主题改为 **站点设置**（`blog.theme`）
- 公开 SPA 不再页头切换主题；控制台博客管理可改，保存后对所有访客生效

---

## [0.1.16] - 2026-09-13

### 体验

- 公开文章页统一 **860px 容器**：正文、侧栏插件、post_bottom（问答/评论/相关）同宽居中
- 公开页品牌改为爱库录；页脚链到交互版 / 静态页

---

## [0.1.15] - 2026-09-13

### 体验

- 静态页导航/页脚增加 **「切换主题」** 入口（`/app#/blog?view=public`）
- SPA 主题条增加 **「静态页」** 反向链接

---

## [0.1.14] - 2026-09-13

### SEO

- 公开页注入 **JSON-LD**：列表 `Blog`+`BlogPosting`，文章 `BlogPosting`（headline/description/dateModified/wordCount）

---

## [0.1.13] - 2026-09-13

### SEO

- **服务端公开页**：`GET /blog` 列表、`GET /blog/{slug}` 文章（goldmark 渲染，爬虫可读全文）
- sitemap 改为无哈希 `/blog/{slug}`
- 主题公开链接改为 `/blog/{slug}`

---

## [0.1.12] - 2026-09-13

### 修复

- 主题列表链接改为 **优先 slug**（`?slug=`），改文件名/分类不碎链；无 slug 回退 `?path=`（旧链接仍可用）

---

## [0.1.11] - 2026-09-13

### 新增

- `GET /robots.txt`（指向 sitemap，禁止抓控制台）
- 白皮书更新至 v1.0（与 0.1.10 功能对齐）

---

## [0.1.10] - 2026-09-13

读者 AI、访问统计、第三主题。

### 新增

- **读者 AI 问答**：`POST /api/v1/public/blog/ask`（公开、限流；仅注入公开文章上下文）+ `blog-reader-ai` 插件
- **访问统计**：`POST/GET /api/v1/public/blog/pv` + `blog-stats` 插件（本地 KV 计数）
- **技术文档主题** `themes/docs`（侧栏目录 + 正文栏）

### 安全

- 公开问答不开放文件库工具，防私有数据外泄
- 每 IP 5 分钟最多 8 次提问

---

## [0.1.9] - 2026-09-13

### 新增

- **极简阅读主题** `themes/minimal`：单栏沉浸列表，可在博客主题切换器中选择

---

## [0.1.8] - 2026-09-13

应用市场 v0.2：安装与启停。

### 新增

- `POST /api/v1/admin/apps/install-zip`：multipart zip（含 `manifest.json`）或 JSON `{manifest}`
- 设置页「应用市场」：上传安装、启用/禁用、卸载
- `builtinFrontendEntries` 补全 9 个内置组件入口

### 说明

- 当前仅允许 `frontend_entry` 命中内置组件的应用（组件随主系统构建）
- 免费应用阶段；签名与远程索引见协议 v0.3

---

## [0.1.7] - 2026-09-13

文档与样板房内容补齐。

### 新增

- `docs/PLUGIN-MARKET.md` 应用市场协议草案（v0.1 包结构 / manifest / 安装阶段）
- 样板房新增 3 篇：工作流、自部署价值、0.1.x 路线图（共 **7** 篇公开文）

---

## [0.1.6] - 2026-09-13

侧栏支持 **左 / 右** 切换（默认右侧，`localStorage` 记忆 `aiklog.sidebar`）。

---

## [0.1.5] - 2026-09-13

爱库录主题增加 **侧栏**（桌面双栏）。

### 新增

- 右侧栏：关于 / 分类 / 归档
- 分类与月份可点击筛选列表，可一键清除
- 窄屏侧栏移到列表上方

---

## [0.1.4] - 2026-09-13

爱库录主题列表改为 **左图右文** 博客卡片。

### 变更

- 列表卡片：左侧封面（preview 首图，无图时分类首字占位）+ 右侧标题/摘要/分类/日期
- Hero 文案对齐主宣传「目录即站点，文件即文章」
- GitHub 链接指向 `AiKdex/AiKlog`

---

## [0.1.3] - 2026-09-13

阶段 C 前半：封面插件、公开页品牌与摘要清理、LICENSE。

### 新增

- **封面图** `blog-cover`（list_item）：从 preview 提取首图
- **LICENSE**（MIT）

### 变更

- 公开默认主题顶栏改为「爱库录 / AiKlog」，入口指向控制台
- 列表日期优先 `file.updated_at`；摘要去 markdown 图/标题与降级前缀

---

## [0.1.2] - 2026-09-13

阶段 B：博客门面插件 + SEO sitemap + 安装文档。

### 新增

- **相关推荐** `blog-related-posts`（post_bottom）：同分类优先，按更新时间，纯公开数据
- **归档** `blog-archives`（sidebar）：按月分组
- **分类云** `blog-tag-cloud`（sidebar）：path 首段计数
- **sitemap** `GET /api/v1/blog/sitemap.xml`（公开）
- `docs/INSTALL.md` 安装与部署 runbook
- 发布脚本已可注入版本（见 0.1.1）

### 变更

- 内置插件作者名与信息卡来源改为「爱库录」
- 种子插件增加 related/archives/tag-cloud（启动自动登记）

---

## [0.1.1] - 2026-09-13

响应上游《AiKlog 精简版审查报告》：品牌收口 + schema 精简 + 发布脚本。

### 修复

- **P1 品牌默认值**：`site.name`、博客站点默认标题/简介/页脚、SPA `index.html`、启动日志、favicon 均默认 **爱库录**（新部署不再露出 AiKmap）
- 通知面板文案去掉「采集完成」
- `config_test` 期望值同步

### 变更

- **P2 schema**：删除孤儿表 `sources` / `collect_runs` / `collect_tombstones` 与 `skill_*` / `prompt_templates` / `workflow_defs`；删除对应列迁移
- 新增 `scripts/build-release.sh`：npm → webdist → go build，注入 `-X main.version=`
- favicon 改为爱库录玉色标识

### 说明

- 旧库中已存在的空表不影响运行；新库不再创建
- 用发布脚本构建时 `/api/v1/version` 返回真实版本号

---

## [0.1.0] - 2026-09-13

首个可部署精简发行快照，并与样板房 `aiklog.com` 对齐。

### 新增

- **爱库录主题**（`web/src/themes/aiklog`）：冷纸底 + 玉色目录卡；`aiklog.com` 默认激活  
- **后台路径** `/#/desk`（`/login` 重定向；不对外宣传）  
- **上传后自动发布** 设置 `blog.auto_publish_on_upload`（默认 `false`）  
  - 开启：博客管理拖拽/选择的 Markdown 上传即 `published`  
  - 关闭：保持草稿，由站长确认后发布  
- 博客管理站点设置中可切换上述开关；上传区提示随开关变化  
- 文档：白皮书、精简发行方案、样板房计划、主题设计  

### 变更（相对全量 AiKmap）

- **裁剪采集**：collector / source / collect API / Agent 采集工具  
- **裁剪 IM**：Telegram 网关与意图路由  
- **裁剪依赖**：`bogdanfinn/tls-client` 及间接依赖；前端移除 `echarts`  
- **裁剪前端**：情报频道 `ChannelView`、求智（采集型）主题不注册  
- **知识图谱**：精简版占位提示，不再依赖 ECharts  

### 体积

| 项 | 全量 | 精简 |
|---|---|---|
| Linux 二进制 | ~20MB | **~13MB** |

### 保留（博客核心）

文件引擎、标签、发布/shares、slug、站点设置、评论核、RSS、四挂载点插件、主题协议、鉴权、SQLite、事件总线、语义检索底座。

### 样板房

- HTTPS `https://aiklog.com`  
- 产品落地页（静态）+ `/media` 静态图  
- 示例文章与图文已发布  

### 安全

- 登录限流、HttpOnly Cookie（HTTPS 自动 Secure）、公开面方法敏感白名单  
- 站长须自行修改管理员口令  

### 已知限制

- 应用市场 / 付费主题未做（规划中）  
- 相关推荐、归档、标签云、封面等 P1 插件待做  
- 相对全量版无采集与 IM  

---

## 后续计划（摘录）

见 `docs/爱库录白皮书.md` §11 实施落地计划：

1. P1 插件（相关推荐 / 归档 / 标签云 / 封面）  
2. 安装 runbook 与发布包  
3. 读者 AI 问答入口  
4. 应用市场雏形（免费应用）  
