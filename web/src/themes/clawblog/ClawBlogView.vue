<template>
  <div class="cl-root">
    <!-- 页头：玻璃拟态导航 -->
    <header class="cl-header">
      <div class="cl-header-in">
        <a class="cl-brand" :href="homeHref">
          <span class="cl-logo-icon">{{ avatarLetter }}</span>
          <span class="cl-logo-text">{{ siteName }}</span>
        </a>
        <nav class="cl-nav">
          <a :href="homeHref" class="cl-nav-a active">{{ $t('首页') }}</a>
          <a :href="archiveHref" class="cl-nav-a">{{ $t('归档') }}</a>
          <ThemeSwitch />
          <a :href="rssHref" class="cl-nav-a">RSS</a>
        </nav>
        <button class="cl-search-btn" @click="showSearch = !showSearch" :title="$t('搜索')">🔍</button>
      </div>
    </header>

    <!-- 搜索栏（展开/收起） -->
    <div v-if="showSearch" class="cl-search-bar">
      <div class="cl-search-in">
        <input
          v-model="searchQuery"
          class="cl-search-input"
          type="text"
          :placeholder="$t('输入关键词搜索文章…')"
          @keydown.enter="goSearch"
        />
        <button class="cl-search-go" @click="goSearch">{{ $t('搜索') }}</button>
      </div>
    </div>

    <!-- Hero 区域（头条文章） -->
    <section class="cl-hero" v-if="featured">
      <div class="cl-hero-card">
        <div class="cl-hero-content">
          <span class="cl-hero-badge"><span class="cl-pulse"></span> {{ $t('最新发布') }}</span>
          <h1 class="cl-hero-title">
            <a :href="hrefOf(featured)">{{ titleOf(featured) }}</a>
          </h1>
          <p class="cl-hero-excerpt" v-if="featured.preview">{{ cleanPreview(featured.preview) }}</p>
          <div class="cl-hero-actions">
            <a class="cl-btn cl-btn-fill" :href="hrefOf(featured)">{{ $t('阅读全文 →') }}</a>
            <a class="cl-btn cl-btn-ghost" :href="archiveHref">{{ $t('浏览归档 ↗') }}</a>
          </div>
        </div>
      </div>
    </section>

    <!-- 状态处理：加载中 / 错误 / 空态 -->
    <div v-if="loading" class="cl-state">
      <div class="cl-state-icon">⏳</div>
      <p>{{ $t('正在加载文章…') }}</p>
    </div>
    <div v-else-if="error" class="cl-state cl-state-error">
      <div class="cl-state-icon">⚠️</div>
      <p>{{ error }}</p>
    </div>
    <div v-else-if="!posts.length" class="cl-state">
      <div class="cl-state-icon">📭</div>
      <p>{{ $t('还没有公开文章') }}</p>
    </div>

    <template v-else>
      <!-- 精选推荐：2 列大卡片 -->
      <section class="cl-section cl-section-out" v-if="picks.length">
        <div class="cl-section-head">
          <h2 class="cl-section-title"><span class="cl-sec-icon cl-sec-grad">📌</span> {{ $t('近期更新') }}</h2>
        </div>
        <div class="cl-grid-2">
          <article v-for="p in picks" :key="p.token" class="cl-card">
            <a :href="hrefOf(p)" class="cl-card-link">
              <div class="cl-card-thumb" :style="{ background: gradientOf(p) }">
                <span class="cl-card-emoji">{{ emojiOf(p) }}</span>
                <div class="cl-card-tags" v-if="catOf(p)">
                  <span class="cl-card-tag">{{ catOf(p) }}</span>
                </div>
              </div>
              <div class="cl-card-body">
                <h3 class="cl-card-title">{{ titleOf(p) }}</h3>
                <p class="cl-card-excerpt" v-if="p.preview">{{ cleanPreview(p.preview) }}</p>
                <div class="cl-card-meta">
                  <span class="cl-meta-date">{{ dateOf(p) }}</span>
                </div>
              </div>
            </a>
          </article>
        </div>
      </section>

      <!-- 最新文章：3 列网格 -->
      <section class="cl-section cl-section-out" v-if="gridPosts.length">
        <div class="cl-section-head">
          <h2 class="cl-section-title"><span class="cl-sec-icon cl-sec-pink">✨</span> {{ $t('最新文章') }}</h2>
        </div>
        <div class="cl-grid-3">
          <article v-for="p in gridPosts" :key="p.token" class="cl-card">
            <a :href="hrefOf(p)" class="cl-card-link">
              <PostCover :post="p" round>
                <div class="cl-card-thumb" :style="{ background: gradientOf(p) }">
                  <span class="cl-card-emoji">{{ emojiOf(p) }}</span>
                  <div class="cl-card-tags" v-if="catOf(p)">
                    <span class="cl-card-tag">{{ catOf(p) }}</span>
                  </div>
                </div>
              </PostCover>
              <div class="cl-card-body">
                <h3 class="cl-card-title">{{ titleOf(p) }}</h3>
                <p class="cl-card-excerpt" v-if="p.preview">{{ cleanPreview(p.preview) }}</p>
                <div class="cl-card-meta">
                  <span class="cl-meta-date">{{ dateOf(p) }}</span>
                </div>
              </div>
            </a>
          </article>
        </div>
      </section>

      <!-- 全部文章：左列表 + 右侧栏 -->
      <section class="cl-section cl-section-out" v-if="listPosts.length">
        <div class="cl-section-head">
          <h2 class="cl-section-title"><span class="cl-sec-icon cl-sec-grad">📖</span> {{ $t('全部文章') }}</h2>
        </div>
        <div class="cl-main-area">
          <div class="cl-left-col">
            <article v-for="p in listPosts" :key="p.token" class="cl-list-item">
              <a :href="hrefOf(p)" class="cl-list-link">
                <PostCover :post="p" round>
                  <div class="cl-list-thumb" :style="{ background: gradientOf(p) }">
                    <span class="cl-list-emoji">{{ emojiOf(p) }}</span>
                  </div>
                </PostCover>
                <div class="cl-list-body">
                  <div class="cl-list-tags" v-if="catOf(p)">
                    <span class="cl-list-tag">{{ catOf(p) }}</span>
                  </div>
                  <h3 class="cl-list-title">{{ titleOf(p) }}</h3>
                  <p class="cl-list-excerpt" v-if="p.preview">{{ cleanPreview(p.preview) }}</p>
                  <div class="cl-list-meta">
                    <span class="cl-meta-date">{{ dateOf(p) }}</span>
                  </div>
                </div>
              </a>
            </article>
          </div>

          <aside class="cl-right-col">
            <!-- 个人简介 -->
            <div class="cl-widget">
              <div class="cl-profile">
                <div class="cl-profile-avatar">{{ avatarLetter }}</div>
                <div class="cl-profile-name">{{ siteName }}</div>
                <div class="cl-profile-desc">{{ ctx.siteDesc || $t("记录技术与生活的博客") }}</div>
                <div class="cl-profile-stats">
                  <div class="cl-pstat"><div class="cl-pstat-num">{{ posts.length }}</div><div class="cl-pstat-label">{{ $t('文章') }}</div></div>
                  <div class="cl-pstat"><div class="cl-pstat-num">{{ cats.length }}</div><div class="cl-pstat-label">{{ $t('分类') }}</div></div>
                  <div class="cl-pstat"><div class="cl-pstat-num">{{ tags.length }}</div><div class="cl-pstat-label">{{ $t('标签') }}</div></div>
                </div>
              </div>
            </div>

            <!-- 文章分类 -->
            <div class="cl-widget" v-if="cats.length">
              <div class="cl-widget-head">
                <h3>{{ $t('📂 文章分类') }}</h3>
                <a :href="archiveHref" class="cl-widget-more">{{ $t('归档 →') }}</a>
              </div>
              <div class="cl-cat-list">
                <a v-for="(c, i) in cats.slice(0, 6)" :key="c.name" class="cl-cat-item" :href="'/app#/blog/cat/' + encodeURIComponent(c.name)">
                  <span class="cl-cat-left">
                    <span class="cl-cat-icon" :class="'cl-ci' + ((i % 6) + 1)">{{ catEmoji(c.name) }}</span>
                    <span class="cl-cat-name">{{ c.name }}</span>
                  </span>
                  <span class="cl-cat-count">{{ c.count }}</span>
                </a>
              </div>
            </div>

            <!-- 标签云 -->
            <div class="cl-widget" v-if="tags.length">
              <div class="cl-widget-head">
                <h3>{{ $t('🏷️ 标签') }}</h3>
              </div>
              <div class="cl-tagcloud">
                <a v-for="t in tags" :key="t.name" class="cl-tcloud-t" :href="'/app#/blog/tag/' + encodeURIComponent(t.name)">{{ t.name }}</a>
              </div>
            </div>
          </aside>
        </div>
      </section>
    </template>

    <!-- 页脚：四栏深色 -->
    <footer class="cl-footer">
      <div class="cl-footer-grid">
        <div class="cl-fbrand">
          <div class="cl-fbrand-logo">
            <span class="cl-logo-icon">{{ avatarLetter }}</span>
            <span class="cl-logo-text">{{ siteName }}</span>
          </div>
          <p class="cl-fbrand-desc">{{ ctx.siteDesc || $t("一个关于技术与生活的博客") }}</p>
          <div class="cl-fbrand-links">
            <a :href="rssHref" :title="$t('RSS 订阅')">📡</a>
            <a :href="homeHref" :title="$t('首页')">🏠</a>
            <a :href="archiveHref" :title="$t('归档')">📅</a>
          </div>
        </div>
        <div class="cl-fcol">
          <h4>{{ $t('导航') }}</h4>
          <ul>
            <li><a :href="homeHref">{{ $t('首页') }}</a></li>
            <li><a :href="archiveHref">{{ $t('归档') }}</a></li>
            <li><a href="/app#/blog/search">{{ $t('搜索') }}</a></li>
          </ul>
        </div>
        <div class="cl-fcol" v-if="cats.length">
          <h4>{{ $t('分类') }}</h4>
          <ul>
            <li v-for="c in cats.slice(0, 4)" :key="c.name"><a :href="'/app#/blog/cat/' + encodeURIComponent(c.name)">{{ c.name }}</a></li>
          </ul>
        </div>
        <div class="cl-fcol">
          <h4>{{ $t('订阅') }}</h4>
          <ul>
            <li><a :href="rssHref">{{ $t('RSS 订阅') }}</a></li>
            <li><a href="/api/v1/blog/sitemap.xml">Sitemap</a></li>
          </ul>
        </div>
      </div>
      <div class="cl-footer-bottom">
        <span>© {{ currentYear }} {{ siteName }}</span>
        <span>Powered by AiKlog</span>
      </div>
    </footer>

    <!-- 回到顶部 -->
    <button class="cl-fab" @click="scrollToTop" :title="$t('回到顶部')">↑</button>
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { ref, computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import PostCover from '@/components/PostCover.vue'
import './style.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], tags: [], loading: true })

const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => ctx.value.tags || [])
const loading = computed(() => ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || 'ClawBlog')
const avatarLetter = computed(() => (ctx.value.siteName || 'C')[0].toUpperCase())
const currentYear = new Date().getFullYear()

