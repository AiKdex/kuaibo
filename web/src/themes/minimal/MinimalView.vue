<template>
  <div class="mn-root">
    <header class="mn-head">
      <a class="mn-brand" :href="publicHref">{{ ctx.siteName || '爱库录' }}</a>
      <p class="mn-sub">{{ ctx.siteDesc || $t("目录即站点，文件即文章") }}</p>
      <nav class="mn-nav">
        <ThemeSwitch />
        <a :href="rssHref" target="_blank" rel="noopener">RSS</a>
        <a href="https://aiklog.com">{{ $t('官网') }}</a>
      </nav>
    </header>

    <main class="mn-main">
      <div v-if="ctx.loading" class="mn-state">{{ $t('加载中…') }}</div>
      <div v-else-if="ctx.error" class="mn-state">{{ ctx.error }}</div>
      <div v-else-if="!posts.length" class="mn-state">{{ $t('还没有公开文章') }}</div>

      <article v-for="(p, i) in posts" v-else :key="i" class="mn-item">
        <PostCover :post="p" round />
        <a class="mn-link" :href="href(p)">
          <h2 class="mn-title">{{ title(p) }}</h2>
          <p v-if="p.preview" class="mn-excerpt">{{ clean(p.preview) }}</p>
        </a>
        <div class="mn-meta">
          <span>{{ date(p) }}</span>
          <span v-if="cat(p)">· {{ cat(p) }}</span>
        </div>
      </article>
    </main>

    <footer class="mn-foot">
      <span>{{ ctx.siteName || '爱库录' }}</span>
      <a :href="rssHref">RSS</a>
    </footer>
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import PostCover from '@/components/PostCover.vue'
import './style.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], loading: true })
const posts = computed(() => ctx.value.posts || [])
const publicHref = '#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'

function href(p) {
  try {
    return ctx.value.postUrl ? ctx.value.postUrl(p) : `#/p/${p.token}`
  } catch {
    return '#'
  }
}
function title(p) {
  const n = p.file?.name || p.name || p.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('common.unnamed')
}
function cat(p) {
  const path = p.path || ''
  return path.includes('/') ? path.split('/')[0] : ''
}
function date(p) {
  const v = p.file?.updated_at || p.created_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
function clean(s) {
  return String(s || '')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/^（降级摘要）文件名：[^\n#]+/m, '')
    .replace(/^#{1,6}\s+[^\n]+$/gm, '')
    .replace(/\s+/g, ' ')
    .trim()
}
</script>
