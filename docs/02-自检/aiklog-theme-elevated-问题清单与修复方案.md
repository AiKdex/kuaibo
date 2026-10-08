# AiKlog 主题「Elevated（高端暗色）」问题清单与修复方案

> 审查对象：`AiKlog/docs/aiklog-theme-elevated/`（ElevatedView.vue / style.css / manifest.js / index.js / README.md）
> 对照基准 ①：设计稿 `AiKlog/docs/aiklog-blog-theme.html`
> 对照基准 ②：AiKlog 真实主题插件协议 `web/src/themes/index.js` + `web/src/views/BlogView.vue` + 现有参考实现 `web/src/themes/minimal/`
> 审查时间：2026-09-14

---

## 0. 总体结论

- **视觉 DNA 一致**（玉色 #2dd4a8 系、暗色底、卡片/侧栏/AI 问答结构都对得上设计稿），但缺了顶部导航条、页脚、渐变标题、噪点层等关键元素，且尺寸整体缩了一号。
- **代码有 4 个协议级问题**：按现状安装进 AiKlog，主题**不会被注册 / 渲染出来是空白页 / 完全无样式**。必须先修协议问题，再谈样式还原。

问题分级：🔴 P0 = 装上就坏；🟠 P1 = 功能错误；🟡 P2 = 样式与设计稿不符。

---

## 一、🔴 P0 协议级问题（不修无法运行）

### P0-1 主题注册方式与真实协议不符 → 主题永远不注册

**现状**（`index.js`）：

```js
export default function registerElevated(app, { registerTheme }) {
  registerTheme(manifest.id, { ...manifest, component: () => import('./ElevatedView.vue') })
}
```

**真实协议**：BlogView 是 `import '@/themes/xxx'` 副作用式注册，`registerTheme(theme)` 直接调用；参考 `web/src/themes/minimal/index.js`：

```js
import MinimalView from './MinimalView.vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const theme = { ...manifest, entry: MinimalView }
registerTheme(theme)
export default theme
```

**修复方法**：改成同样的直接注册式。若想保留懒加载，可 `entry: defineAsyncComponent(() => import('./ElevatedView.vue'))`，但必须赋给 `entry` 而不是 `component`。

```js
import { defineAsyncComponent } from 'vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./ElevatedView.vue')),
}
registerTheme(theme)
export default theme
```

### P0-2 manifest 字段名不符合协议 → 注册被拒 / 切换条显示 undefined

**现状**（`manifest.js`）：用了 `name / nameZh / description / descriptionZh / thumbnail / features / slots`。

**真实协议**（`themes/index.js` 头部注释）：核心字段为 `id`、`title`（展示名）、`desc?`（一句话描述）、`version`、`entry`、`dataSources`、`dataScope`。BlogView 切换条渲染的是 `activeTheme.title` 和 `activeTheme.desc`。

**修复方法**：

```js
export default {
  id: 'elevated',
  title: '高端暗色 Elevated',
  desc: '暗色高端博客主题：玉色点缀、衬线正文、AI 侧栏',
  version: '1.0.0',
  dataSources: ['blog'],
  dataScope: 'shared',
}
```

补充说明：
- `dataScope: 'shared'` 是对的（只展示已分享文件），保留。
- `thumbnail: './thumb.png'` 引用的文件不在目录里（目录只有 5 个文件），要么补图，要么删掉该字段。
- `features / slots` 协议里没有，可作为扩展元数据保留但不依赖。

### P0-3 数据获取方式全错（props vs inject）→ 渲染出来是空白页

**现状**（`ElevatedView.vue`）：组件靠 **props** 接收 `posts / siteName / tagList / archives / comments / relatedPosts / totalPosts / totalPages / githubUrl ...` 共 17 个 props。

**真实协议**：BlogView 渲染主题时是：

```html
<component :is="activeTheme.entry" :key="activeTheme.id" />
```

**一个 props 都不传**，数据通过 `provide('themeContext', ctx)` 注入，主题应 `inject('themeContext')` 获取。参考 `minimal/MinimalView.vue`：

```js
const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], loading: true })
```

