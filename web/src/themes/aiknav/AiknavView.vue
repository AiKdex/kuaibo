<template>
  <div class="an-root">
    <!-- Top Header -->
    <header class="an-top">
      <div class="an-top-in">
        <div class="an-logo">
          <span class="an-logo-icon">◆</span>
          <span>{{ ctx.siteName || '好站导航' }}</span>
        </div>
        <nav class="an-nav">
          <button
            type="button"
            :class="['an-nav-btn', { 'an-on': !activeCat }]"
            @click="scrollToTop"
          >{{ $t('全部') }}</button>
          <button
            v-for="c in cats.slice(0, 5)"
            :key="c.name"
            type="button"
            :class="['an-nav-btn', { 'an-on': activeCat === c.name }]"
            @click="jumpCat(c.name)"
          >{{ c.name }}</button>
          <ThemeSwitch />
          <a class="an-cta" :href="rssHref">RSS</a>
        </nav>
      </div>
    </header>

    <!-- Hero -->
    <section class="an-hero">
      <h1 class="an-hero-h1">{{ heroTitle }}</h1>
      <p class="an-hero-p">{{ ctx.siteDesc || $t("优质内容，分类整理，持续更新。") }}</p>
      <div class="an-search">
        <input
          v-model="q"
          class="an-search-input"
          :placeholder="$t('搜索文章、工具、资源...')"
          @keyup.enter="doSearch"
        />
        <button type="button" class="an-search-btn" @click="doSearch">{{ $t('搜索') }}</button>
      </div>
      <div class="an-stats">
        <span>🔗 <strong>{{ ctx.posts?.length || 0 }}</strong> {{ $t('篇文章') }}</span>
        <span>📂 <strong>{{ cats.length }}</strong> {{ $t('个分类') }}</span>
      </div>
    </section>

    <!-- Search Results Overlay -->
    <div v-if="searched" class="an-wrap">
      <main class="an-main">
        <div class="an-section">
          <span class="an-dot" style="background:var(--th-blue)"></span>
          <h2 class="an-section-h2">{{ $t('搜索「') }}{{ q }}{{ $t('」的结果') }}</h2>
          <span class="an-line"></span>
          <span class="an-cnt">{{ filtered.length }}</span>
        </div>
        <div v-if="filtered.length" class="an-grid">
          <a
            v-for="p in filtered"
            :key="p.token"
            class="an-card"
            :href="postHref(p)"
          >
            <div class="an-card-icon"><span>📄</span></div>
            <div class="an-card-info">
              <h3 class="an-card-h3">{{ postTitle(p) }}</h3>
              <p class="an-card-desc">{{ p.preview || $t("暂无摘要") }}</p>
              <div class="an-card-meta">
                <span class="an-tag an-tag-blue">{{ postCat(p) || $t("未分类") }}</span>
                <span class="an-tag an-tag-green">{{ postDate(p) }}</span>
              </div>
            </div>
            <span class="an-arrow">→</span>
          </a>
        </div>
        <p v-else class="an-empty">{{ $t('未找到匹配的文章') }}</p>
        <button type="button" class="an-back-btn" @click="clearSearch">{{ $t('← 返回全部') }}</button>
      </main>
    </div>

    <!-- Main Content -->
    <div v-else class="an-wrap">
      <!-- Sidebar -->
      <aside class="an-side">
        <div class="an-sb">
          <h4 class="an-sb-h4">{{ $t('📂 分类') }}</h4>
          <ul class="an-cat-tree">
            <li
              :class="['an-cat-item', { 'an-on': !activeCat }]"
              @click="activeCat = ''"
            >
              <span class="an-cat-dot" style="background:var(--th-blue)"></span>
              {{ $t('全部') }}
              <span class="an-cat-cnt">{{ ctx.posts?.length || 0 }}</span>
            </li>
            <li
              v-for="c in cats"
              :key="c.name"
              :class="['an-cat-item', { 'an-on': activeCat === c.name }]"
              @click="activeCat = c.name"
            >
              <span class="an-cat-dot" :style="{ background: catColor(c.name) }"></span>
              {{ c.name }}
              <span class="an-cat-cnt">{{ c.count }}</span>
            </li>
          </ul>
        </div>

        <div class="an-sb">
          <h4 class="an-sb-h4">{{ $t('🕐 最新更新') }}</h4>
          <ul class="an-latest-list">
            <li
              v-for="(p, i) in latest.slice(0, 5)"
              :key="p.token"
              class="an-latest-item"
            >
              <span :class="['an-latest-num', { 'an-latest-top': i === 0 }]">{{ i + 1 }}</span>
              <a class="an-latest-name" :href="postHref(p)">{{ postTitle(p) }}</a>
              <span class="an-latest-date">{{ postDate(p, 'short') }}</span>
            </li>
          </ul>
        </div>

        <div class="an-sb">
          <h4 class="an-sb-h4">{{ $t('🔗 链接') }}</h4>
          <div class="an-sb-links">
            <a :href="rssHref">{{ $t('📡 RSS 订阅') }}</a>
            <a :href="archiveHref">{{ $t('📖 归档') }}</a>
            <a :href="homeHref">{{ $t('🏠 首页') }}</a>
          </div>
        </div>
      </aside>

      <!-- Main -->
      <main class="an-main">
        <!-- Loading -->
        <div v-if="loading" class="an-loading">
          <p>{{ $t('⏳ 加载中...') }}</p>
        </div>

        <!-- Error -->
        <div v-else-if="error" class="an-error">
          <p>❌ {{ error }}</p>
        </div>

        <!-- Empty -->
        <div v-else-if="!ctx.posts?.length" class="an-empty">
          <p>{{ $t('📭 还没有公开文章') }}</p>
        </div>

        <!-- Posts grouped by category -->
        <template v-else>
          <div
            v-for="group in displayGroups"
            :key="group.name"
            :id="'cat-' + group.name"
            class="an-group"
          >
            <div class="an-section">
              <span class="an-dot" :style="{ background: catColor(group.name) }"></span>
              <h2 class="an-section-h2">{{ group.name || $t("未分类") }}</h2>
              <span class="an-line"></span>
              <span class="an-cnt">{{ group.posts.length }}</span>
            </div>
            <div class="an-grid">
              <a
                v-for="p in group.posts"
                :key="p.token"
                class="an-card"
                :href="postHref(p)"
              >
                <div class="an-card-icon"><span>{{ catEmoji(group.name) }}</span></div>
                <div class="an-card-info">
                  <h3 class="an-card-h3">{{ postTitle(p) }}</h3>
                  <p class="an-card-desc">{{ p.preview || $t("暂无摘要") }}</p>
                  <div class="an-card-meta">
                    <span class="an-tag" :class="catTagClass(group.name)">{{ group.name || $t("未分类") }}</span>
                    <span class="an-tag an-tag-green">{{ postDate(p) }}</span>
                  </div>
                </div>
                <span class="an-arrow">→</span>
              </a>
            </div>
          </div>
        </template>
      </main>
    </div>

    <!-- Footer -->
    <footer class="an-footer">
      <div class="an-footer-links">
        <a :href="rssHref">RSS</a>
        <a :href="archiveHref">{{ $t('归档') }}</a>
        <a :href="homeHref">{{ $t('首页') }}</a>
      </div>
      <p>© {{ year }} {{ ctx.siteName || '好站导航' }} · Powered by AiKlog</p>
    </footer>
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { ref, computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], tags: [], loading: true })
const loading = computed(() => ctx.value.loading)
const error = computed(() => ctx.value.error || '')

