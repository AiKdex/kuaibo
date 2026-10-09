<template>
  <div class="zc-root">
    <!-- Top Bar -->
    <header class="zc-top">
      <div class="zc-top-in">
        <div class="zc-logo">
          <div class="zc-logo-icon">📚</div>
          <span>{{ ctx.siteName || '知藏' }}</span>
        </div>
        <nav class="zc-nav">
          <button type="button" :class="['zc-nav-a', { 'zc-on': !filterCat }]" @click="filterCat = ''">{{ $t('发现') }}</button>
          <button type="button" :class="['zc-nav-a', { 'zc-on': filterCat === c.name }]" v-for="c in cats.slice(0, 4)" :key="c.name" @click="filterCat = c.name">{{ c.name }}</button>
          <ThemeSwitch />
        </nav>
        <div class="zc-nav-right">
          <div class="zc-search-trigger" @click="showSearch = true">{{ $t('🔍 搜索资源...') }}</div>
        </div>
      </div>
    </header>

    <!-- Search Modal -->
    <div v-if="showSearch" class="zc-search-overlay" @click.self="showSearch = false">
      <div class="zc-search-modal">
        <input ref="searchRef" v-model="searchQ" class="zc-search-input" :placeholder="$t('搜索文章标题、摘要...')" @keyup.escape="showSearch = false" />
        <div v-if="searchResults.length" class="zc-search-results">
          <a v-for="p in searchResults" :key="p.token" class="zc-search-item" :href="postHref(p)">
            <span class="zc-search-item-title">{{ postTitle(p) }}</span>
            <span class="zc-search-item-cat">{{ postCat(p) || $t("未分类") }}</span>
          </a>
        </div>
        <p v-else-if="searchQ.trim()" class="zc-search-empty">{{ $t('未找到匹配资源') }}</p>
      </div>
    </div>

    <!-- Hero -->
    <section class="zc-hero">
      <div class="zc-hero-text">
        <h1 class="zc-hero-h1">{{ $t('优质资源') }}<br><em>{{ $t('系统学习') }}</em></h1>
        <p class="zc-hero-p">{{ ctx.siteDesc || $t("汇聚优质内容，分类整理，助你高效构建知识体系。") }}</p>
      </div>
      <div class="zc-hero-card">
        <div class="zc-hero-card-title">{{ $t('📊 站点概览') }}</div>
        <div class="zc-hero-stats">
          <div class="zc-hero-stat">
            <div class="zc-hero-stat-icon">📦</div>
            <span class="zc-hero-stat-num">{{ ctx.posts?.length || 0 }}</span>
            <span class="zc-hero-stat-label">{{ $t('资源总数') }}</span>
          </div>
          <div class="zc-hero-stat">
            <div class="zc-hero-stat-icon">📂</div>
            <span class="zc-hero-stat-num">{{ cats.length }}</span>
            <span class="zc-hero-stat-label">{{ $t('分类') }}</span>
          </div>
          <div class="zc-hero-stat">
            <div class="zc-hero-stat-icon">🕐</div>
            <span class="zc-hero-stat-num">{{ latestDate }}</span>
            <span class="zc-hero-stat-label">{{ $t('最近更新') }}</span>
          </div>
          <div class="zc-hero-stat">
            <div class="zc-hero-stat-icon">✨</div>
            <span class="zc-hero-stat-num">{{ $t('持续') }}</span>
            <span class="zc-hero-stat-label">{{ $t('更新中') }}</span>
          </div>
        </div>
        <div class="zc-hero-foot">
          <span class="zc-pulse-dot"></span>
          {{ $t('持续更新中 · 最近更新') }} {{ latestDate }}
        </div>
      </div>
    </section>

    <!-- Category Chips -->
    <div class="zc-section">
      <div class="zc-cats">
        <button type="button" :class="['zc-chip', { 'zc-on': !filterCat }]" @click="filterCat = ''"><span class="zc-chip-emoji">✨</span> {{ $t('全部') }}</button>
        <button type="button" :class="['zc-chip', { 'zc-on': filterCat === c.name }]" v-for="c in cats" :key="c.name" @click="filterCat = c.name">
          <span class="zc-chip-emoji">{{ catEmoji(c.name) }}</span> {{ c.name }}
        </button>
      </div>
    </div>

    <!-- Loading / Error / Empty -->
    <div v-if="loading" class="zc-status"><p>{{ $t('⏳ 加载中...') }}</p></div>
    <div v-else-if="error" class="zc-status"><p>❌ {{ error }}</p></div>
    <div v-else-if="!ctx.posts?.length" class="zc-status"><p>{{ $t('📭 还没有公开资源') }}</p></div>

    <!-- Main Content + Sidebar -->
    <div v-else class="zc-main-wrap">
      <main class="zc-main">

        <!-- Courses Section (first category as horizontal cards) -->
        <div v-if="courseGroup" class="zc-section-head">
          <div class="zc-section-title"><span class="zc-icon">🎓</span> {{ courseGroup.name }}</div>
          <span class="zc-section-cnt">{{ courseGroup.posts.length }} {{ $t('篇') }}</span>
        </div>
        <div v-if="courseGroup" class="zc-course-list">
          <a v-for="p in courseGroup.posts" :key="p.token" class="zc-course-card" :href="postHref(p)">
            <PostCover :post="p" round>
              <div class="zc-course-thumb" :style="thumbStyle(p)"><span>{{ catEmoji(postCat(p)) }}</span></div>
            </PostCover>
            <div class="zc-course-info">
              <h3>{{ postTitle(p) }}</h3>
              <p>{{ p.preview || $t("暂无摘要") }}</p>
              <div class="zc-course-meta">
                <span>📅 {{ postDate(p) }}</span>
                <span>{{ postCat(p) || $t("未分类") }}</span>
              </div>
            </div>
          </a>
        </div>

        <!-- Resource Grid (remaining posts) -->
        <div v-if="gridPosts.length" class="zc-section-head">
          <div class="zc-section-title"><span class="zc-icon">📦</span> {{ filterCat || $t("最新资源") }}</div>
          <span class="zc-section-cnt">{{ gridPosts.length }} {{ $t('篇') }}</span>
        </div>
        <div class="zc-res-grid">
          <a v-for="p in gridPosts" :key="p.token" class="zc-res-card" :href="postHref(p)">
            <PostCover :post="p" round>
              <div class="zc-res-cover" :style="coverStyle(p)">
                <div class="zc-res-pattern">{{ catEmoji(postCat(p)) }}</div>
              </div>
            </PostCover>
            <div class="zc-res-body">
              <div class="zc-res-type">{{ postCat(p) || '资源' }}</div>
              <h3>{{ postTitle(p) }}</h3>
              <p>{{ p.preview || $t("暂无摘要") }}</p>
              <div class="zc-res-meta">
                <span>📅 {{ postDate(p) }}</span>
              </div>
              <div class="zc-res-tags">
                <span class="zc-res-tag">{{ postCat(p) || $t("未分类") }}</span>
              </div>
            </div>
          </a>
        </div>

      </main>

      <!-- Sidebar -->
      <aside class="zc-side">

        <!-- Hot / Latest Resources -->
        <div class="zc-sb">
          <div class="zc-sb-title"><span class="zc-icon">🔥</span> {{ $t('最新资源') }}</div>
          <ul class="zc-hot-list">
            <li v-for="(p, i) in latest.slice(0, 5)" :key="p.token" class="zc-hot-item">
              <span class="zc-hot-rank">{{ i + 1 }}</span>
              <div class="zc-hot-text">
                <div class="zc-hot-name">{{ postTitle(p) }}</div>
                <div class="zc-hot-type">{{ postCat(p) || $t("未分类") }} · {{ postDate(p, 'short') }}</div>
              </div>
            </li>
          </ul>
        </div>

        <!-- Tags -->
        <div v-if="ctx.tags && ctx.tags.length" class="zc-sb">
          <div class="zc-sb-title"><span class="zc-icon">🏷️</span> {{ $t('标签') }}</div>
          <div class="zc-tag-cloud">
            <span v-for="t in ctx.tags" :key="t.name" class="zc-tag-item">{{ t.name }}</span>
          </div>
        </div>

        <!-- Archives -->
        <div v-if="archives.length" class="zc-sb">
          <div class="zc-sb-title"><span class="zc-icon">📅</span> {{ $t('归档') }}</div>
          <ul class="zc-archive-list">
            <li v-for="a in archives" :key="a.label">
              <span class="zc-archive-link">{{ a.label }}</span>
              <span class="zc-archive-cnt">{{ a.count }}</span>
            </li>
          </ul>
        </div>

        <!-- Quick Links -->
        <div class="zc-sb">
          <div class="zc-sb-title"><span class="zc-icon">🔗</span> {{ $t('快捷链接') }}</div>
          <div class="zc-sb-links">
            <a :href="rssHref"><span class="zc-icon">📡</span> {{ $t('RSS 订阅') }}</a>
            <a :href="homeHref"><span class="zc-icon">🏠</span> {{ $t('首页') }}</a>
          </div>
        </div>

      </aside>
    </div>

    <!-- Footer -->
    <footer class="zc-footer">
      <div class="zc-footer-brand">📚 {{ ctx.siteName || '知藏' }}</div>
      <div class="zc-footer-links">
        <a :href="rssHref">RSS</a>
        <a :href="homeHref">{{ $t('首页') }}</a>
      </div>
      <p class="zc-footer-copy">© {{ year }} {{ ctx.siteName || '知藏' }} · Powered by AiKlog</p>
    </footer>
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { ref, computed, inject, watch, nextTick } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import PostCover from '@/components/PostCover.vue'
import './style.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], tags: [], loading: true })
const loading = computed(() => ctx.value.loading)
const error = computed(() => ctx.value.error || '')