**修复方法**：删掉全部 props 声明，改用 inject，并处理 loading/error 空态：

```js
import { computed, inject } from 'vue'
const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], loading: true })
const posts = computed(() => ctx.value.posts || [])
```

模板中 `siteName` → `ctx.siteName`、`tagList` → `ctx.tags` 等。

### P0-4 数据字段名与真实契约不匹配 → 全部渲染为空

**现状**：模板/逻辑使用 `post.id`、`post.summary`、`post.cover`、`post.tags`、`post.readTime`、`post.views`、`post.comments`、`currentPost.contentHtml`、`archives`、`totalViews` 等。

**真实契约**（BlogView 注入）：posts 元素为

```
{ token, path, preview, created_at, file: { name, slug, updated_at } }
```

`id / summary / cover / readTime / views / comments / tags / contentHtml / archives / totalViews` **全都不存在**。字段派生方式参考 MinimalView：
- 标题：`p.file?.name` 去掉 `.md` 后取末段（或 `p.path`）
- 分类：`p.path` 的第一段
- 日期：`p.file?.updated_at || p.created_at`（毫秒时间戳兼容秒）
- 摘要：`p.preview`（建议加 clean 去 markdown 头部，minimal 有现成 `clean()`）
- 唯一键：`:key="p.token || i"`（没有 `id` 字段）
- 文章地址：`ctx.postUrl(p)`（生成根路径中文 slug，如 `/INSTALL`）

**修复方法**：所有取值处改为上述派生逻辑；不存在的能力（封面图、阅读时长、浏览数、评论、相关推荐、归档）分两类处理：
1. **砍掉**（对齐"精简发行"）或**降级隐藏**：`v-if` 条件渲染本已有，但数据源没有就永远不显示——干脆移除代码。
2. **要保留**：需另行扩展 `themeContext` 契约（后端加接口），属于后续迭代，不在本轮。

### P0-5 style.css 从未被引入 → 完全无样式

**现状**：`ElevatedView.vue` 没有 `import './style.css'`，也没有 `<style>` 块。MinimalView 的做法是 `import './style.css'`。

**修复方法**：在 `<script setup>` 顶部加 `import './style.css'`（若 index.js 用普通组件引入则亦可放在 index.js 中 import 一次）。

---

## 二、🟠 P1 逻辑问题

### P1-1 首篇文章重复渲染

**现状**：`featured = posts[0]`，同时 `v-for="post in posts"` 把 posts[0] 又渲染一遍 → 置顶文出现两次。

**修复方法**：

```js
const rest = computed(() => posts.value.slice(featured.value ? 1 : 0))
```

模板改为 `v-for="post in rest"`。另外文章列表页是分页场景的话，featured 应取当前页第一条而非全局第一条（与 BlogView 分页逻辑对齐；当前契约无分页，可先整页展示不做分页，`totalPages/currentPage` 相关代码移除）。

### P1-2 AI 问答：返回字段读错 + 未处理限流

**现状**：`askAI()` 读 `data.answer`。

**事实**：服务端 `POST /api/v1/public/blog/ask`（`public_blog_ai.go`）返回字段是 **`reply`**；该接口有速率限制（限流返回 429）。

**修复方法**：

```js
const data = await res.json()
this.aiAnswer = data.reply || data.answer || '暂无回答'
```

并加 `if (res.status === 429) { this.aiAnswer = '提问太频繁，请 5 分钟后再试'; return }`。
顺带把侧栏第三条特性「限流保护，每 5 分钟 8 次」补回（设计稿有）。

### P1-3 文章打开方式与 SSR/SEO 模型冲突

**现状**：组件内置 SPA 文章视图，渲染 `currentPost.contentHtml`（该字段不存在），配内部 `back-btn` 返回。

**事实**：AiKlog 的对外文章页是**服务端渲染**（`GET /{slug}` / `/blog/{slug}`），SEO 双轨的根基；契约提供 `postUrl(p)` 生成真实地址。SPA 内渲染拿不到正文 HTML，且制造第二套非 SSR 的文章视图。

**修复方法**：`openPost` 改为跳转 SSR 页，删除内部 article 视图与 back-btn：

