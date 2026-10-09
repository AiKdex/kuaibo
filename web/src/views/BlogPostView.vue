<template>
  <div ref="viewEl" class="bp-view" :class="[themeClass, 'th-view-post']">
    <!-- 插件挂载点：head（SEO meta 注入，隐藏容器） -->
    <BlogPluginSlot mount="head" :ctx="pluginCtx" />

    <!-- 极简顶栏：返回列表 + 站点名 + 静态页分享链接 + 主题切换 -->
    <header class="bp-bar">
      <a class="bp-back" href="#/blog?view=public" :title="$t('返回博客列表')">
        <span class="bp-arrow">←</span>
        <span class="bp-site">{{  ctx.siteName  }}</span>
      </a>
      <span class="bp-spacer"></span>
      <a v-if="ssrHref" class="bp-share" :href="ssrHref" :title="$t('静态页链接（适合复制分享、搜索引擎收录）')">
        {{  $t('分享链接')  }}
      </a>
      <GuestEntry variant="bar" />
      <ThemeSwitch v-if="showDockSwitch" mode="dock" />
    </header>

    <div v-if="ctx.loading" class="bp-center">
      <div class="spinner"></div>
      <span>{{  $t('加载中…')  }}</span>
    </div>

    <div v-else-if="ctx.error" class="bp-center">
      <AikIcon name="info" :size="20" />
      <p>{{  ctx.error  }}</p>
      <a class="bp-home" href="#/blog?view=public">{{  $t('返回博客列表')  }}</a>
    </div>

    <template v-else>
      <!-- 主题声明了 entries.post → 整页交给主题（宿主已注入 ctx.page.post：标题/正文/分类/作者） -->
      <component :is="postEntry" v-if="postEntry" :key="`${activeThemeId}:post`" />

      <!-- 未声明 → 宿主默认文章页；仍挂 .th-<id> 与主题变量钩子，保证至少配色/字体跟随 -->
      <main v-else class="bp-shell">
        <MarkdownArticle
          v-if="isText"
          :title="title"
          :html="html"
          :meta="meta"
          :footer="footer"
        />

        <div v-else class="bp-center">
          <AikIcon name="file" :size="36" />
          <p>{{  title  }}</p>
          <a v-if="ssrHref" class="bp-home" :href="ssrHref">{{  $t('在静态页查看')  }}</a>
        </div>

        <!-- 正文下方：相关推荐/评论/问答/统计（sidebar 类插件先挂 post_bottom） -->
        <BlogPluginSlot v-if="isText" mount="post_bottom" :ctx="pluginCtx" class="bp-plugins" />
      </main>
    </template>

    <footer class="bp-foot">
      <span>
        {{  ctx.siteName  }} ·
        <a href="#/blog?view=public">{{ $t('交互版列表') }}</a> ·
        <a :href="ssrHref || '/blog'">{{ $t('静态页') }}</a>
      </span>
    </footer>
  </div>
</template>

<script setup>
/**
 * BlogPostView —— 对外博客「文章页」的 SPA 交互版（人看侧主链）。
 *
 * 双轨定位（与 SSR 分工，不是替代）：
 *  - 人看：本组件（`/app#/post/{token}?path=&slug=`）——随主题走、带评论等插件、可换肤；
 *  - 机器看：SSR `/{slug}`（服务端渲染，canonical / sitemap / 分享快照）。
 *  两者由同一份数据渲染，主题只需保证视觉一致（`ssr.css` 管 SSR，主题组件管 SPA）。
 *
 * 取数复用「分享」通道（posts[].token 即分享 token，博客固定为 `blog`）：
 *  博客目录分享（scope=dir）→ 按 slug/path 命中文件 → fetchDirShareContent(token, path)
 *  单文件分享（scope=file）→ fetchShareContent(token)
 *
 * 主题接入：
 *  - 根节点挂 `.th-<id>`，主题可写 `.th-<id> { --th-*: … }` 定义变量钩子；
 *  - 主题声明 `entries.post` 则整页交由主题渲染，否则用本默认页（配色至少跟随）。
 */
