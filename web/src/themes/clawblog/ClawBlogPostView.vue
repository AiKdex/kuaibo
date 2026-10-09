<template>
  <div class="cl-root">
    <!-- 页头 -->
    <header class="cl-header">
      <div class="cl-header-in">
        <a class="cl-brand" :href="homeHref">
          <span class="cl-logo-icon">C</span>
          <span class="cl-logo-text">{{ siteName }}</span>
        </a>
        <nav class="cl-nav">
          <a :href="homeHref" class="cl-nav-a">{{ $t('首页') }}</a>
          <a :href="archiveHref" class="cl-nav-a">{{ $t('归档') }}</a>
          <ThemeSwitch />
          <a :href="rssHref" class="cl-nav-a">RSS</a>
        </nav>
      </div>
    </header>

    <!-- 文章内容 -->
    <main v-if="post" class="cb-layout">
      <div class="cb-article">
      <article class="cl-post">
        <div class="cl-post-header">
          <div class="cl-post-tags" v-if="post.cat || post.category">
            <a :href="'#/blog/cat/' + encodeURIComponent(post.cat || post.category)" class="cl-post-cat">
              {{ post.cat || post.category }}
            </a>
          </div>
          <h1 class="cl-post-title">{{ post.title || titleOf(post) }}</h1>
          <div class="cl-post-meta">
            <span v-if="post.author" class="cl-post-author">{{ post.author }}</span>
            <span v-if="post.author" class="cl-meta-dot"></span>
            <span class="cl-meta-date">{{ dateOf(post) }}</span>
            <span class="cl-meta-dot"></span>
            <a :href="shareHref" class="cl-post-share" target="_blank" rel="noopener">{{ $t('查看静态页') }}</a>
            <BlogPluginSlot mount="post_meta" :ctx="{ slug: post.slug }" />
          </div>
        </div>
        <div class="cl-post-body" v-html="post.html"></div>
      </article>

      <!-- 相关文章 -->
      <section v-if="related.length" class="cl-related">
        <h2 class="cl-section-title">{{ $t('📖 更多文章') }}</h2>
        <div class="cl-related-list">
          <a v-for="r in related" :key="r.token" :href="hrefOf(r)" class="cl-related-item">
            <span class="cl-related-title">{{ titleOf(r) }}</span>
            <span class="cl-related-date">{{ dateOf(r) }}</span>
          </a>
        </div>
      </section>
      </div>
      <aside class="cb-aside"><SideWidget /></aside>
    </main>
    <div v-else class="cl-state">
      <div class="cl-state-icon">📄</div>
      <p>{{ $t('正在加载文章…') }}</p>
    </div>

    <!-- 页脚 -->
    <footer class="cl-footer">
      <div class="cl-footer-in">
        <p class="cl-footer-copy">© {{ currentYear }} {{ siteName }} · Powered by AiKlog</p>
      </div>
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

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], page: {} })

const siteName = computed(() => ctx.value.siteName || 'ClawBlog')
const currentYear = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
const archiveHref = '/app#/blog/archive'
const rssHref = '/api/v1/blog/feed.xml'

const page = computed(() => ctx.value.page || {})
const post = computed(() => page.value.post || null)
const shareHref = computed(() => {
  if (!post.value) return '/blog'
  return ctx.value.postSsrUrl ? ctx.value.postSsrUrl(post.value) : '/blog'
})

/* 相关文章：同分类前 5 篇，排除当前 */
const related = computed(() => {
  if (!post.value) return []
  const curCat = post.value.cat || post.value.category || ''
  const curToken = post.value.token || ''
  return (ctx.value.posts || [])
    .filter((p) => {
      const c = (p.path || '').includes('/') ? p.path.split('/')[0] : ''
      return c === curCat && p.token !== curToken
    })
    .slice(0, 5)
})

function titleOf(p) {
  const n = p.file?.name || p.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('common.unnamed')
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
</script>