```js
function openPost(p) {
  const url = ctx.value.postUrl ? ctx.value.postUrl(p) : `#/p/${p.token}`
  window.location.href = url   // 或 <a :href> 语义化链接（更推荐，见 P2-1）
}
```

**推荐做法**：卡片外层直接用 `<a :href="hrefOf(p)">` 包裹（像 MinimalView），保留 hover 动效，天然支持中键/Ctrl+点击新开标签。
AI 摘要、评论区随内部文章视图一并移除（SSR 页已有正文；评论/AI 摘要如要保留需后续走 SSR 页扩展）。

### P1-4 manifest.thumbnail 指向不存在的文件

**修复方法**：补一张 `thumb.png`（建议 640×400 暗色缩略图），或先删字段。主题切换 UI 若展示缩略图会因此裂图。

---

## 三、🟡 P2 样式与设计稿不符清单

| # | 设计稿元素 | 主题现状 | 修复方法 |
|---|---|---|---|
| P2-1 | **顶部导航条**（sticky top-bar：logo「● AiKlog 爱库录」+ 博客/归档/标签/关于 + ⌘K 搜索框 + 🌙 切换） | **完全没有** | 在组件顶部补 `.el-topbar` 区块，样式从设计稿照搬（背景 rgba(11,15,26,0.85) + backdrop-filter blur(24px)）；搜索按钮可先触发 `filterTag` 式过滤或留占位 |
| P2-2 | **页脚**（品牌行 + 首页/博客/归档/RSS/GitHub 链接 + 版权行「Powered by Go + SQLite + AI」） | **完全没有** | 补 `.el-footer`，链接接真实地址：博客 `/blog`、RSS `/api/v1/blog/feed.xml`、GitHub/文档待定 URL |
| P2-3 | **hero 渐变大标题**：两行「目录即站点 / 文件即文章」，白→灰渐变字（`background-clip: text`） | h1 是站名纯白 #fff | 恢复设计稿写法：主标题固定文案+渐变字，站名可挪到 top-bar；CSS 补 `-webkit-background-clip:text; -webkit-text-fill-color:transparent; background:linear-gradient(135deg,var(--t),var(--t2))` |
| P2-4 | **背景噪点层** `.bg-noise`（SVG fractalNoise，opacity 0.3） | 只有 `.bg-ambient` | 照搬设计稿的 data-uri SVG 噪点层 |
| P2-5 | **卡片图占位分类渐变底色**（如 AI：`#2a1a3a→#1a0d33`；IM：`#1a2a1a→#0d330d`…） | 纯色 `--ink3` + emoji | `getEmoji()` 扩展为返回 `{emoji, gradient}`，占位 div 内联渐变背景 |
| P2-6 | **尺寸整体缩一号**：容器 1200→1100、侧栏 300→280、卡片图列 260→240、卡片 padding/gap 28→24、卡片标题 20→18px、featured 标题 28→24、文章 h1 40→36、正文衬线 17→16、h2 26→24、h3 20→19、头像 72→64、侧栏 padding 24→20、分页按钮 38→36 | —— | 逐项校回设计稿数值（上表左=设计稿值） |
| P2-7 | AI 侧栏三条特性（含「限流保护，每 5 分钟 8 次」） | 只有两条 | 补第三条 |
| P2-8 | 设计稿变量 `--jade-subtle: rgba(45,212,168,0.06)` | `--js` 为 0.05 | 微调对齐（影响极小） |
| P2-9 | 全局 reset（`*{margin:0;padding:0;box-sizing:border-box}`）与全局 `a` 色 | 无 reset | 在 `.elevated-root` 内加作用域版 reset：`.elevated-root *, .elevated-root *::before { margin:0; padding:0; box-sizing:border-box }` |

✅ **已一致、无需动的**：玉色系全部色值、文字灰阶、边框色、圆角体系、卡片/侧栏/AI 框/标签云/归档/FAB 的视觉语言、响应式断点（900px/600px）策略、`::selection`。