const homeHref = '/app#/blog?view=public'
const archiveHref = '/app#/blog/archive'
const rssHref = '/api/v1/blog/feed.xml'

const showSearch = ref(false)
const searchQuery = ref('')

function goSearch() {
  const q = searchQuery.value.trim()
  if (q) window.location.href = `/app#/blog/search?q=${encodeURIComponent(q)}`
}

function scrollToTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

/* ── 派生函数（与 SSR 保持一致）── */
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
  return ctx.value.postUrl ? ctx.value.postUrl(p) : '#'
}
function cleanPreview(s) {
  return String(s || '').replace(/^#+\s*/gm, '').replace(/[*_`~]/g, '').slice(0, 120)
}

/* ── 分类聚合（列表页侧栏与页脚共用）── */
const cats = computed(() => {
  const map = {}
  for (const p of posts.value) {
    const c = catOf(p)
    if (!c) continue
    map[c] = (map[c] || 0) + 1
  }
  return Object.entries(map).map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count)
})
const catEmojiArr = ['💻', '⚙️', '🚀', '🧠', '🎨', '🌿']
const emojiCache = {}
function catEmoji(name) {
  if (!emojiCache[name]) {
    const idx = cats.value.findIndex((c) => c.name === name)
    emojiCache[name] = catEmojiArr[(idx >= 0 ? idx : 0) % catEmojiArr.length]
  }
  return emojiCache[name]
}

