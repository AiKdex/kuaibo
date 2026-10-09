<template>
  <div class="zcp-root">
    <!-- Top Bar -->
    <header class="zcp-top">
      <div class="zcp-top-in">
        <div class="zcp-logo">
          <div class="zcp-logo-icon">📚</div>
          <a :href="homeHref" class="zcp-logo-text">{{ ctx.siteName || '知藏' }}</a>
        </div>
        <nav class="zcp-nav">
          <a class="zcp-nav-a" :href="homeHref">{{ $t('← 返回列表') }}</a>
          <ThemeSwitch />
        </nav>
      </div>
    </header>

    <!-- Loading / Error -->
    <div v-if="loading" class="zcp-status"><p>{{ $t('⏳ 加载中...') }}</p></div>
    <div v-else-if="error" class="zcp-status"><p>❌ {{ error }}</p></div>

    <!-- Article -->
    <div v-else class="zc-layout">
      <article class="zc-article zcp-wrapper">

        <!-- Header -->
        <header class="zcp-head">
          <div class="zcp-head-tags">
            <span v-if="cat" class="zcp-chip">{{ cat }}</span>
          </div>
          <h1 class="zcp-h1">{{ title }}</h1>
          <div class="zcp-meta">
            <span v-if="author" class="zcp-meta-item">✍ {{ author }}</span>
            <span v-if="date" class="zcp-meta-item">📅 {{ date }}</span>
            <span v-if="cat" class="zcp-meta-item">📂 {{ cat }}</span>
            <BlogPluginSlot mount="post_meta" :ctx="{ slug: post.slug }" />
          </div>
        </header>

        <!-- AI Summary -->
        <div v-if="post.preview" class="zcp-summary">
          <div class="zcp-summary-label">{{ $t('📋 摘要') }}</div>
          <p>{{ post.preview }}</p>
        </div>

        <!-- Body -->
        <div class="zcp-body" v-html="postHtml"></div>

        <!-- Share -->
        <div v-if="shareHref" class="zcp-share">
          <a :href="shareHref" target="_blank" rel="noopener">{{ $t('🔗 查看静态页面 / 分享链接') }}</a>
        </div>

        <!-- Tags -->
        <div class="zcp-tags">
          <span v-if="cat" class="zcp-chip">{{ cat }}</span>
        </div>

        <!-- Related -->
        <div v-if="related.length" class="zcp-related">
          <h3 class="zcp-related-title">{{ $t('📎 相关资源') }}</h3>
          <div class="zcp-related-grid">
            <a v-for="p in related" :key="p.token" class="zcp-related-card" :href="relatedHref(p)">
              <h4>{{ relatedTitle(p) }}</h4>
              <p>{{ relatedDate(p) }}</p>
            </a>
          </div>
        </div>

      </article>
      <aside class="zc-aside"><SideWidget /></aside>
    </div>

    <!-- Footer -->
    <footer class="zcp-footer">
      <p>© {{ year }} {{ ctx.siteName || '知藏' }} · Powered by AiKlog</p>
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
const shareHref = computed(() => ctx.value.postSsrUrl && post.value ? ctx.value.postSsrUrl(post.value) : '')

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
function relatedHref(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '#' }
</script>
