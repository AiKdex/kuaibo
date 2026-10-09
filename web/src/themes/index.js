/**
 * 博客主题插件协议 + 注册表（Blog Theme Protocol）—— AiKlog（爱库录）博客插件化基础。
 *
 * 目标：把对外博客（/blog）的展示形态以「主题」为单位解耦、可注册、可切换；
 * 第三方（含 E03「博客插件化」方向）复用同一套协议扩展主题，无需改动 BlogView 核心。
 *
 * ── 协议（主题对象）─────────────────────────────────────────────
 * {
 *   id: string,                 // 唯一标识（kebab-case）
 *   title: string,              // 展示名
 *   desc?: string,              // 一句话描述
 *   version: string,            // 语义化版本
 *   entry: Component,           // 主题入口组件（Vue SFC，渲染整页内容）
 *   pages?: Array<string>,      // 主题提供的页面标识（如 ['jobs','training']）
 *   tokens?: Object,            // 设计令牌说明（主题 CSS 变量，约定 --th-* 前缀防污染）
 *   dataSources?: Array<string> // 数据来源声明，决定主题如何取数：
 *                               //   'blog'   → 通用博客内容（推荐）：BlogView 统一注入
 *                               //              themeContext（siteName/posts/tags/postUrl…），
 *                               //              主题只管样式与排版，安装即显示博客分享内容
 *                               //   'collect'→ 采集型视图（特殊场景，如求智岗位情报）：
 *                               //              主题自带数据提供者或对接采集 API，非通用
 *   ui?: { themeSwitcher?: boolean } // 主题自己在页头渲染了 <ThemeSwitch>（inline 模式）。
 *                               //   置 true 后宿主不再渲染右上角悬浮切换器，避免重复。
 *                               //   不声明的主题，宿主自动在公开页右上角补一个悬浮切换器。
 *   entries?: Object            // 页面级入口（见「页面契约」）：{ post, tag, category, archive, search }
 *                               //   主题只需声明自己要接管的页面，未声明的走宿主默认页面 + 主题 tokens
 *                               //   post 页宿主默认为 BlogPostView（/app#/post/{token}）；
 *                               //   主题声明 entries.post 后整页由主题渲染，从 ctx.page.post 取
 *                               //   { token, path, slug, title, html, cat, category, author, updatedAt, size }
 *                               //   （html 为系统渲染好的正文 HTML，用 v-html 输出）
 *   dataScope?: 'shared'|'library' // 数据可见范围（隐私边界）：
 *                               //   'shared' （默认，安全）→ 主题只展示「已分享」的文件
 *                               //              集合——系统层默认；公开部署博客时不泄露私有文件
 *                               //   'library'（自用特例）→ 主题直读文件库（如采集目录）。
 *                               //              仅限单机/内网自用视图，必须显式声明，
 *                               //              不得伪装为通用博客主题对外公开
 * }
 *
 * ── 通用数据契约（dataSources 含 'blog' 的主题）────────────────
 * BlogView 通过 provide('themeContext', …) 注入：
 * {
 *   siteName: string,           // 站点名（默认「爱库录」）
 *   siteDesc: string,           // 站点描述
 *   posts: Array<{ token, path, preview, file:{ name, slug, size, updated_at, kind, mime } }>, // 公开文章列表
 *   tags: Array<{ name, count }>,  // 公开文章标签（带计数；由 GET /api/v1/public/tags 聚合）
 *   postUrl(post): string,         // 单篇地址生成器 → SPA 文章页（#/post/{token}?path=&slug=）
 *   postSsrUrl(post): string,      // 单篇 SSR 静态页地址（/{slug}）——用于「分享链接」与 SEO
 *   page?: { kind, post, slug, path, cat, tag, archive, q }, // 页面参数（内页/文章页）
 *   loading: boolean, error: string // 加载状态（主题可展示空态/错误态）
 * }
 * 主题组件用 inject('themeContext') 接收；字段可选，主题按需使用。
 * 这样任何符合协议的主题「安装即用」，不需要自己对接数据。
 *
 * ── 页面契约（entries）──────────────────────────────────────────
 * 一个主题不只渲染首页列表，还要对「文章页 / 标签页 / 分类页 / 归档页 / 搜索页」负责。
 * 主题可声明 entries 接管其中部分页面（宿主优先用主题入口，未声明的走宿主默认页面，
 * 但统一套用主题的 --th-* 令牌，保证整站气质一致）：
 *   entries: {
 *     list?:     Component,  // 列表页（默认 = theme.entry）
 *     post?:     Component,  // 文章详情页（SPA /app#/p/{token}）
 *     tag?:      Component,  // 标签聚合页（?tag=xxx）
 *     category?: Component,  // 分类页（?cat=xxx，分类 = 目录层级）
 *     archive?:  Component,  // 归档页（?archive=YYYY-MM）
 *     search?:   Component,  // 搜索结果页（?q=xxx）
 *   }
 * 约定：页面组件同样用 inject('themeContext') 取数，额外从 ctx.page 读当前页参数
 * { kind:'post'|'tag'|…, tag, cat, q, archive, post }。
 * 未提供 entries.post 的主题，宿主 BlogPostView 用系统默认版式渲染正文，并挂载主题 class
 * （.th-<id>）与 --th-* 变量 —— 至少保证配色/字体一致；要"点进文章也是主题版式"就声明
 * entries.post（正文 HTML 由 ctx.page.post.html 提供，主题不负责正文渲染本身）。
 *
 * 说明：文章页双轨 —— 人看 SPA（#/post/{token}，随主题、带评论/侧栏），
 * 机器看 SSR（/{slug}，canonical/sitemap/分享快照），两者由同一份数据渲染。
 *
 * 说明：
 * - 主题通过 registerTheme 注册；BlogView 按 getActiveTheme() 动态渲染 <component :is="theme.entry">。
 * - 激活主题持久化于 localStorage（key: aikmap.blog.theme）；默认 'default'（内置文章列表）。
 * - 主题样式必须作用域隔离：Vue scoped style 或类名前缀，变量统一 --th-* 前缀，避免污染主应用。
 * - 单项主题抛错不影响其它主题与主应用（切换处 try-catch 隔离）。
 */