⚠️ **主题样式作用域**：协议要求样式隔离（类名前缀或 scoped）。主题用 `.elevated-root` 包裹 + 类前缀 `el-`，基本合规；CSS 变量声明在 `.elevated-root` 上而非 `:root`，不会污染主应用，可保留（协议建议 `--th-*` 前缀，如追求严格合规可批量改前缀，非必须）。

---

## 四、建议修复顺序与验收清单

**顺序**：P0-1/2（注册）→ P0-3/4（数据）→ P0-5（样式引入）→ P1-1/2/3（逻辑）→ P2（样式还原，按 P2-1→2→3→4→5→6→7→9→8）。

**验收清单**：
1. `npm run build`（web 目录）无报错；控制台无 `[themes] 主题缺少 entry` 警告。
2. 后台站点设置把主题切到 elevated → `?view=public` 读者视图正常显示文章列表（字段有值：标题/日期/摘要/分类）。
3. 列表无重复首篇；点卡片跳转到 SSR 文章页（`/{slug}`）且能打开。
4. AI 侧栏输入问题能返回回答（`reply` 字段）；连点触发 429 有友好提示。
5. 与 `aiklog-blog-theme.html` 并排对比：top-bar / 渐变 hero / 噪点 / 页脚 / 分类渐变占位 / 尺寸一致。
6. 移动端（<900px）侧栏隐藏、卡片单列正常。
7. 切回 default/minimal 主题不受污染（样式隔离验证）。

---

---

## 五、修订版审查：docs/(2) 与 docs/(3) 两套主题（23:22 补审）

用户随后提供了两套新代码：`docs/(2)/`（Elevated 修订版）与 `docs/(3)/`（Parchment 暖纸杂志，全新主题）。

### 5.1 docs/(2) Elevated 修订版 —— 协议层全部修复 ✅

对照第一部分清单逐项核验：

| 原问题 | 状态 |
|---|---|
| P0-1 注册方式 | ✅ 已改为直接 `registerTheme(theme)` + `defineAsyncComponent` |
| P0-2 manifest 字段 | ✅ 已改 `title/desc`，删掉不存在的 thumbnail |
| P0-3 props→inject | ✅ 已用 `inject('themeContext')` |
| P0-4 数据字段 | ✅ 全部换成真实契约（token/path/file.name/preview/updated_at），posts 用 token 作 key |
| P0-5 style.css 引入 | ✅ 已 import |
| P1-1 首篇重复 | ✅ featured + restPosts(slice) 去重 |
| P1-2 AI 问答 | ✅ 读 `reply` + 处理 429 |
| P1-3 文章打开方式 | ✅ 改 `<a :href="postUrl()">` 跳 SSR 页，删内部文章视图 |
| P2-1~7 样式还原 | ✅ topbar / footer / 噪点层 / 渐变 hero（「文章」二字玉色渐变是合理增强）/ 分类渐变占位 / 三条 AI 特性 / 尺寸校回 1200/300/260/28 |

**剩余问题（3 个）**：

1. 🔴 **reset 选择器写漏**（style.css 第 7-13 行）：只写了 `.elevated-root`、`.elevated-root *::before`、`.elevated-root *::after`，**漏了 `.elevated-root *` 本身**——后代元素的 UA 默认 margin/padding 没被清掉。修复：
   ```css
   .elevated-root,
   .elevated-root *,
   .elevated-root *::before,
   .elevated-root *::after { margin:0; padding:0; box-sizing:border-box; }
   ```
2. 🟠 **标签云永远不显示 / 标签链接是死链**：`ctx.tags` 在 BlogView 中**从未被填充**（恒为 `[]`，`v-if="tags.length"` 永假）；且协议定义是 `Array<{name, count}>` 对象，将来填充后 `{{ t }}` 会渲染成 `[object Object]`，应写 `t.name`。另外 `/blog?tag=xxx` 链接：PublicHome **不处理 `?tag=` query**，点击等于回博客首页。
   → 修复：`{{ t.name ?? t }}` 兼容写法；`?tag=` 能力需后端/BlogView 支持后再启用，或先去掉链接只留展示。
