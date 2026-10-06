<template>
  <div class="cl-root">
    <header class="cl-header">
      <div class="cl-header-in">
        <a class="cl-brand" :href="homeHref"><span class="cl-logo-icon">C</span><span class="cl-logo-text">{{ siteName }}</span></a>
        <nav class="cl-nav"><a :href="homeHref" class="cl-nav-a">{{ $t('首页') }}</a><ThemeSwitch /></nav>
      </div>
    </header>
    <main class="cl-inner-main">
      <h1 class="cl-inner-title">{{ $t('归档') }}{{ archiveLabel ? '：' + archiveLabel : '' }}</h1>
      <p class="cl-inner-count">{{ posts.length }} {{ $t('篇文章') }}</p>

      <div v-if="grouped.length">
        <div v-for="g in grouped" :key="g.ym" class="cl-archive-group">
          <h2 class="cl-archive-year">
            <button type="button" class="cl-archive-btn" @click="jumpTo(g.ym)">{{ g.ym }}</button>
            <span class="cl-archive-count">{{ g.items.length }} {{ $t('篇') }}</span>
          </h2>
          <ul :id="'cl-ym-' + g.ym" class="cl-archive-list">
            <li v-for="p in g.items" :key="p.token" class="cl-archive-item">
              <span class="cl-archive-date">{{ dateOf(p) }}</span>
              <a :href="hrefOf(p)" class="cl-archive-link">{{ titleOf(p) }}</a>
            </li>
          </ul>
        </div>
      </div>
      <p v-else class="cl-empty">{{ $t('暂无文章') }}</p>
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
const archiveLabel = computed(() => page.value.archive || '')
const posts = computed(() => ctx.value.posts || [])

/* 按年月分组 */
const grouped = computed(() => {
  const map = {}
  for (const p of posts.value) {
    const v = p.file?.updated_at
    if (!v) continue
    const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
    if (!Number.isFinite(ms)) continue
    const d = new Date(ms)
    const ym = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
    if (archiveLabel.value && ym !== archiveLabel.value) continue
    if (!map[ym]) map[ym] = []
    map[ym].push(p)
  }
  return Object.entries(map)
    .sort(([a], [b]) => b.localeCompare(a))
    .map(([ym, items]) => ({ ym, items }))
})

function jumpTo(ym) {
  const el = document.getElementById('cl-ym-' + ym)
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
function titleOf(p) { const n = p.file?.name || p.path || ''; return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop() || t('common.unnamed') }
function dateOf(p) { const v = p.file?.updated_at; if (!v) return ''; const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v); if (!Number.isFinite(ms)) return ''; const d = new Date(ms); const z = (n) => String(n).padStart(2,'0'); return `${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}` }
function hrefOf(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '#' }
</script>