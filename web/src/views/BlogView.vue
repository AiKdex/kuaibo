<template>
  <div class="blog-view">
    <!-- 管理态工具条：公开读者视图不显示（主题在控制台站点设置里改） -->
    <div v-if="!viewPublic" class="th-switchbar">
      <span class="th-label">{{  $t('博客')  }}</span>
      <span v-if="activeTheme" class="th-desc">{{  activeTheme.title  }} · {{  activeTheme.desc  }}</span>
      <a class="th-ssr" href="/blog" :title="$t('服务端静态页（SEO）')">{{  $t('静态页')  }}</a>
      <button v-if="isAdmin" class="th-manage" :class="{ on: manageMode }" @click="manageMode = !manageMode">
        {{  manageMode ? $t('返回预览') : $t('管理博客')  }}
      </button>
    </div>

    <!-- 博客管理（登录态） -->
    <BlogManage v-if="manageMode" />

    <!-- 默认主题 + 列表页：博客文章列表（原 PublicHome 形态） -->
    <PublicHome v-else-if="isDefaultTheme && pageKind === 'list'" />

    <!-- 主题页面：按页面契约分发（list → entry；内页 → entries.*），注入通用博客数据契约 -->
    <!-- 视图根包裹层：th-<id> 承载主题令牌/样式作用域；th-view-<kind> 供构建插件区分视图 -->
    <div v-else class="th-view-list" :class="[activeTheme ? 'th-' + activeTheme.id : '', 'th-view-' + pageKind]">
      <component
        v-if="themeEntry"
        :is="themeEntry"
        :key="pageKind + ':' + (activeTheme ? activeTheme.id : '')"
      />
      <!-- 系统默认内页：主题未声明该 entries.* 时的兜底（套主题 .th-<id> 令牌，配色字体不割裂） -->
      <div v-else class="th-fallback">
        <header class="th-fb-head">
          <a class="th-fb-brand" href="#/blog?view=public">{{  ctx.siteName  }}</a>
          <nav class="th-fb-nav">
            <ThemeSwitch mode="inline" />
            <a href="/blog">{{  $t('静态页')  }}</a>
          </nav>
        </header>
        <main class="th-fb-main">
          <h1 class="th-fb-title">{{  fallbackTitle  }}</h1>
          <p class="th-fb-count">{{  fallbackPosts.length  }} {{  $t('篇')  }}</p>
          <ul class="th-fb-list">
            <li v-for="p in fallbackPosts" :key="p.token + (p.path || '')">
              <a :href="ctx.postUrl(p)">{{  postTitle(p)  }}</a>
              <span class="th-fb-date">{{  postDate(p)  }}</span>
            </li>
          </ul>
          <p v-if="!fallbackPosts.length && !ctx.loading" class="th-fb-empty">{{  fallbackEmpty  }}</p>
          <p v-if="ctx.loading" class="th-fb-empty">{{  $t('加载中…')  }}</p>
        </main>
        <footer class="th-fb-foot">{{  ctx.siteName  }}</footer>
      </div>
    </div>

    <!-- 公开页主题切换：主题自带 inline 切换器时不重复渲染；否则宿主补右上角悬浮版 -->
    <ThemeSwitch v-if="showDockSwitch" mode="dock" />
    <!-- 语言切换：公开页脱离应用外壳，顶栏那颗覆盖不到访客，故宿主补悬浮版 -->
    <LangSwitch mode="dock" />

    <!-- 公开读者视图：右下角固定的登录 / 注册入口（已登录则显示控制台） -->
    <GuestEntry v-if="showGuestDock" variant="dock" />
  </div>
</template>

<script setup>
import { t as i18t } from '@/i18n'
import { ref, computed, onMounted, watch, provide } from 'vue'
import { useRoute } from 'vue-router'
import PublicHome from './PublicHome.vue'
import BlogManage from './BlogManage.vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import LangSwitch from '@/components/LangSwitch.vue'
import GuestEntry from '@/components/GuestEntry.vue'
import { listThemes, activeThemeId, applyServerTheme, pageEntry } from '@/themes'
import { publicPosts, publicSite, publicTags, isAuthed } from '@/api'
import { postCover } from '@/utils/postCover.js'
import '@/themes/all' // 注册全部主题（与文章页 BlogPostView 共用同一份清单）
import { t } from '@/i18n'

const route = useRoute()

