<template>
  <div class="cl-root">
    <header class="cl-header">
      <div class="cl-header-in">
        <a class="cl-brand" :href="homeHref"><span class="cl-logo-icon">C</span><span class="cl-logo-text">{{ siteName }}</span></a>
        <nav class="cl-nav"><a :href="homeHref" class="cl-nav-a">{{ $t('首页') }}</a><ThemeSwitch /></nav>
      </div>
    </header>
    <main class="cl-inner-main">
      <h1 class="cl-inner-title">{{ $t('搜索') }}</h1>
      <div class="cl-search-box">
        <input
          v-model="keyword"
          class="cl-search-input"
          type="text"
          :placeholder="$t('输入关键词搜索文章标题和摘要…')"
          @keydown.enter="doSearch"
        />
        <button class="cl-search-go" @click="doSearch">{{ $t('搜索') }}</button>
      </div>

      <div v-if="!q" class="cl-state">
        <div class="cl-state-icon">🔍</div>
        <p>{{ $t('输入关键词开始检索') }}</p>
      </div>
      <div v-else-if="!results.length" class="cl-state">
        <div class="cl-state-icon">😕</div>
        <p>{{ $t('未找到与「') }}{{ q }}{{ $t('」相关的文章') }}</p>
      </div>
      <div v-else>
        <p class="cl-inner-count">{{ $t('找到') }} {{ results.length }} {{ $t('篇相关文章') }}</p>
        <div class="cl-list">
          <article v-for="p in results" :key="p.token" class="cl-list-item">
            <a :href="hrefOf(p)" class="cl-list-link">
              <div class="cl-list-thumb" style="background:linear-gradient(135deg,#6c5ce7,#a29bfe)"><span class="cl-list-emoji">🔍</span></div>
              <div class="cl-list-body">
                <h3 class="cl-list-title">{{ titleOf(p) }}</h3>
                <p class="cl-list-excerpt" v-if="p.preview">{{ cleanPreview(p.preview) }}</p>
                <div class="cl-list-meta"><span class="cl-meta-date">{{ dateOf(p) }}</span></div>
              </div>
            </a>
          </article>
        </div>
      </div>
    </main>
    <footer class="cl-footer"><div class="cl-footer-in"><p class="cl-footer-copy">© {{ currentYear }} {{ siteName }}</p></div></footer>
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { ref, computed, inject, watch } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], page: {} })
const siteName = computed(() => ctx.value.siteName || 'ClawBlog')
const currentYear = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
const page = computed(() => ctx.value.page || {})
const q = computed(() => page.value.q || '')
const keyword = ref(q.value || '')

watch(q, (v) => { keyword.value = v })

function doSearch() {
  const k = keyword.value.trim()
  if (k) window.location.href = `/app#/blog/search?q=${encodeURIComponent(k)}`
}

const results = computed(() => {
  const k = q.value.trim().toLowerCase()
  if (!k) return []
  return (ctx.value.posts || []).filter((p) => {
    const title = titleOf(p).toLowerCase()
    const preview = (p.preview || '').toLowerCase()
    return title.includes(k) || preview.includes(k)
  })
})

function titleOf(p) { const n = p.file?.name || p.path || ''; return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop() || t('common.unnamed') }
function dateOf(p) { const v = p.file?.updated_at; if (!v) return ''; const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v); if (!Number.isFinite(ms)) return ''; const d = new Date(ms); const z = (n) => String(n).padStart(2,'0'); return `${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}` }
function hrefOf(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '#' }
function cleanPreview(s) { return String(s || '').replace(/^#+\s*/gm, '').replace(/[*_`~]/g, '').slice(0, 120) }
</script>