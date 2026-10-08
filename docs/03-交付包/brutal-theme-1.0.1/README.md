# 新粗野 Brutal —— AiKlog 博客主题交付包

| 项 | 值 |
|---|---|
| 主题 id | `brutal` |
| 主题名 | 新粗野 Brutal |
| 版本 | `1.0.1` |
| 本版性质 | **自检修复版**：修复令牌台账漏报 5 项、悬空类 1 处、文案不一致 1 处（详见 §七） |
| 适配 | AiKlog 交互版博客（SPA 主题协议 v1 + SSR 外置样式） |
| 规范依据 | 《博客主题开发规范（第三方完整交付版）》v1.0 |
| 风格 | 新粗野主义（Neubrutalism）：硬边框 + 硬阴影 + 高饱和撞色 + 零圆角 |

---

## 一、交付清单

```
brutal-theme-1.0.1/
├── README.md                                  ← 本文件
├── web/src/themes/brutal/                     ← SPA 主题（放入 web 源码树后参与构建）
│   ├── manifest.js                            主题元数据（§1.1 字段）
│   ├── index.js                               模块加载即 registerTheme
│   ├── helpers.js                             契约字段派生层（标题/分类/日期/大小/相对时间）
│   ├── BrutalView.vue                         列表页主入口（Hero + 头条 + 筛选 + 网格 + 分页）
│   ├── BrutalHead.vue                         页头（跑马灯 + 品牌区 + 导航 + ThemeSwitch）
│   ├── BrutalCard.vue                         文章卡片（列表/分类/作者/搜索共用）
│   ├── BrutalSide.vue                         侧栏区（分类目录 / 最近更新 / AI 问答 / 标签）
│   ├── BrutalFoot.vue                         页脚
│   ├── BrutalCatView.vue                      内页 · 分类（entries.cat / category）
│   ├── BrutalAuthorView.vue                   内页 · 作者（entries.author）
│   ├── BrutalArchiveView.vue                  内页 · 归档（entries.archive）
│   ├── BrutalSearchView.vue                   内页 · 搜索（entries.search）
│   └── style.css                              全部样式（令牌 --th-*，选择器限定 .brutal-root）
├── server/data/themes/brutal/                 ← SSR 外置样式（**热生效，无需重编译**）
│   ├── ssr.css                                列表页 /blog 与文章页 /{slug} 共用
│   └── manifest.json                          外置主题元数据（title / version）
├── tools/
│   └── theme-lint.sh                          交付自检脚本（悬空类 / 令牌台账 / hash 锚点 / 高危文案词）
└── design/
    └── design-brutal-list-and-post.html       设计稿对照（列表页 + 文章页，浏览器直接打开）
```

---

## 二、安装步骤（四步）

### ① 前端主题模块
```bash
# 拷贝主题目录
cp -r web/src/themes/brutal  <AiKlog>/web/src/themes/brutal

# 在公开视图注册（BlogView.vue 的 import 区，追加一行）
#   import '@/themes/brutal' // 注册新粗野主题（硬边框硬阴影，高饱和撞色）
```
注册后控制台的博客主题下拉会自动出现「新粗野 Brutal」。

### ② 构建前端
```bash
cd <AiKlog>/web && npm run build
# 产出 dist/ 后按项目既有流程同步到 webdist（通常 cp -r dist/* <部署目录>/webdist/）
```

### ③ 投放 SSR 外置样式（**关键，缺这步文章页不跟随主题**）
```bash
scp server/data/themes/brutal/ssr.css   root@<host>:/opt/aiklog/data/themes/brutal/ssr.css
scp server/data/themes/brutal/manifest.json root@<host>:/opt/aiklog/data/themes/brutal/manifest.json
```
`data/themes/<id>/ssr.css` 按文件 mtime 缓存，**投放后立即生效，不需要重编译、不需要重启**。

### ④ 确认白名单
`server/internal/handler/settings.go` 中 `blog.theme` 的 switch 分支需包含 `brutal`：

```go
case "", "default", "aiklog", "minimal", "docs", "paper", "elevated", "parchment", "emforum", "brutal":
```
漏掉该项时，控制台切换会返回 `SETTING_INVALID`。

