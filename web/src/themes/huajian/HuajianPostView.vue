<template>
  <div class="hj-root">
    <div class="hj-topline"></div>

    <!-- 页头（与列表页同构） -->
    <header class="hj-hdr">
      <div class="hj-hdr-in">
        <a class="hj-brand" :href="homeHref">
          <span class="hj-brand-name">{{ siteName }}</span>
          <span class="hj-brand-dot"></span>
        </a>
        <nav class="hj-nav">
          <a :href="homeHref">{{ $t('首页') }}</a>
          <a :href="archiveHref">{{ $t('归档') }}</a>
          <a :href="tagBaseHref">{{ $t('标签') }}</a>
          <ThemeSwitch />
        </nav>
        <div class="hj-hdr-right">
          <button class="hj-icon-btn" @click="showSearch = !showSearch" :title="$t('搜索')" :aria-label="$t('搜索')">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7"></circle><line x1="21" y1="21" x2="16.5" y2="16.5"></line></svg>
          </button>
          <a class="hj-btn hj-btn-amber" :href="rssHref">RSS</a>
          <a ref="searchLink" :href="searchHref" style="display:none" aria-hidden="true"></a>
        </div>
      </div>
    </header>

    <!-- 搜索栏（展开/收起） -->
    <div v-if="showSearch" class="hj-search-bar">
      <div class="hj-search-in">
        <input
          v-model="searchQuery"
          class="hj-search-input"
          type="text"
          :placeholder="$t('输入关键词搜索文章…')"
          @keydown.enter="goSearch"
        />
        <button class="hj-btn hj-btn-amber" @click="goSearch">{{ $t('搜索') }}</button>
      </div>
    </div>

    <!-- 状态处理：加载中 / 错误 / 空态 -->
    <div v-if="loading" class="hj-state">
      <div class="hj-state-icon">⏳</div>
      <p>{{ $t('正在加载文章…') }}</p>
    </div>
    <div v-else-if="error" class="hj-state hj-state-error">
      <div class="hj-state-icon">⚠️</div>
      <p>{{ error }}</p>
    </div>
    <div v-else-if="!post" class="hj-state">
      <div class="hj-state-icon">📄</div>
      <p>{{ $t('文章不存在或未公开') }}</p>
    </div>

    <template v-else>
      <!-- 正文 + 侧栏（与列表页双栏同构） -->
      <div class="hj-post-main">
        <div class="hj-post-col">
          <article class="hj-post">
            <div v-if="post.cat || post.category" class="hj-post-tags">
              <a :href="catHref" class="hj-post-cat">{{ post.cat || post.category }}</a>
            </div>
            <h1 class="hj-post-title">{{ post.title || titleOf(post) }}</h1>
            <div class="hj-post-meta">
              <span v-if="post.author" class="hj-post-author">{{ post.author }}</span>
              <span v-if="post.author" class="hj-meta-dot"></span>
              <span>{{ dateOf(post) }}</span>
              <span class="hj-meta-dot"></span>
              <a :href="shareHref" class="hj-post-share" target="_blank" rel="noopener">{{ $t('查看静态页') }}</a>
            </div>
            <div class="hj-post-body" v-html="post.html"></div>
          </article>

          <!-- 相关文章 -->
          <section v-if="related.length" class="hj-related">
            <h2 class="hj-related-title">{{ $t('更多文章') }}</h2>
            <div class="hj-related-list">
              <a v-for="r in related" :key="r.token" :href="hrefOf(r)" class="hj-related-item">
                <span class="hj-related-name">{{ titleOf(r) }}</span>
                <span class="hj-related-date">{{ dateOf(r) }}</span>
              </a>
            </div>
          </section>

          <!-- 评论插件槽 -->
          <section class="hj-plugins">
            <BlogPluginSlot mount="post_bottom" :ctx="pluginCtx" />
          </section>
        </div>

        <!-- 侧栏（与列表页同构） -->
        <aside class="hj-side">
          <!-- 关于 -->
          <div class="hj-side-widget">
            <div class="hj-about-card">
              <div class="hj-about-avatar">{{ initialOf(siteName) }}</div>
              <div class="hj-about-name">{{ siteName }}</div>
              <p class="hj-about-desc">{{ siteDesc || $t("写代码的人，偶尔写点别的。") }}</p>
              <div class="hj-about-stats">
                <div class="hj-about-stat"><div class="num">{{ posts.length }}</div><div class="label">{{ $t('文章') }}</div></div>
                <div class="hj-about-stat"><div class="num">{{ catStats.length }}</div><div class="label">{{ $t('分类') }}</div></div>
                <div class="hj-about-stat"><div class="num">{{ tags.length }}</div><div class="label">{{ $t('标签') }}</div></div>
              </div>
            </div>
          </div>

          <!-- 分类 -->
          <div v-if="catStats.length" class="hj-side-widget">
            <div class="hj-side-head"><h3>{{ $t('分类') }}</h3><span class="hj-side-line"></span></div>
            <div class="hj-cat-list">
              <a
                v-for="c in catStats"
                :key="c.name"
                class="hj-cat-item"
                :href="'/app#/blog/cat/' + encodeURIComponent(c.name)"
              >
                <span class="hj-cat-left">
                  <span class="hj-cat-dot" :style="{ background: catColor(c.name) }"></span>
                  <span class="hj-cat-name">{{ c.name }}</span>
                </span>
                <span class="hj-cat-count">{{ c.count }}</span>
              </a>
            </div>
          </div>

          <!-- 标签 -->
          <div v-if="tags.length" class="hj-side-widget">
            <div class="hj-side-head"><h3>{{ $t('标签') }}</h3><span class="hj-side-line"></span></div>
            <div class="hj-tags">
              <a v-for="t in tags" :key="t.name" :href="tagHref(t.name)">{{ t.name }}</a>
            </div>
          </div>

          <!-- 引言 -->
          <div class="hj-side-widget">
            <div class="hj-quote-card">
              <p class="q">{{ $t('「好的设计是尽可能少的设计。」') }}</p>
              <span class="author">— Dieter Rams</span>
            </div>
          </div>
        </aside>
      </div>
    </template>

    <!-- 页脚 -->
    <footer class="hj-ft">
      <div class="hj-ft-in">
        <div class="hj-ft-bottom" style="border-top:none;padding-top:0">
          <span>© {{ currentYear }} {{ siteName }}</span>
          <span>Powered by AiKlog</span>
        </div>
      </div>
    </footer>

    <!-- AI 对话窗（文章页自动带上当前文章 path） -->
    <HuajianAsk />
  </div>
