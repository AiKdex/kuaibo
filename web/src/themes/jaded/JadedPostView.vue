<template>
  <div class="jdp-root">
    <!-- Ambient Background -->
    <div class="jdp-bg-ambient"></div>
    <div class="jdp-bg-noise"></div>

    <!-- Top Bar -->
    <header class="jdp-topbar">
      <div class="jdp-topbar-in">
        <a class="jdp-logo" :href="homeHref">
          <span class="jdp-logo-dot">●</span>
          <span>{{ ctx.siteName || 'AiKlog' }}</span>
        </a>
        <nav class="jdp-nav">
          <a :href="homeHref" class="jdp-nav-a">{{ $t('← 返回列表') }}</a>
        </nav>
        <div class="jdp-nav-right">
          <ThemeSwitch />
        </div>
      </div>
    </header>

    <!-- Loading / Error -->
    <div v-if="loading" class="jdp-status"><p>{{ $t('⏳ 加载中...') }}</p></div>
    <div v-else-if="error" class="jdp-status"><p>❌ {{ error }}</p></div>

    <!-- Article -->
    <div v-else class="jd-layout">
      <article class="jd-article jdp-wrapper">

        <!-- Article Header -->
        <header class="jdp-head">
          <div class="jdp-head-tags">
            <span v-if="cat" class="jdp-chip">{{ cat }}</span>
          </div>
          <h1 class="jdp-h1">{{ title }}</h1>
          <div class="jdp-meta">
            <div class="jdp-author">
              <div class="jdp-author-avatar">{{ avatarLetter }}</div>
              <span class="jdp-author-name">{{ ctx.siteName || '爱库录' }}</span>
            </div>
            <span v-if="date" class="jdp-meta-item">📅 {{ date }}</span>
            <span v-if="size" class="jdp-meta-item">📊 {{ size }} {{ $t('字') }}</span>
          </div>
        </header>

        <!-- AI Summary (from preview) -->
        <div v-if="post.preview" class="jdp-ai-summary">
          <div class="jdp-ai-label">{{ $t('🤖 AI 摘要') }}</div>
          <p>{{ post.preview }}</p>
        </div>

        <!-- Article Body -->
        <div class="jdp-body" v-html="postHtml"></div>

        <!-- Share Link -->
        <div v-if="shareHref" class="jdp-share">
          <a :href="shareHref" target="_blank" rel="noopener">{{ $t('🔗 查看静态页面 / 分享链接') }}</a>
        </div>

        <!-- Tags -->
        <div class="jdp-tags">
          <span v-if="cat" class="jdp-chip">{{ cat }}</span>
        </div>

        <!-- Related Posts -->
        <div v-if="related.length" class="jdp-related">
          <h3 class="jdp-related-title">{{ $t('📎 相关文章') }}</h3>
          <div class="jdp-related-grid">
            <a
              v-for="p in related"
              :key="p.token"
              class="jdp-related-card"
              :href="relatedHref(p)"
            >
              <h4>{{ relatedTitle(p) }}</h4>
              <p>{{ relatedDate(p) }}</p>
            </a>
          </div>
        </div>

      </article>
      <aside class="jd-aside"><SideWidget /></aside>
    </div>

    <!-- Footer -->
    <footer class="jdp-footer">
      <div class="jdp-footer-brand">
        <span class="jdp-logo-dot">●</span> {{ ctx.siteName || 'AiKlog' }}
      </div>
      <p class="jdp-footer-copy">© {{ year }} · Powered by AiKlog</p>
    </footer>
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import SideWidget from '../SideWidget.vue'
import { computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'
import './post.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], loading: true })
const loading = computed(() => ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const page = computed(() => ctx.value.page || {})
const post = computed(() => page.value.post || {})

const year = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'

const title = computed(() => post.value.title || t('common.unnamed'))
const cat = computed(() => post.value.cat || post.value.category || '')
const date = computed(() => {
  const v = post.value.updatedAt
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  const z = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}`
})
const size = computed(() => post.value.size || 0)
const postHtml = computed(() => post.value.html || '')
const avatarLetter = computed(() => (ctx.value.siteName || 'A')[0].toUpperCase())

const shareHref = computed(() => {
  if (!ctx.value.postSsrUrl || !post.value) return ''
  return ctx.value.postSsrUrl(post.value)
})

const related = computed(() => {
  const posts = ctx.value.posts || []
  const curToken = post.value.token
  const curCat = cat.value
  return posts
    .filter((p) => p.token !== curToken && (curCat ? (p.path || '').startsWith(curCat + '/') : true))
    .slice(0, 3)
})

function relatedTitle(p) {
  const n = p.file?.name || p.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('common.unnamed')
}
function relatedDate(p) {
  const v = p.file?.updated_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  const z = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}`
}
function relatedHref(p) {
  return ctx.value.postUrl ? ctx.value.postUrl(p) : '#'
}
</script>