### 切换与预览
- 控制台 → 站点设置 → 博客主题 → 选择「新粗野 Brutal」
- 或接口：`PUT /api/v1/settings  {"key":"blog.theme","value":"brutal"}`
- 预览：`/blog?theme=brutal`

> ⚠️ 覆盖服务端二进制前必须先 `pkill -x aiklog`，否则会出现 `Text file busy` 或继续跑旧二进制。

---

## 三、契约遵循说明

| 契约点 | 本主题实现 |
|---|---|
| 数据获取 | 全部 `inject('themeContext')`，兼容 `raw?.value \|\| raw`；组件**不接收 props 传数据** |
| 使用字段 | 仅 `token / path / preview / file.{name,slug,updated_at,size}`，**未使用任何臆造字段** |
| 未使用的字段 | 无 `title/cover/author/created_at/views` —— 卡片封面为**纯 CSS 生成几何图案**（契约无封面图字段） |
| 标题派生 | `file.name`（兼容 `.md` / `.markdown`）→ 取末段，缺省「无标题」 |
| 分类派生 | `path` 首段；无 `/` 归入「未分类」，展示层统一补名 |
| 日期 | `file.updated_at`（兼容毫秒 / 秒级 / 字符串） |
| 卡片副信息 | 文件大小（真实字段）+ 相对时间，**不显示浏览量/点赞数等不存在的数据** |
| 链接 | 一律 `ctx.postUrl(post)` 跳 SSR 文章页；**未做 SPA 文章视图**、未拼 `#/p/` |
| 排序 | 默认保持系统顺序（置顶优先 + 时间倒序）；仅当读者显式选择排序项才重排 |
| 标签 | `v-if="tags.length"` + 非链接 `<span>`，tags 恒空时自动隐藏，不产生 `?tag=` 死链 |
| AI 问答 | `POST /api/v1/public/blog/ask`，读 `reply` 字段，429 给出友好提示，空输入时按钮禁用 |
| 三态 | loading / error / 空态齐全（列表页、内页均有） |
| 站名语义 | 站名用 `<div>`，不占用 `<h1>`；区块标题用 `<h2>` |
| 主题切换器 | 页头渲染 `<ThemeSwitch />`，manifest 声明 `ui.themeSwitcher: true`，宿主不再补悬浮版 |
| 内页参数 | 优先 `ctx.page.{cat,author,archive,q}`，系统未注入时回退解析 SPA hash，保证当前版本可预览 |
| SSR 侧 | 仅覆盖附录 A 既有钩子，**未改动任何 Go 源码** |

### 内页 entries 声明
```js
entries: { cat, category, author, archive, search }
```
`cat` 与 `category` 两个键同时声明，兼容规范 §3.1 与 §4.10 的命名差异。`post` 与 `tag` 按规范不接管。

---

## 四、设计令牌（`--th-*`）

| 令牌 | 色值 | 用途 |
|---|---|---|
| `--th-ink` | `#101010` | 正文色（近黑）· 同时是描边与硬阴影色 |
| `--th-ink-2` | `#3a3a3a` | 次要文字 |
| `--th-ink-3` | `#6b6b6b` | 弱化文字（日期、计数） |
| `--th-paper` | `#fffbf0` | 页面底色（米白，叠加 24px 点阵纹理） |
| `--th-card` | `#ffffff` | 卡片底色 |
| `--th-line` | `#101010` | 描边 / 分隔线（纯黑，不做半透明淡化） |
| `--th-accent` | `#ffe04b` | 主强调（明黄）：主按钮、分类 c1、强调条 |
| `--th-accent-2` | `#ff6e9c` | 次强调（亮粉）：Hero 贴纸字、分类 c2 |
| `--th-accent-3` | `#4de3a2` | 状态色（薄荷）：分类 c3、圆形贴纸 |
| `--th-deco-1` | `#8b7bff` | 装饰（电光紫）：拼贴大卡、分类 c4 |
| `--th-deco-2` | `#57b8ff` | 装饰（天蓝）：分类 c5、图案 c |
| `--th-deco-3` | `#ff8a3d` | 装饰（橙）：图案 d |
| `--th-font-d` | Archivo Black / Arial Black / 思源黑体 / 苹方 / 雅黑 | 标题（超粗无衬线） |
| `--th-font-b` | Space Grotesk / Inter / 苹方 / 雅黑 / system-ui | 正文 |

