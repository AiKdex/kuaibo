<template>
  <div class="anp-root">
    <!-- Top Header -->
    <header class="anp-top">
      <div class="anp-top-in">
        <div class="anp-logo">
          <span class="anp-logo-icon">◆</span>
          <a :href="homeHref" class="anp-logo-text">{{ ctx.siteName || '好站导航' }}</a>
        </div>
        <nav class="anp-nav">
          <a class="anp-nav-btn" :href="homeHref">{{ $t('← 返回列表') }}</a>
          <ThemeSwitch />
        </nav>
      </div>
    </header>

    <!-- Loading -->
    <div v-if="loading" class="anp-loading">
      <p>{{ $t('⏳ 加载中...') }}</p>
    </div>

    <!-- Error -->
    <div v-else-if="error" class="anp-error">
      <p>❌ {{ error }}</p>
    </div>

    <!-- Article + Sidebar -->
    <div v-else class="anp-layout"><article class="anp-article">
      <header class="anp-article-head">
        <h1 class="anp-article-h1">{{ title }}</h1>
        <div class="anp-article-meta">
          <span v-if="cat" class="anp-tag anp-tag-blue">{{ cat }}</span>
          <span v-if="author" class="anp-meta-date">✍ {{ author }}</span>
          <span v-if="date" class="anp-meta-date">📅 {{ date }}</span>
          <BlogPluginSlot mount="post_meta" :ctx="{ slug: post.slug }" />
        </div>
      </header>

      <!-- Post body (injected by system) -->
      <div class="anp-body" v-html="postHtml"></div>

      <!-- Share link (SSR) -->
      <div v-if="shareHref" class="anp-share">
        <a :href="shareHref" target="_blank" rel="noopener">{{ $t('🔗 查看静态页面 / 分享链接') }}</a>
      </div>

      <!-- Related posts -->
      <div v-if="related.length" class="anp-related">
        <div class="anp-section">
          <span class="anp-dot"></span>
          <h2 class="anp-section-h2">{{ $t('相关文章') }}</h2>
          <span class="anp-line"></span>
        </div>
        <div class="anp-related-grid">
          <a
            v-for="p in related"
            :key="p.token"
            class="anp-related-card"
            :href="relatedHref(p)"
          >
            <h3 class="anp-related-h3">{{ relatedTitle(p) }}</h3>
            <p class="anp-related-date">{{ relatedDate(p) }}</p>
          </a>
        </div>
      </div>
    </article>
    <aside class="anp-aside">
      <SideWidget />
    </aside>
    </div>

    <!-- Footer -->
    <footer class="anp-footer">
      <p>© {{ year }} {{ ctx.siteName || '好站导航' }} · Powered by AiKlog</p>
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
import BlogPluginSlot from '@/components/BlogPluginSlot.vue'
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
const author = computed(() => String(post.value.author || ''))
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
const postHtml = computed(() => post.value.html || '')
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
    .slice(0, 6)
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