import { ref } from 'vue'
import { t } from '@/i18n'
import { THEME_LOADERS, themeCatalog } from './catalog.js'

// 管理端/切换器需要全量主题元数据（含未安装项）→ 转出口径
export { themeCatalog }

const STORAGE_KEY = 'aikmap.blog.theme' // 站点设置下发的主题（服务端权威值缓存）
const OVERRIDE_KEY = 'aiklog.theme.override' // 访客/站长在公开页手动选的主题（本地优先）

/** 主题注册表：id -> theme */
const registry = new Map()

/** 注册表版本号（响应式）：懒加载主题注册完成后自增，驱动 listThemes() 的 computed 重算 */
export const registryVersion = ref(0)

/** 已安装主题 id 集合（后端 blog_plugins kind=theme 驱动；null=尚未拉取，视为全部可用以降级） */
export const installedThemeIds = ref(null)

/** 当前生效主题 id（响应式：切换后所有组件同步） */
export const activeThemeId = ref(readStoredId())
/** 站点设置下发的主题 id（服务端权威） */
/** 是否允许访客自由切换主题（per-site site_settings.allow_visitor_theme_switch；默认允许） */
export const allowVisitorSwitch = ref(true)
export const serverThemeId = ref('')

/**
 * 注册主题。id 冲突时后注册覆盖（便于热替换/调试）。
 */
export function registerTheme(theme) {
  if (!theme || !theme.id) {
    console.warn('[themes] 主题缺少 id，忽略注册:', theme)
    return
  }
  // 内置 default 主题由 BlogView 内部渲染文章列表，允许 entry 为空；
  // 其余主题必须有入口组件，否则无法渲染
  if (theme.id !== 'default' && !theme.entry) {
    console.warn('[themes] 主题缺少 entry，忽略注册:', theme)
    return
  }
  registry.set(theme.id, theme)
  registryVersion.value++
}

/**
 * 列出全部已注册主题（默认主题排最前）。
 * registryVersion 读取建立响应式依赖：懒加载主题注册后调用方 computed 自动重算。
 */
export function listThemes() {
  registryVersion.value // 响应式依赖（见上）
  const arr = []
  if (registry.has('default')) arr.push(registry.get('default'))
  // 参数名避开 t：防止遮蔽 i18n 的 t（B23/B24 同款坑）
  registry.forEach((th, id) => { if (id !== 'default') arr.push(th) })
  return arr
}

// ============ 懒加载主题机制（应用中心按需安装，2026-10-09） ============
// 懒主题经 catalog.js 的 THEME_LOADERS 动态 import（Vite 独立 chunk），
// 其 index.js 副作用 registerTheme 后进入 registry；未加载前 listThemes() 不含它们。

/**
 * 确保主题已注册：已注册直接返回 true；在懒加载表中则动态加载并等注册完成；
 * 两者皆否（未知 id / 加载失败）返回 false。加载失败自动回落默认主题。
 */