**风格公式**：`3px 纯黑边框 + 6px 零模糊硬阴影 + border-radius: 0`；
悬停统一「位移 4–6px + 阴影归零」，制造物理按压感。

### 字体说明
未引入任何外部字体（无 CDN 依赖）。英文标题使用系统 `Arial Black`，中文依赖 `font-weight: 900` + 负字距撑体量；
**中文一律不使用 `-webkit-text-stroke`**（笔画会粘连糊成一团），描边仅用于英文大标题与数字。

---

## 五、自测记录

| 检查项 | 结果 |
|---|---|
| `vite build` 编译 | ✅ 通过，零错误（207 模块 transform 成功） |
| 产物 chunk | ✅ `BrutalView 8.78 kB` · `BrutalCatView 2.65` · `BrutalSearchView 2.95` · `BrutalArchiveView 2.40` · `BrutalAuthorView 2.24`（内页均为懒加载独立 chunk） |
| console 无主题告警 | ✅ 无 `[themes] 主题缺少 entry/id` |
| 三态齐全 | ✅ loading / error / 空态（含搜索无结果态） |
| 首篇不重复 | ✅ 精选区取 `posts[0]`，网格池从 `slice(1)` 开始；仅首页 + 默认排序 + 无关键词时展示精选 |
| 点击跳转 | ✅ 全部 `ctx.postUrl(post)` 跳 SSR 页，无 SPA 内渲染 |
| 标签云 | ✅ tags 恒空 → 整块 `v-if` 隐藏，非链接渲染 |
| AI 问答 | ✅ 读 `reply`、429 提示、空输入禁用按钮、请求中显示「思考中…」 |
| 样式隔离 | ✅ 所有选择器挂 `.brutal-root`，令牌统一 `--th-*`，无全局污染 |
| 移动端 | ✅ 1024px / 720px 两档断点（网格 3→2→1 列、侧栏 3→2→1 列、精选转上下堆叠） |
| 死 CSS | ✅ 无（每个类名均在模板中使用） |
| SSR 钩子 | ✅ 仅使用附录 A 列出的既有类名与变量 |

---

## 六、已知限制（均属系统侧，主题已做优雅降级）

| 限制 | 主题侧现状 |
|---|---|
| `ctx.page` 未注入 | 内页参数自动回退解析 hash（`#/blog/cat/xxx`、`?q=xxx`），当前版本即可预览 |
| `ctx.tags` 恒空 | 标签云整块自动隐藏，系统补齐后无需改代码即可显示 |
| `file.author` 未透出 | 作者页显示明确说明卡片而非伪造作者数据；系统透出后本页自动按作者聚合 |
| SPA 内页路由未上线 | `entries` 已按规范声明（懒加载），系统补齐路由后自动生效 |
| 归档页无月份锚点 | 现按年份分组（`updated_at` 派生），如需月份级可后续迭代 |

---

## 七、版本记录

| 版本 | 日期 | 变更 |
|---|---|---|
| `1.0.1` | 2026-09-15 | **自检修复版**，3 处：<br>① `manifest.tokens` 补齐 5 项漏报（`--th-bw` / `--th-bw-2` / `--th-sh` / `--th-sh-lg` / `--th-sh-sm`，均为硬边框与硬阴影尺寸变量）——声明与 `style.css` 现逐项一致；<br>② 补齐悬空类 `.bt-hero-main` 的规则（`min-width: 0`，防 hero 左栏长标题/搜索框撑破 grid 列宽）；<br>③ 头条区文案由「本期精选 / Editor's pick / Featured」改为「**头条 / Top story**」——「精选」宣称编辑挑选（契约无法证明），「头条」描述版面位置（= `posts[0]`，可证），设计稿同步更新。<br>另：随包附 `tools/theme-lint.sh` 交付自检脚本。 |
| `1.0.0` | 2026-09-15 | 首版：列表页 + 4 个内页 + 外置 SSR 样式；新粗野风完整落地 |