</template>

<script setup>
import { t } from '@/i18n'
import { ref, computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import BlogPluginSlot from '@/components/BlogPluginSlot.vue'
import HuajianAsk from './HuajianAsk.vue'
import './style.css'
import './post.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], tags: [], page: {} })

const siteName = computed(() => ctx.value.siteName || '花笺')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const currentYear = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
const archiveHref = '/app#/blog/archive'
const tagBaseHref = '/app#/blog/tag'
const rssHref = '/api/v1/blog/feed.xml'

const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => ctx.value.tags || [])
const page = computed(() => ctx.value.page || {})
const post = computed(() => page.value.post || null)
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const shareHref = computed(() => {
  if (!post.value) return '/blog'
  return ctx.value.postSsrUrl ? ctx.value.postSsrUrl(post.value) : '/blog'
})
const catHref = computed(() => {
  const c = post.value?.cat || post.value?.category || ''
  return c ? '/app#/blog/cat/' + encodeURIComponent(c) : '/app#/blog?view=public'
})

/* 相关文章：同分类前 5 篇，排除当前（保持系统顺序） */
const related = computed(() => {
  if (!post.value) return []
  const curCat = post.value.cat || post.value.category || ''
  const curToken = post.value.token || ''
  return posts.value
    .filter((p) => catOf(p) === curCat && p.token !== curToken)
    .slice(0, 5)
})

const showSearch = ref(false)
const searchQuery = ref('')
const searchLink = ref(null)
const searchHref = computed(() => {
  const q = searchQuery.value.trim()
  return q ? `/app#/blog/search?q=${encodeURIComponent(q)}` : '/app#/blog/search'
})

function goSearch() {
  const q = searchQuery.value.trim()
  if (q) searchLink.value?.click()
}

const pluginCtx = computed(() => ({
  title: post.value?.title,
  path: post.value?.path,
  slug: post.value?.slug,
  category: post.value?.cat || post.value?.category,
  updatedAt: post.value?.updatedAt,
  size: post.value?.size,
  token: post.value?.token,
}))

function titleOf(p) {
  const n = p.file?.name || p.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('common.unnamed')
}
function catOf(p) {
  const path = p.path || ''
  return path.includes('/') ? path.split('/')[0] : ''
}
function dateOf(p) {
  const v = p.file?.updated_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  const z = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}`
}
function hrefOf(p) {
  return ctx.value.postUrl ? ctx.value.postUrl(p) : '/app#/blog?view=public'
}
function tagHref(name) {
  return '/app#/blog/tag/' + encodeURIComponent(name)
}
function initialOf(name) {
  return String(name || 'H').trim().charAt(0).toUpperCase() || 'H'
}

/* ── 分类统计与色板（派生，无臆造字段）── */
const catStats = computed(() => {
  const map = {}
  for (const p of posts.value) {
    const c = catOf(p)
    if (!c) continue
    map[c] = (map[c] || 0) + 1
  }
  return Object.entries(map)
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count)
})
const catPalette = [
  'var(--th-accent)', 'var(--th-olive)', 'var(--th-accent-deep)',
  '#7c6e8a', '#5a7c8a', '#8a7a5a', '#6b7c5e', '#a05c50',
]
function catColor(name) {
  let h = 0
  for (const ch of String(name)) h = (h * 31 + ch.charCodeAt(0)) % 997
  return catPalette[h % catPalette.length]
}
</script>