export async function ensureTheme(id) {
  if (!id || registry.has(id)) return registry.has(id)
  let load = null
  try { load = (THEME_LOADERS[id]) } catch (e) { /* catalog 未引入 */ }
  if (!load) return false
  try {
    await load()
  } catch (e) {
    console.warn('[themes] 主题动态加载失败，回落默认:', id, e)
    return false
  }
  return registry.has(id)
}

/** 拉取到「已安装主题列表」后调用：驱动切换器/列表只显示已安装主题。 */
export function applyInstalledThemes(ids) {
  installedThemeIds.value = Array.isArray(ids) ? ids : []
}

/** 主题是否已安装（未拉取过列表时返回 true —— 降级为老的全量可见行为）。 */
export function isThemeInstalled(id) {
  if (!id || id === 'default' || id === 'aiklog') return true
  if (!Array.isArray(installedThemeIds.value)) return true
  return installedThemeIds.value.includes(id)
}

/**
 * 取当前激活主题；未注册/未知 id 回退默认。
 */
export function getActiveTheme() {
  if (registry.has(activeThemeId.value)) return registry.get(activeThemeId.value)
  return registry.get('default') || null
}

/**
 * 用户手动选择主题（公开页切换器 / 控制台下拉）。
 * 写入 override，优先级高于站点设置，刷新后保持。
 */
export function setActiveTheme(id) {
  if (id && registry.has(id)) {
    try {
      localStorage.setItem(OVERRIDE_KEY, id)
    } catch (e) { /* 隐私模式忽略 */ }
    activeThemeId.value = id
    return registry.get(id)
  }
  // id 为空 = 恢复「跟随站点设置」
  try {
    localStorage.removeItem(OVERRIDE_KEY)
  } catch (e) { /* ignore */ }
  activeThemeId.value = serverThemeId.value || 'default'
  return getActiveTheme()
}

/**
 * 站点设置下发主题（服务端权威）。已有用户本地选择时不覆盖。
 */
export function applyServerTheme(id, opts = {}) {
  const sid = id || 'default'
  serverThemeId.value = sid
  // 访客切换开关（per-site site_settings.allow_visitor_theme_switch）：
  // false = 关闭，强制使用站点默认主题、忽略访客本地覆盖（SPEC-MS-001 M1）。
  const allowSwitch = opts.allowVisitorSwitch !== false
  allowVisitorSwitch.value = allowSwitch
  try {
    localStorage.setItem(STORAGE_KEY, sid)
    if (opts.force) localStorage.removeItem(OVERRIDE_KEY) // 站长刚改了站点主题 → 丢弃本地覆盖
  } catch (e) { /* ignore */ }
  if (!allowSwitch) {
    // 关闭访客切换：忽略本地覆盖，统一使用站点默认主题
    activeThemeId.value = sid
    return getActiveTheme()
  }
  const ov = readOverride()
  activeThemeId.value = ov || sid
  return getActiveTheme()
}

/** 当前是否存在用户本地覆盖（用于显示「恢复站点默认」） */
export function readOverride() {
  try {
    return localStorage.getItem(OVERRIDE_KEY) || ''
  } catch (e) {
    return ''
  }
}

/**
 * 取主题在某页面上的入口组件；未声明返回 null（宿主渲染默认页面）。
 * page: 'list' | 'post' | 'tag' | 'category' | 'archive' | 'search'
 */
export function pageEntry(theme, page) {
  if (!theme) return null
  if (page === 'list') return theme.entry || null
  const e = theme.entries || {}
  // 页面键别名：规范正文用 cat，早期协议枚举用 category，两者互通（主题只需声明其一）
  for (const k of PAGE_ALIAS[page] || [page]) {
    if (e[k]) return e[k]
  }
  return null
}

/** 页面键别名表（cat ↔ category 双向兼容） */
const PAGE_ALIAS = { cat: ['cat', 'category'], category: ['category', 'cat'] }

function readStoredId() {
  try {
    return localStorage.getItem(OVERRIDE_KEY) || localStorage.getItem(STORAGE_KEY) || 'default'
  } catch (e) {
    return 'default'
  }
}

// ============ 内置默认主题（博客文章列表，即原 PublicHome 形态） ============
registerTheme({
  id: 'default',
  title: t('默认 · 博客文章'),
  desc: '爱库录内置博客形态：公开文章列表',
  version: '1.0.0',
  entry: null, // 由 BlogView 内部渲染默认列表（避免循环依赖）；此处仅占位
  pages: ['posts'],
  dataSources: ['blog'],
  dataScope: 'shared' // 系统层默认：只展示已分享文件
})