// ===== 内页识别（分类 / 作者 / 标签 / 归档 / 搜索）=====
// 两种等价寻址：query（#/blog?view=public&cat=xxx）与 path 段（#/blog/cat/xxx）。
// 未命中任何内页参数 → list（由主题入口组件渲染整页列表）。
const PAGE_KEYS = ['cat', 'author', 'tag', 'archive']
const PATH_PAGE_RE = /^\/blog\/(cat|author|tag|archive)\/(.+?)\/?$/

function pagePathMatch() {
  return PATH_PAGE_RE.exec(route.path || '')
}

const pageKind = computed(() => {
  const q = route.query || {}
  if (String(q.q || '').trim()) return 'search'
  for (const k of PAGE_KEYS) {
    if (String(q[k] || '').trim()) return k
  }
  const m = pagePathMatch()
  if (m) return m[1]
  if (/^\/blog\/search\/?$/.test(route.path || '')) return 'search'
  return 'list'
})

/** 当前页参数 → ctx.page（供主题 entries.* 组件读取；契约见 themes/index.js） */
const pageCtx = computed(() => {
  const kind = pageKind.value
  const q = route.query || {}
  const m = pagePathMatch()
  const fromPath = (k) => (m && m[1] === k ? decodeURIComponent(m[2]) : '')
  const one = (k) => fromPath(k) || String(q[k] || '')
  return {
    kind,
    cat: kind === 'cat' ? one('cat') : '',
    author: kind === 'author' ? one('author') : '',
    tag: kind === 'tag' ? one('tag') : '',
    archive: kind === 'archive' ? one('archive') : '',
    q: kind === 'search' ? String(q.q || '') : '',
  }
})

// 文章链接辅助 —— slug 派生（优先 file.slug，回退文件名去扩展名）与定位查询串。
// path/slug 同时带上：目录分享（博客固定 token）按 path 命中，slug 用于改名后仍可定位。
function postSlug(post) {
  const f = post?.file || {}
  if (f.slug) return String(f.slug)
  const p = post?.path || f.path || ''
  return p ? String(p).split('/').pop().replace(/\.(md|markdown)$/i, '') : ''
}

function postQuery(post) {
  const f = post?.file || {}
  const p = post?.path || f.path || ''
  const slug = postSlug(post)
  const qs = new URLSearchParams()
  if (p) qs.set('path', p)
  if (slug) qs.set('slug', slug)
  const q = qs.toString()
  return q ? `?${q}` : ''
}
// 注册表可能在主题异步加载后变化，用 computed 每次取最新
const themes = computed(() => listThemes())
// 登录态判定：cookie 双轨后新会话不再写 localStorage token，改用 sessionStorage 标记（isAuthed）
const isAdmin = computed(() => isAuthed())
// 登录态进入 /blog 默认即管理界面（目录管理/上传/发布）；?view=public 强制读者视图（对外首页）
// 内页路由（cat/author/tag/archive/search）恒为公开视图，不受「管理员进 /blog 默认管理态」影响
const viewPublic = computed(() => route.query.view === 'public' || pageKind.value !== 'list')
const manageMode = ref(false)
// 初始视图 + SPA 内 query 变化（如 管理↔对外预览 切换）时同步
function syncView() {
  // 管理员进 /blog 默认是后台「博客管理」（留在 AppShell 壳内）；?view=public 才是读者预览
  manageMode.value = isAdmin.value && !viewPublic.value
}
onMounted(syncView)
watch(() => route.query.view, syncView)
watch(isAdmin, syncView)

// ===== 通用数据契约：博客公开内容（供 dataSources 含 'blog' 的主题使用） =====
const ctx = ref({
  siteName: '爱库录',
  siteDesc: t('爱库录 · AI 知识库博客：目录即站点，文件即文章'),
  aiAskOpen: true, // 站长级 AI 问答开关（blog.ai_ask_open）；默认开，/public/site 下发后覆盖
  posts: [],
  tags: [],
  page: pageCtx, // 页面参数（list/post/cat/author/tag/archive/search）
  // 文章地址生成器 —— 双轨分工：
  //   人看（postUrl）  → SPA 文章页 #/post/{token}，随主题走、带评论等插件、可换肤；
  //   机器看（postSsrUrl）→ SSR /{slug}，canonical / sitemap / 社交分享快照。
  // 主题内跳文章一律用 postUrl；「复制链接/分享」场景用 postSsrUrl。
  postUrl: (post) => `#/post/${encodeURIComponent(post?.token || '')}${postQuery(post)}`,
  postSsrUrl: (post) => {
    const slug = postSlug(post)
    return slug ? `/${encodeURIComponent(slug)}` : '/blog'
  },
  loading: true,
  error: ''
})

