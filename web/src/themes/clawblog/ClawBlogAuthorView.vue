<template>
  <div class="cl-root">
    <header class="cl-header">
      <div class="cl-header-in">
        <a class="cl-brand" :href="homeHref"><span class="cl-logo-icon">C</span><span class="cl-logo-text">{{ siteName }}</span></a>
        <nav class="cl-nav"><a :href="homeHref" class="cl-nav-a">{{ $t('首页') }}</a><ThemeSwitch /></nav>
      </div>
    </header>
    <main class="cl-inner-main">
      <h1 class="cl-inner-title">{{ $t('作者：') }}{{ author }}</h1>
      <p class="cl-inner-count">{{ list.length }} {{ $t('篇') }}</p>
      <div v-if="list.length" class="cl-list">
        <article v-for="p in list" :key="p.token" class="cl-list-item">
          <a :href="hrefOf(p)" class="cl-list-link">
            <div class="cl-list-thumb" style="background:linear-gradient(135deg,#6c5ce7,#a29bfe)"><span class="cl-list-emoji">📄</span></div>
            <div class="cl-list-body">
              <h3 class="cl-list-title">{{ titleOf(p) }}</h3>
              <div class="cl-list-meta"><span class="cl-meta-date">{{ dateOf(p) }}</span></div>
            </div>
          </a>
        </article>
      </div>
      <p v-else class="cl-empty">{{ $t('该作者下暂无文章') }}</p>
    </main>
    <footer class="cl-footer"><div class="cl-footer-in"><p class="cl-footer-copy">© {{ currentYear }} {{ siteName }}</p></div></footer>
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], page: {} })
const siteName = computed(() => ctx.value.siteName || 'ClawBlog')
const currentYear = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
const page = computed(() => ctx.value.page || {})
const author = computed(() => page.value.author || '')
const list = computed(() =>
  (ctx.value.posts || []).filter((p) => {
    const a = p.file?.author || ''
    return a === author.value
  })
)
function titleOf(p) { const n = p.file?.name || p.path || ''; return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop() || t('common.unnamed') }
function dateOf(p) { const v = p.file?.updated_at; if (!v) return ''; const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v); if (!Number.isFinite(ms)) return ''; const d = new Date(ms); const z = (n) => String(n).padStart(2,'0'); return `${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}` }
function hrefOf(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '#' }
</script>