import { ref, reactive, computed, onMounted, nextTick, watch, provide } from 'vue'
import { useRoute } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import MarkdownArticle from '@/components/MarkdownArticle.vue'
import BlogPluginSlot from '@/components/BlogPluginSlot.vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import GuestEntry from '@/components/GuestEntry.vue'
import { getShare, fetchShareContent, fetchDirShareContent, publicSite, publicPosts } from '@/api'
import { renderMarkdown, enhanceMermaid } from '@/utils/markdown'
import DOMPurify from 'dompurify'
import '@/blogPlugins/builtin' // 注册内置博客插件（评论/相关/统计/AI 阅读…）
import { loadEnabledPlugins } from '@/blogPlugins'
import '@/themes/all' // 注册全部主题（与列表页同一套注册表）
import { listThemes, activeThemeId, applyServerTheme, pageEntry } from '@/themes'
import { t } from '@/i18n'

const route = useRoute()

const TEXT_EXT = ['md', 'markdown', 'txt', 'log', 'json', 'html', 'htm']
const file = ref(null)
const html = ref('')

function extOf(name) {
  const i = (name || '').lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

const isText = computed(() => TEXT_EXT.includes(extOf(file.value?.name)))
const title = computed(() => (file.value?.name || '').replace(/\.(md|markdown|txt|html?)$/i, ''))

/** 分享给外部/搜索引擎用的静态页地址（SSR 主链） */
const ssrHref = computed(() => {
  const slug = String(route.query.slug || file.value?.slug || '').trim()
  if (slug) return `/${encodeURIComponent(slug)}`
  const t = title.value.trim()
  return t ? `/${encodeURIComponent(t)}` : ''
})

/** 文章页参数（供主题 entries.post 与插件槽读） */
const pagePost = computed(() => {
  const f = file.value || {}
  const p = String(route.query.path || f.path || '')
  const cat = p.includes('/') ? p.split('/')[0] : ''
  return {
    token: String(route.params.token || ''),
    path: p,
    slug: String(route.query.slug || f.slug || ''),
    title: title.value,
    // F7 修复：主题层（19 个 PostView.vue）一律 v-html="post.html"，此处是唯一入口 ——
    // 在这一道统一消毒，等于给所有主题加了兜底（html.value 本已过 renderMarkdown 消毒，
    // DOMPurify 对同一白名单是幂等的，二次消毒不改变 KaTeX/Mermaid 正常产出）。
    html: DOMPurify.sanitize(html.value),
    cat,
    category: cat,
    author: f.author || '',
    updatedAt: f.updated_at || 0,
    size: f.size || 0,
  }
})

/** 单篇地址生成器（主题内跳其它文章用；与列表页一致走 SPA） */
function postUrl(post) {
  const f = post?.file || {}
  const p = post?.path || f.path || ''
  const slug = f.slug || (p ? p.split('/').pop().replace(/\.(md|markdown)$/i, '') : '')
  const qs = new URLSearchParams()
  if (p) qs.set('path', p)
  if (slug) qs.set('slug', slug)
  const q = qs.toString()
  return `#/post/${encodeURIComponent(post?.token || '')}${q ? `?${q}` : ''}`
}

// 通用数据契约：与 BlogView 同构（多了 page，供文章页主题读取）
const ctx = reactive({
  siteName: t('爱库录'),
  siteDesc: t('爱库录 · AI 知识库博客：目录即站点，文件即文章'),
  aiAskOpen: true, // 站长级 AI 问答开关（blog.ai_ask_open）；默认开，/public/site 下发后覆盖
  posts: [],
  tags: [],
  postUrl,
  loading: true,
  error: '',
  page: computed(() => ({
    kind: 'post',
    post: pagePost.value,
    slug: pagePost.value.slug,
    path: pagePost.value.path,
    cat: pagePost.value.cat,
    q: '',
    archive: '',
    tag: '',
  })),
})
provide('themeContext', ctx)

/** 插件槽上下文（评论/相关推荐等） */
const pluginCtx = computed(() => {
  const f = file.value || {}
  return {
    title: f.name || '',
    path: pagePost.value.path,
    slug: pagePost.value.slug,
    category: pagePost.value.cat,
    updatedAt: f.updated_at || 0,
    size: f.size || 0,
    token: pagePost.value.token,
  }
})

function formatSize(n) {
  if (!n) return ''
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(2)} MB`
}

function formatTime(ms) {
  if (!ms) return ''
  const d = new Date(ms)
  if (isNaN(d.getTime())) return ''
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

const meta = computed(() => {
  const f = file.value || {}
  const arr = []
  if (f.updated_at) arr.push(`${formatTime(f.updated_at)} 更新`)
  if (f.author) arr.push(String(f.author))
  if (pagePost.value.cat) arr.push(pagePost.value.cat)
  if (f.size) arr.push(formatSize(f.size))
  return arr
})

const footer = computed(() => `本文由${ctx.siteName}发布 · 目录即站点，文件即文章`)

// ===== 主题 =====
const themes = computed(() => listThemes())
const activeTheme = computed(() => themes.value.find(t => t.id === activeThemeId.value) || null)
const postEntry = computed(() => pageEntry(activeTheme.value, 'post'))
const themeClass = computed(() => (activeTheme.value ? `th-${activeTheme.value.id}` : ''))
const showDockSwitch = computed(() => !!activeTheme.value && !activeTheme.value.ui?.themeSwitcher)

// ===== 取数 =====
async function loadSite() {
  try {
    const [siteRes, postsRes, themesRes] = await Promise.all([
      publicSite().catch(() => null),
      publicPosts().catch(() => ({ items: [] })),
      publicBlogThemes().catch(() => null), // 已安装主题列表（失败降级为全量可见）
    ])
    if (themesRes) applyInstalledThemes((themesRes.items || []).map((x) => x.id))
    if (siteRes?.site) {
      if (siteRes.site.title) ctx.siteName = siteRes.site.title
      if (siteRes.site.description) ctx.siteDesc = siteRes.site.description
      // AI 问答开关（缺省视为开；仅显式 "false" 关闭）
      ctx.aiAskOpen = siteRes.site.ai_ask_open !== 'false' && siteRes.site.ai_ask_open !== false
      applyServerTheme(siteRes.site.theme || 'aiklog', {
        allowVisitorSwitch: siteRes.site.allow_visitor_theme_switch !== 'false' && siteRes.site.allow_visitor_theme_switch !== false,
      })
    }
    // 访客本地覆盖指向「未安装/已卸载」的主题 → 丢弃覆盖，回落站点设置
    const ov = readOverride()
    if (ov && !isThemeInstalled(ov)) {
      try { localStorage.removeItem('aiklog.theme.override') } catch (e) { /* ignore */ }
      activeThemeId.value = serverThemeId.value || 'default'
    }
    // 确保当前生效主题已加载（懒 chunk）；失败/未安装 → 归位（宿主默认文章版式）
    if (activeThemeId.value && activeThemeId.value !== 'default') {
      const ok = await ensureTheme(activeThemeId.value)
      if (!ok) activeThemeId.value = serverThemeId.value && isThemeInstalled(serverThemeId.value) ? serverThemeId.value : 'default'
    }
    // 列表数据：主题在文章页可能用于「相关文章 / 上下篇」
    ctx.posts = postsRes.items || []
  } catch (e) {
    console.warn(t('[BlogPostView] 站点信息加载失败：'), e?.message || e)
  }
}

async function load() {
  const token = String(route.params.token || '')
  const qpath = String(route.query.path || '')
  const qslug = String(route.query.slug || '')
  ctx.loading = true
  ctx.error = ''
  file.value = null
  html.value = ''
  if (!token) {
    ctx.error = t('文章链接无效')
    ctx.loading = false
    return
  }
  try {
    const data = await getShare(token)
    if (data.share?.scope === 'dir') {
      const files = data.files || []
      let hit = qslug ? files.find((f) => f.slug === qslug) : null
      if (!hit && qpath) hit = files.find((f) => f.path === qpath)
      if (!hit) {
        ctx.error = t('文章不存在或已下线')
        return
      }
      file.value = hit
      if (isText.value) html.value = renderMarkdown(await fetchDirShareContent(token, hit.path))
    } else {
      file.value = data.file
      if (isText.value) html.value = renderMarkdown(await fetchShareContent(token))
    }
    if (file.value?.name) document.title = `${title.value} · ${ctx.siteName}`
  } catch (e) {
    ctx.error = e.message || t('文章加载失败')
  } finally {
    ctx.loading = false
  }
}

onMounted(async () => {
  await Promise.all([loadSite(), load()])
  loadEnabledPlugins()
})

// 同页 hash 导航（上下篇/相关文章跳转）：组件不重挂载，需监听路由变化
watch(
  () => [route.params.token, route.query.path, route.query.slug],
  () => { load() }
)

// Mermaid 增强：正文更新后扫描 ```mermaid 代码块替换为 SVG（默认页与主题文章页都覆盖）
const viewEl = ref(null)
watch(html, () => {
  nextTick(() => {
    if (viewEl.value) enhanceMermaid(viewEl.value)
  })
})
</script>

<style scoped>
.bp-view {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  /* 跟随当前博客主题：根节点已挂 .th-<id>（主题 CSS 全局加载后携带 --th-* 令牌）。
     这里把 --th-* 桥接成 MarkdownArticle / 应用壳消费的变量，使默认文章页（无 entries.post 的主题）
     也随主题换肤（配色 + 字体）。未挂主题类时回落中性默认值。 */
  background: var(--th-paper, #f5f6f8);
  --reading-bg-active: var(--th-paper, #fff);
  --reading-text-active: var(--th-ink, #111827);
  --reading-font-family: var(--th-font-b, var(--font, system-ui));
  --primary: var(--th-accent, #4c7df0);
  --accent: var(--th-accent, #4c7df0);
  --border: var(--th-line, #e5e7eb);
  --surface: var(--th-card, #fff);
  --surface-2: var(--th-card, #fff);
  --text: var(--th-ink, #111827);
  --text-2: var(--th-ink-2, #374151);
  --text-3: var(--th-ink-3, #6b7280);
  --font: var(--th-font-b, system-ui);
  --font-mono: var(--th-font-mono, ui-monospace, monospace);
  --hover: var(--th-accent-soft, rgba(76, 125, 240, 0.1));
}

.bp-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 24px;
  height: 56px;
  background: var(--th-card, #fff);
  border-bottom: 1px solid var(--th-line, #e5e7eb);
  position: sticky;
  top: 0;
  z-index: 10;
}

.bp-back {
  display: flex;
  align-items: center;
  gap: 8px;
  text-decoration: none;
  color: var(--text, #111827);
  font-weight: 600;
  font-size: 15px;
}

.bp-arrow {
  color: var(--primary, #4c7df0);
  font-size: 16px;
}

.bp-spacer { flex: 1; }

.bp-share {
  font-size: 12px;
  color: var(--primary, #4c7df0);
  text-decoration: none;
  padding: 4px 10px;
  border: 1px solid currentColor;
  border-radius: 999px;
  opacity: 0.85;
}

.bp-share:hover { opacity: 1; }

.bp-shell {
  flex: 1;
  width: min(100%, 860px);
  margin: 0 auto;
  padding: 0 20px 32px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.bp-shell :deep(.md-article) {
  --reading-maxw: 100%;
  padding: 28px 0 8px;
  background: transparent;
}

.bp-plugins {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.bp-plugins:empty { display: none; }

.bp-center {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  padding: 80px 24px;
  color: var(--text-3, #6b7280);
  font-size: 14px;
}

.bp-home {
  font-size: 13px;
  color: var(--primary, #4c7df0);
  text-decoration: none;
  padding: 6px 14px;
  border: 1px solid currentColor;
  border-radius: 999px;
}

.bp-foot {
  padding: 20px 24px 28px;
  text-align: center;
  font-size: 13px;
  color: var(--th-ink-3, #6b7280);
  border-top: 1px solid var(--th-line, #e5e7eb);
  background: var(--th-card, #fff);
}

.bp-foot a {
  color: var(--primary, #4c7df0);
  text-decoration: none;
}

.spinner {
  width: 26px;
  height: 26px;
  border: 3px solid var(--border, #e5e7eb);
  border-top-color: var(--primary, #4c7df0);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 767px) {
  .bp-bar { padding: 0 14px; }
}
</style>