const q = ref('')
const searched = ref(false)
const activeCat = ref('')

const year = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'
const archiveHref = '/app#/blog?view=public'

const heroTitle = computed(() => {
  const name = ctx.value.siteName || '好站导航'
  return `发现好站，高效上网`
})

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
  return Object.entries(map)
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count)
})

const catColors = ['var(--th-blue)', 'var(--th-green)', 'var(--th-purple)', 'var(--th-orange)', 'var(--th-pink)', 'var(--th-cyan)']
const catEmojis = { '': '📄' }
const tagClasses = { '': 'an-tag-blue' }
function catColor(name) {
  const idx = cats.value.findIndex((c) => c.name === name)
  return catColors[idx % catColors.length]
}
function catEmoji(name) {
  if (!catEmojis[name]) {
    const emojis = ['📁', '🔧', '🤖', '🎨', '⚡', '📝', '🚀', '💡', '📊', '🗂️']
    const idx = cats.value.findIndex((c) => c.name === name)
    catEmojis[name] = emojis[idx % emojis.length]
  }
  return catEmojis[name]
}
function catTagClass(name) {
  if (!tagClasses[name]) {
    const classes = ['an-tag-blue', 'an-tag-green', 'an-tag-purple', 'an-tag-orange', 'an-tag-pink', 'an-tag-cyan']
    const idx = cats.value.findIndex((c) => c.name === name)
    tagClasses[name] = classes[idx % classes.length]
  }
  return tagClasses[name]
}

/* ── display groups ──────────────────────── */
const filtered = computed(() => {
  const posts = ctx.value.posts || []
  const keyword = q.value.trim().toLowerCase()
  if (!keyword) return posts
  return posts.filter((p) => {
    const t = postTitle(p).toLowerCase()
    const prev = (p.preview || '').toLowerCase()
    const cat = postCat(p).toLowerCase()
    return t.includes(keyword) || prev.includes(keyword) || cat.includes(keyword)
  })
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

const displayGroups = computed(() => {
  let posts = ctx.value.posts || []
  if (activeCat.value) {
    posts = posts.filter((p) => postCat(p) === activeCat.value)
  }
  const map = {}
  for (const p of posts) {
    const c = postCat(p)
    if (!map[c]) map[c] = []
    map[c].push(p)
  }
  return Object.entries(map)
    .map(([name, list]) => ({ name, posts: list }))
    .sort((a, b) => b.posts.length - a.posts.length)
})

/* ── search ──────────────────────────────── */
function doSearch() {
  if (q.value.trim()) searched.value = true
}
function clearSearch() {
  q.value = ''
  searched.value = false
}

/* ── scroll navigation ───────────────────── */
function scrollToTop() {
  activeCat.value = ''
  window.scrollTo({ top: 0, behavior: 'smooth' })
}
function jumpCat(name) {
  activeCat.value = name
  // scroll to the category section after DOM update
  setTimeout(() => {
    const el = document.getElementById('cat-' + name)
    if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }, 100)
}
</script>