async function loadBlogContext() {
  ctx.value.loading = true
  ctx.value.error = ''
  try {
    const [postsRes, siteRes, tagsRes] = await Promise.all([
      publicPosts().catch(() => ({ items: [] })),
      publicSite().catch(() => null),
      publicTags().catch(() => ({ items: [] })),
    ])
    // 统一注入封面字段（单一来源）：各主题卡片直接读 post.cover 即可，无需各自计算
    ctx.value.posts = (postsRes.items || []).map((p) => ({ ...p, cover: postCover(p) }))
    ctx.value.tags = tagsRes?.items || []
    if (siteRes?.site) {
      if (siteRes.site.title) ctx.value.siteName = siteRes.site.title
      if (siteRes.site.description) ctx.value.siteDesc = siteRes.site.description
      // AI 问答开关（缺省视为开；仅显式 "false" 关闭）
      ctx.value.aiAskOpen = siteRes.site.ai_ask_open !== 'false' && siteRes.site.ai_ask_open !== false
      // 站点设置里的主题 → 交互版公开视图；读者可在页头切换器覆盖（本地优先）
      applyServerTheme(siteRes.site.theme || 'aiklog', {
        allowVisitorSwitch: siteRes.site.allow_visitor_theme_switch !== 'false' && siteRes.site.allow_visitor_theme_switch !== false,
      })
    }
  } catch (e) {
    ctx.value.error = e.message || t('加载失败')
  } finally {
    ctx.value.loading = false
  }
}

provide('themeContext', ctx)

const activeTheme = computed(() => themes.value.find(t => t.id === activeThemeId.value) || null)
// 页面级入口分发：list → theme.entry；内页 → theme.entries.{cat|category,author,tag,archive,search}
// 主题未声明该内页 → null（宿主回退系统默认内页，仍套 .th-<id> 令牌，配色字体不割裂）
const themeEntry = computed(() => pageEntry(activeTheme.value, pageKind.value))
const isDefaultTheme = computed(() => !activeTheme.value || activeTheme.value.id === 'default')

// ===== 系统默认内页（主题未声明该 entries.* 时的兜底渲染）=====
function postTitle(post) {
  const p = String(post?.path || '')
  const name = post?.file?.name || (p ? p.split('/').pop() : '')
  return String(name || '').replace(/\.(md|markdown|txt|html?)$/i, '') || t('无标题')
}
function postDate(post) {
  let ms = post?.file?.updated_at || 0
  if (!ms) return ''
  // 兼容历史秒级时间戳（< 1e12）：归一为毫秒，避免显示 1970
  if (ms < 1e12) ms *= 1000
  const d = new Date(ms)
  if (isNaN(d.getTime())) return ''
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}
const fallbackPosts = computed(() => {
  const list = ctx.value.posts || []
  const k = pageKind.value
  // 分类支持多级：cat=tech 命中 tech/ai/post.md；cat=tech/ai 命中二级子目录（前缀匹配）
  if (k === 'cat') { const c = pageCtx.value.cat || ''; return list.filter(p => { const path = String(p.path || ''); return path === c + '.md' || path.startsWith(c + '/') }) }
  if (k === 'author') return list.filter(p => String(p.file?.author || '') === pageCtx.value.author)
  if (k === 'tag') return list.filter(p => (p.file?.tags || []).includes(pageCtx.value.tag))
  if (k === 'archive') return list.filter(p => postDate(p).slice(0, 7) === pageCtx.value.archive)
  if (k === 'search') {
    const q = pageCtx.value.q.trim().toLowerCase()
    if (!q) return []
    return list.filter(p => `${postTitle(p)} ${p.preview || ''}`.toLowerCase().includes(q))
  }
  return list
})
const fallbackTitle = computed(() => {
  const v = pageCtx.value
  if (v.kind === 'cat') return `分类：${v.cat || '未指定'}`
  if (v.kind === 'author') return `作者：${v.author || '未指定'}`
  if (v.kind === 'tag') return `标签：${v.tag || '未指定'}`
  if (v.kind === 'archive') return `归档：${v.archive || '全部'}`
  if (v.kind === 'search') return `搜索：${v.q || ''}`
  return ctx.value.siteName
})
const fallbackEmpty = computed(() => {
  const v = pageCtx.value
  if (v.kind === 'search') return i18t('没有匹配的文章')
  if (v.kind === 'tag') return i18t('该标签下暂无文章')
  if (v.kind === 'archive') return i18t('该月份暂无文章')
  if (v.kind === 'author') return i18t('该作者暂无文章')
  if (v.kind === 'cat') return i18t('该分类下暂无文章')
  return i18t('还没有公开文章')
})

