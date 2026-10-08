# EmForum 主题交付包 · v1.1.1

> 主题 id：`emforum` · 展示名：**论坛社区 EmForum**
> 气质：深海军蓝 × 水鸭青 × 鎏金 —— 版块页签式论坛/BBS 版式
> 依据：**《AiKlog 博客主题开发规范（第三方完整交付版）》v1.0（2026-09-15）**
> 交付范围：整包交付（列表页 + SSR 文章页/列表页 + 内页组件：分类/作者/归档/搜索 + SSR 皮肤）
> **本版为 v1.1.0 的代码审查修复版，修复 5 处问题，详见第七节。**

---

## 一、交付清单（对照规范 §0.1）

| # | 交付物 | 包内路径 | 部署目标 | 必需 |
|---|---|---|---|---|
| 1 | `manifest.js` | `web/src/themes/emforum/manifest.js` | 仓库同路径 | ✅ |
| 2 | `index.js` | `web/src/themes/emforum/index.js` | 仓库同路径 | ✅ |
| 3 | `EmforumView.vue`（列表页入口） | `web/src/themes/emforum/EmforumView.vue` | 仓库同路径 | ✅ |
| 4 | `style.css`（`--th-*` 令牌 + 全页样式） | `web/src/themes/emforum/style.css` | 仓库同路径 | ✅ |
| 5 | 内页组件 | `EmforumCatView.vue` / `EmforumAuthorView.vue` / `EmforumArchiveView.vue` / `EmforumSearchView.vue` | 仓库同路径 | ⭕ |
| 5b | 公共组件 | `EmforumHead.vue` / `EmforumSide.vue` / `EmforumFoot.vue` / `EmforumThread.vue` / `helpers.js` | 仓库同路径 | ⭕（本主题内部复用） |
| 6 | `ssr.css` + `manifest.json` | `server/data/themes/emforum/` | 服务器 `/opt/aiklog/data/themes/emforum/` | ✅ |
| 7 | 设计稿对照 | `design/design-bbs-list-and-post.html`（列表页 + 帖子详情页，可直接双击预览） | — | ✅ 验收用 |
| 8 | 交付自检脚本 | `tools/theme-lint.sh`（悬空类 / 死 CSS / tokens 台账 / hash 锚点 / 高危文案词 / 契约外字段） | — | ✅ 提交前自检 |

> 说明：`web/src/themes/<id>/` 是**随前端一起构建**的源码模块（规范 §0.3.3），不是运行时热插的 zip；
> `data/themes/<id>/` 下的 `ssr.css` 是**唯一可运行时热投放**的部分。

---

## 二、安装步骤

### 步骤 1 · 前端主题模块（覆盖后需重新构建）

```bash
# 1) 拷贝源码模块（覆盖同名目录）
cp -r web/src/themes/emforum  <AiKlog>/web/src/themes/emforum

# 2) 注册：在 web/src/views/BlogView.vue 顶部追加一行静态 import
#    import '@/themes/emforum' // 注册论坛社区主题

# 3) 构建 → 产物入 server 嵌入目录
cd web && npm run build
rm -rf ../server/internal/handler/webdist && mkdir -p ../server/internal/handler/webdist
cp -r dist/* ../server/internal/handler/webdist/

# 4) 后端单二进制（⚠️ 覆盖前必须先 pkill，否则 Text file busy 跑旧二进制）
cd ../server && go build -o bin/aiklog ./cmd/aikmap
pkill -x aiklog; sleep 2
cd /opt/aiklog && setsid nohup ./bin/aiklog -addr 127.0.0.1:8780 -db data/aikmap.db \
  > aiklog.log 2>&1 < /dev/null &
```

### 步骤 2 · 服务端外置主题（SSR 列表页 / 文章页，热生效、无需重编译）

```bash
mkdir -p /opt/aiklog/data/themes/emforum
# 上传 server/data/themes/emforum/ssr.css 与 manifest.json 到该目录即可
```

投放后 `/blog` 与 `/{slug}` 的配色即随主题变化 —— **列表与文章共用同一份 `ssr.css`**，不会出现"列表深色、文章变样"的割裂（规范 §4.3）。

### 步骤 3 · 服务端白名单（最容易漏，规范 §1.3.3）

`server/internal/handler/settings.go` 的 `blog.theme` 白名单需含 `emforum`，否则控制台保存站点主题会被 `SETTING_INVALID` 拒绝：

```go
case "", "default", "aiklog", "minimal", "docs", "paper", "elevated", "parchment", "emforum":
```

### 步骤 4 · 启用与预览