3. 🟡 小项：`getTitle` 只去 `.md` 不去 `.markdown`（`.replace(/\.(md|markdown)$/i, ...)`）；`ctx.blogUrl` 不在协议字段里（fallback '/blog' 可用，标注即可）；topbar 无「归档/关于」与 ⌘K 搜索（设计稿有，属可接受裁剪）。

### 5.2 docs/(3) Parchment 暖纸杂志（新主题）—— 协议层同样全对 ✅

注册/manifest/inject/数据契约/style.css 引入全部符合协议，双列网格 + 琥珀强调 + 衬线排版视觉自洽，与 (2) 同一质量水准。**剩余问题（4 个）**：

1. 🟠 **暗色渐变贴亮纸**：`gradientMap` 里 Docker/AI/IM/Go/安全 沿用了 Elevated 的**暗色系渐变**（`#1a3a4a→#0d2233` 等），放在 `#faf8f5` 暖纸底上会出现突兀暗块。应换低饱和暖色系分类渐变（琥珀/陶土/橄榄/雾蓝），如：
   ```js
   'Docker': 'linear-gradient(135deg,#dbeafe,#bfdbfe)',
   'AI':     'linear-gradient(135deg,#fce7f3,#fbcfe8)',
   'IM':     'linear-gradient(135deg,#dcfce7,#bbf7d0)',
   'Go':     'linear-gradient(135deg,#e0e7ff,#c7d2fe)',
   '安全':   'linear-gradient(135deg,#fee2e2,#fecaca)',
   ```
2. 🟠 **约 350 行死 CSS**：`.pa-pg`（分页）与 `.pa-art-*` 全套文章页样式（header/cover/body/ai-sum/tags/related/comments，style.css 第 261-853 行的大部分）在组件里**没有对应模板**（文章页已改为跳 SSR）——不影响运行，但建议整体删除，避免误导后续维护。
3. 🟠 **标签云问题与 (2) 完全相同**：`ctx.tags` 恒为空 + `{{ t }}` 对象渲染隐患 + `?tag=` 死链。
4. 🟡 小项：header 用 `<h1>` 放站名（一页多个 h1 语义问题，建议降级 div）；无 reset（列表元素大多显式 margin，风险低于 (2)，仍建议补作用域 reset）；`.pa-feat` 自己又写了一遍 `max-width:1100px + padding`，与 `.pa-wrap` 容器双轨（可用，统一更佳）。

### 5.3 两套共同问题：tags 能力未兑现

两套主题的标签云都依赖 `ctx.tags`，但 BlogView 从未填充该字段——这是**契约声明了却没实现**的能力。两条路：
- **快**：主题端先隐藏标签云（或留 `t.name ?? t` 兼容写法等后端兑现）；
- **根治**：BlogView `loadBlogContext` 从 posts 派生 tags（`path` 首段聚合 + 计数），或后端提供公开标签接口——这属于系统层改动，建议单独立项。

### 5.4 结论

- **(2) Elevated 修订版**：修 1 个 CSS 选择器 + 标签兼容处理后即可装机，样式还原度高。
- **(3) Parchment**：修暗色渐变 + 删死 CSS + 标签处理后可用，作为第二套官方主题（暗/亮双主题）正合适。
- 均无 P0 级协议问题，比第一版质量高一个档次。

## 附：涉及文件

- `docs/aiklog-theme-elevated/index.js` — P0-1、P0-2
- `docs/aiklog-theme-elevated/manifest.js` — P0-2、P1-4
- `docs/aiklog-theme-elevated/ElevatedView.vue` — P0-3、P0-4、P1-1、P1-2、P1-3、P2-1~5、P2-7
- `docs/aiklog-theme-elevated/style.css` — P0-5（引入）、P2-3/4/6/8/9
- `docs/(2)/`（Elevated 修订版）— §5.1：style.css reset 选择器、ElevatedView.vue 标签写法
- `docs/(3)/`（Parchment）— §5.2：ParchmentView.vue 渐变表/标签写法、style.css 死代码清理
- 参考：`web/src/themes/minimal/`（数据契约的标准用法）、`web/src/themes/index.js`（协议定义）、`web/src/views/BlogView.vue`（注入与渲染方式；tags 未填充）