/* ── 分类渐变色映射 ── */
const gradientMap = {
  'tech': 'linear-gradient(135deg,#6c5ce7,#a29bfe)',
  'ai': 'linear-gradient(135deg,#fd79a8,#e17055)',
  'dev': 'linear-gradient(135deg,#00b894,#55efc4)',
  'life': 'linear-gradient(135deg,#fdcb6e,#e17055)',
  'go': 'linear-gradient(135deg,#0984e3,#74b9ff)',
  'docker': 'linear-gradient(135deg,#00cec9,#0984e3)',
  'linux': 'linear-gradient(135deg,#636e72,#2d3436)',
  'vue': 'linear-gradient(135deg,#2ecc71,#27ae60)',
  'css': 'linear-gradient(135deg,#e17055,#fdcb6e)',
  'default': 'linear-gradient(135deg,#6c5ce7,#a29bfe)',
}
const emojiMap = {
  'tech': '💻', 'ai': '🧠', 'dev': '🚀', 'life': '🌿',
  'go': '🐹', 'docker': '🐳', 'linux': '🐧', 'vue': '💚',
  'css': '🎨', 'default': '📄',
}
function gradientOf(p) {
  const c = catOf(p).toLowerCase()
  return gradientMap[c] || gradientMap.default
}
function emojiOf(p) {
  const c = catOf(p).toLowerCase()
  return emojiMap[c] || emojiMap.default
}

/* ── 布局分区：hero / 精选 2 列 / 最新 3 列 / 全部横向列表 ── */
const featured = computed(() => posts.value[0] || null)
const picks = computed(() => posts.value.slice(1, 3))
const gridPosts = computed(() => posts.value.slice(3, 6))
const listPosts = computed(() => posts.value.slice(6))
</script>