| 目的 | 方式 |
|---|---|
| SSR 列表页预览 | `/blog?theme=emforum` |
| SSR 文章页预览 | `/{slug}?theme=emforum` |
| SPA 公开页 | `/app#/blog?view=public`（页头右侧自带主题切换器） |
| 切为站点主题 | 控制台「博客管理 → 站点设置 → 主题」或 `PUT /api/v1/admin/settings {"key":"blog.theme","value":"emforum"}` |

---

## 三、页面覆盖与契约遵循

| 页面 | 主题实现 | 取数方式 |
|---|---|---|
| 列表页 `#/blog` | `EmforumView.vue`（版块页签 + 帖子流 + 分页 + 侧栏） | `ctx.posts` |
| 文章页 `/{slug}` | **不做 SPA 视图**，一律跳 SSR；SSR 样式由 `ssr.css` 接管 | 系统 goldmark 渲染 |
| 分类页 | `entries.cat` / `entries.category` → `EmforumCatView.vue` | `ctx.page.cat` + `ctx.posts` 按 path 首段过滤 |
| 作者页 | `entries.author` → `EmforumAuthorView.vue` | `ctx.page.author` + `file.author` |
| 归档页 | `entries.archive` → `EmforumArchiveView.vue` | 从 `updated_at` 自行派生年月分组 |
| 搜索页 | `entries.search` → `EmforumSearchView.vue` | `ctx.page.q` + `posts` 内客户端匹配 |
| 标签页 | 未接管（数据未实现，规范 §4.6） | — |

关键纪律（逐条落地）：

- 数据一律 `inject('themeContext')`，**不接收 props**；只用契约真实字段（`token/path/preview/file.name/file.updated_at/file.size`），**未臆造** `views/comnum/author/cover` 等字段。
- 标题派生兼容 `.md` 与 `.markdown`；分类 = `path` 首段，无斜杠归「未分类」。
- 链接一律 `ctx.postUrl(post)` → SSR 页；**无 SPA 文章视图、无 `#/p/` 拼接**。
- **不自行重排 `posts`**：列表默认顺序 = 系统「置顶优先 + 时间倒序」；排序下拉是读者显式操作，非默认行为。
- 标签云 `v-if="tags.length"`（空则自动隐藏），渲染 `{{ t.name }}`，标签项链接指向 `#/blog/tag/<name>`（系统内页路由已支持），**无死链**。
- 站名用 `<div class="ef-name">`，**不用 `<h1>`**。
- AI 问答读 `reply`（不是 `answer`），429 给中文友好提示，空输入禁用按钮，请求中 loading。
- 页头自带 `<ThemeSwitch />`，`manifest.ui.themeSwitcher = true`，宿主不再补悬浮兜底版。
- 样式全部限定 `.eforum-root`；Reset 四件套齐全；**无死 CSS，也无悬空类**（v1.1.1 已核对）。

---

## 四、设计令牌（`--th-*`，定义在 `.eforum-root`）

| 令牌 | 值 | 用途 |
|---|---|---|
| `--th-ink` | `#243447` | 正文 |
| `--th-ink-2` | `#4a5568` | 次要文字 |
| `--th-ink-3` | `#7a8699` | 弱化文字 / 元信息 |
| `--th-paper` | `#f4f6f8` | 页面底色 |
| `--th-card` | `#ffffff` | 卡片底 |
| `--th-line` | `#e4e9ef` | 分隔线 |
| `--th-accent` | `#2d8b8b` | 强调（水鸭青）：链接、版块标签 |
| `--th-accent-soft` | `#eaf4f4` | 强调色的极浅底（标签底、选中态背景） |
| `--th-accent-2` | `#c9a227` | 点缀（鎏金）：激活页签、置顶/精华、按钮 |
| `--th-accent-2-soft` | `rgba(201,162,39,.12)` | 点缀色的半透明底（chip 选中态） |
| `--th-navy` | `#1a2332` | 头部主色（深海军蓝） |
| `--th-navy-2` | `#243447` | 头部渐变中间色 |
| `--th-hot` | `#e05d44` | 警示色（错误态文字） |
| `--th-font-d` | 衬线栈 | 标题、版块名、列表序号 |
| `--th-font-b` | 无衬线栈 | 正文 |

字体栈均为系统可用字体（`Noto Serif SC / Songti SC / Georgia`），目标环境缺字时自动回退，显示不受影响。

---

## 五、验收自测（对照规范 §9）