// 主题自己在页头渲染了切换器（manifest: ui.themeSwitcher）→ 宿主不再悬浮兜底
const showDockSwitch = computed(
  () => viewPublic.value && !!activeTheme.value && !activeTheme.value.ui?.themeSwitcher
)
// 访客入口兜底：主题自行渲染整页时（非 default）在右下角补一个；
// default 主题走 PublicHome，其顶栏已内嵌 GuestEntry，不重复渲染。
const showGuestDock = computed(
  () => viewPublic.value && !!activeTheme.value && activeTheme.value.id !== 'default'
)

onMounted(async () => {
  await loadBlogContext()
})
</script>

<style scoped>
.blog-view {
  min-height: 100vh;
  background: var(--app-bg, #f5f6f8);
}

/* 主题切换条 */
.th-switchbar {
  position: sticky;
  top: 0;
  z-index: 999;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 16px;
  background: #fff;
  border-bottom: 1px solid var(--border, #e5e7eb);
  font-size: 12px;
  color: var(--text-3, #6b7280);
}

.th-label {
  font-weight: 600;
  color: var(--text, #111827);
}

.th-select {
  padding: 3px 8px;
  border: 1px solid var(--border, #e5e7eb);
  border-radius: 6px;
  font-size: 12px;
  background: #fff;
  color: var(--text, #111827);
}

.th-desc {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.th-ssr {
  flex-shrink: 0;
  color: #0d7a6a;
  text-decoration: none;
  font-size: 12px;
}

.th-ssr:hover {
  text-decoration: underline;
}

.th-ver {
  opacity: 0.7;
  flex-shrink: 0;
}

/* 博客管理按钮（登录态显示） */
.th-manage {
  margin-left: auto;
  flex-shrink: 0;
  font-size: 13px;
  padding: 4px 14px;
  border-radius: 999px;
  border: 1px solid var(--accent, #4c7df0);
  background: transparent;
  color: var(--accent, #4c7df0);
  cursor: pointer;
  transition: all 0.15s ease;
}

.th-manage:hover {
  background: rgba(76, 125, 240, 0.1);
}

.th-manage.on {
  background: var(--accent, #4c7df0);
  color: #fff;
}

/* ===== 系统默认内页（主题未声明 entries.* 时的兜底，套 --th-* 令牌） ===== */
.th-fallback {
  min-height: 100vh;
  background: var(--th-paper, #fff);
  color: var(--th-ink, #111827);
}

.th-fb-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 24px;
  border-bottom: 1px solid var(--th-line, #e5e7eb);
}

.th-fb-brand {
  font-weight: 700;
  text-decoration: none;
  color: var(--th-ink, #111827);
}

.th-fb-nav {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 13px;
}

.th-fb-nav a {
  color: var(--th-accent, #0d7a6a);
  text-decoration: none;
}

.th-fb-main {
  max-width: 820px;
  margin: 0 auto;
  padding: 32px 24px 64px;
}

.th-fb-title {
  font-size: 26px;
  margin: 0 0 6px;
}

.th-fb-count {
  margin: 0 0 20px;
  font-size: 13px;
  opacity: 0.65;
}

.th-fb-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.th-fb-list li {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 0;
  border-bottom: 1px dashed var(--th-line, #e5e7eb);
}

.th-fb-list a {
  color: var(--th-ink, #111827);
  text-decoration: none;
  font-weight: 600;
}

.th-fb-list a:hover {
  color: var(--th-accent, #0d7a6a);
}

.th-fb-date {
  font-size: 12px;
  opacity: 0.6;
  white-space: nowrap;
}

.th-fb-empty {
  margin-top: 24px;
  font-size: 14px;
  opacity: 0.6;
}

.th-fb-foot {
  padding: 24px;
  text-align: center;
  font-size: 12px;
  opacity: 0.6;
  border-top: 1px solid var(--th-line, #e5e7eb);
}
</style>