const year = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'

/* ── helpers ─────────────────────────────── */
function postTitle(p) {
  const n = p.file?.name || p.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('common.unnamed')
}
function postCat(p) {
  const path = p.path || ''
  return path.includes('/') ? path.split('/')[0] : ''
}
function postDate(p, fmt) {
  const v = p.file?.updated_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  const z = (n) => String(n).padStart(2, '0')
  if (fmt === 'short') return `${z(d.getMonth() + 1)}-${z(d.getDate())}`
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}`
}
function postHref(p) {
  return ctx.value.postUrl ? ctx.value.postUrl(p) : '#'
}

/* ── categories ──────────────────────────── */
const cats = computed(() => {
  const map = {}
  for (const p of (ctx.value.posts || [])) {
    const c = postCat(p)
    map[c] = (map[c] || 0) + 1
  }
  return Object.entries(map).map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count)
})

const filterCat = ref('')

const catEmojiMap = {}
const emojiList = ['💻', '🤖', '🎨', '📊', '📈', '🔐', '☁️', '🔧', '📱', '🚀', '💡', '📦', '🎓', '🧠']
function catEmoji(name) {
  if (!catEmojiMap[name]) {
    const idx = cats.value.findIndex((c) => c.name === name)
    catEmojiMap[name] = emojiList[(idx >= 0 ? idx : 0) % emojiList.length]
  }
  return catEmojiMap[name]
}

/* ── filtered posts ──────────────────────── */
const filteredPosts = computed(() => {
  const posts = ctx.value.posts || []
  if (!filterCat.value) return posts
  return posts.filter((p) => postCat(p) === filterCat.value)
})

const courseGroup = computed(() => {
  if (filterCat.value) return null
  const groups = {}
  for (const p of filteredPosts.value) {
    const c = postCat(p)
    if (!groups[c]) groups[c] = []
    groups[c].push(p)
  }
  const sorted = Object.entries(groups).sort((a, b) => b[1].length - a[1].length)
  if (sorted.length > 0) {
    return { name: sorted[0][0], posts: sorted[0][1].slice(0, 4) }
  }
  return null
})

const gridPosts = computed(() => {
  if (!filterCat.value && courseGroup.value) {
    const courseTokens = new Set(courseGroup.value.posts.map((p) => p.token))
    return filteredPosts.value.filter((p) => !courseTokens.has(p.token))
  }
  return filteredPosts.value
})

const dateMs = (p) => {
  const v = p.file?.updated_at
  if (!v) return 0
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  return Number.isFinite(ms) ? ms : 0
}

const latest = computed(() => {
  return [...(ctx.value.posts || [])].sort((a, b) => dateMs(b) - dateMs(a))
})

const latestDate = computed(() => {
  if (!latest.value.length) return '--'
  return postDate(latest.value[0], 'short') || '--'
})

/* ── archives ────────────────────────────── */
const archives = computed(() => {
  const map = {}
  for (const p of (ctx.value.posts || [])) {
    const d = postDate(p)
    if (!d) continue
    const ym = d.slice(0, 7)
    map[ym] = (map[ym] || 0) + 1
  }
  return Object.entries(map)
    .map(([label, count]) => ({ label: label.replace('-', ' 年 ') + ' 月', count, raw: label }))
    .sort((a, b) => b.raw.localeCompare(a.raw))
})

/* ── gradient patterns ───────────────────── */
const patterns = [
  'linear-gradient(135deg, #e0e7ff, #c7d2fe)',
  'linear-gradient(135deg, #fce7f3, #fbcfe8)',
  'linear-gradient(135deg, #d1fae5, #a7f3d0)',
  'linear-gradient(135deg, #fef3c7, #fde68a)',
  'linear-gradient(135deg, #e0f2fe, #bae6fd)',
  'linear-gradient(135deg, #ede9fe, #ddd6fe)',
]
function gradFor(name) {
  let h = 0
  for (let i = 0; i < name.length; i++) h = ((h << 5) - h + name.charCodeAt(i)) | 0
  return patterns[Math.abs(h) % patterns.length]
}
function thumbStyle(p) { return { background: gradFor(postCat(p)) } }
function coverStyle(p) { return { background: gradFor(postCat(p)) } }

/* ── search ──────────────────────────────── */
const showSearch = ref(false)
const searchQ = ref('')
const searchRef = ref(null)
const searchResults = computed(() => {
  const q = searchQ.value.trim().toLowerCase()
  if (!q) return []
  return (ctx.value.posts || []).filter((p) => {
    return postTitle(p).toLowerCase().includes(q) || (p.preview || '').toLowerCase().includes(q)
  }).slice(0, 10)
})
watch(showSearch, (v) => { if (v) nextTick(() => searchRef.value?.focus()) })
</script>