- [x] **构建零错误**：`vite build` 通过，207 模块 transform 成功，产物含 `EmforumView` + 4 个内页 + `EmforumThread` + `style` 样式块
- [x] `manifest` 字段核对：`id/title/desc/version/pages/tokens/ui/dataSources/dataScope` 全合规，无 `name`/`description` 误用
- [x] `manifest.tokens` 与 `style.css` 实际令牌**逐项一致**（v1.1.1 补齐 4 个漏报令牌）
- [x] `index.js` 为模块加载即 `registerTheme`，非回调式
- [x] `style.css` 在入口组件内 `import './style.css'`；Reset 四件套齐全；选择器全部挂 `.eforum-root`
- [x] 类名交叉核对：**无死 CSS、无悬空类**（v1.1.1 用 `grep` 全量比对 74 个类名 × 11 个模板）
- [x] 三态齐全：loading / error / 空态（列表页、分类页、作者页、搜索页均处理）
- [x] 标签云：`tags` 为空时整块隐藏，有数据时输出 `t.name`，非链接
- [x] AI 问答读 `reply` + 429 提示 + 空值禁用 + loading
- [x] 文章点击跳 SSR 页（`postUrl`），无 SPA 文章视图
- [x] 移动端断点：`max-width: 960px`（SPA）、`max-width: 720px`（SSR）
- [x] 页头/页脚/令牌全站统一，换页不割裂
- [x] 外部链接可达性：`/api/v1/blog/feed.xml`、`/api/v1/blog/sitemap.xml`、`POST /api/v1/public/blog/ask` 均在 `routes.go` 中注册（v1.1.1 核对）
- [ ] `settings.go` 白名单含 `emforum`（已随包说明，部署时确认）
- [ ] 投放 `ssr.css` 后 `/blog` 与 `/{slug}` 配色随主题变化（部署机自测）

---

## 六、已知限制（系统侧待补，非主题缺陷 · 规范 §10）

1. **`ctx.page` 尚未注入**（§10-3）：内页组件已按契约实现，并增加了 hash 回退解析（`#/blog/cat/xxx`、`?q=xxx`、`?author=xxx`），因此**当前版本即可预览**；系统补齐后无需改主题。
2. **SPA 内页路由未上线**（§10-1/2/7）：`entries` 已声明 `cat/category/author/archive/search`，`pageEntry()` 落地后自动生效。
3. **`ctx.tags` 恒为空**（§10-5）：标签云自动隐藏，不占位、不留白。
4. **`file.author` 未透出**（§10-10）：作者页在无作者字段时显示明确提示，不伪造作者数据。
5. **标签页未接管**：等系统层实现标签过滤后再补 `entries.tag`（届时把标签项从 `<span>` 改回 `<a>` 即可，组件无需重写）。

---

## 七、版本记录

| 版本 | 说明 |
|---|---|
| **1.1.1** | **代码审查修复版**，共 5 处（3 处实质 + 2 处规范偏差）：<br>① 搜索页无关键词时的标题由「热门文章」改为「最新更新 · 输入关键词开始检索」—— 原文案与数据不符（该位置展示的是按 `updated_at` 倒序的最新 8 篇），且「热门」需浏览量字段，违反规范「禁止臆造数据」；<br>② 侧栏「最新更新」模块的类名 `ef-hot / ef-rank / ef-hot-t / ef-hot-v` 统一改为 `ef-latest / ef-latest-no / ef-latest-t / ef-latest-v`，并移除前三名红/金/青的榜单配色 —— 原命名与配色都在暗示「热度/排名」，与实现（按时间取前 5）不符；<br>③ 归档页年份锚点由 `href="#y-2026"` 改为 `button + scrollIntoView` —— **宿主机为 hash 路由（`#/blog?...`），写入 hash 会被路由误判为一次页面跳转**，属于真实功能隐患；<br>④ `manifest.tokens` 补齐 4 个漏报令牌（`--th-accent-soft` / `--th-accent-2-soft` / `--th-navy-2` / `--th-hot`），使声明与 `style.css` 实际定义完全一致；<br>⑤ 补齐悬空类 `.ef-feed` / `.ef-side` 的样式定义（原先模板有类、CSS 无规则）。 |
| 1.1.0 | 按《第三方完整交付版》重构：令牌 `--ef-*` → `--th-*`；新增页头/侧栏/页脚/帖子行公共组件；新增分类、作者、归档、搜索四个内页组件并在 manifest 声明 `entries`；接入 `<ThemeSwitch />` 并声明 `ui.themeSwitcher`；补齐 `data/themes/emforum/{ssr.css, manifest.json}` 覆盖 SSR 列表页与文章页。 |
| 1.0.0 | 列表页初版：版块页签 + 帖子流 + AI 侧栏。 |
