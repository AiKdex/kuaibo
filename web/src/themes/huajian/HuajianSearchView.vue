<template>
  <div class="hj-root">
    <div class="hj-topline"></div>

    <!-- 页头（与列表页同构） -->
    <header class="hj-hdr">
      <div class="hj-hdr-in">
        <a class="hj-brand" :href="homeHref">
          <span class="hj-brand-name">{{ siteName }}</span>
          <span class="hj-brand-dot"></span>
        </a>
        <nav class="hj-nav">
          <a :href="homeHref">{{ $t('首页') }}</a>
          <a :href="archiveHref">{{ $t('归档') }}</a>
          <a :href="tagHref">{{ $t('标签') }}</a>
          <ThemeSwitch />
        </nav>
        <div class="hj-hdr-right">
          <button class="hj-icon-btn" @click="showSearch = !showSearch" :title="$t('搜索')" :aria-label="$t('搜索')">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7"></circle><line x1="21" y1="21" x2="16.5" y2="16.5"></line></svg>
          </button>
          <a class="hj-btn hj-btn-amber" :href="rssHref">RSS</a>
          <a ref="searchLink" :href="searchHref" style="display:none" aria-hidden="true"></a>
        </div>
      </div>
    </header>

    <!-- 搜索栏（展开/收起） -->
    <div v-if="showSearch" class="hj-search-bar">
      <div class="hj-search-in">
        <input v-model="keyword" class="hj-search-input" type="text" :placeholder="$t('输入关键词搜索文章标题和摘要…')" @keydown.enter="doSearch" />
        <button class="hj-btn hj-btn-amber" @click="doSearch">{{ $t('搜索') }}</button>
      </div>
    </div>

    <main class="hj-inner-main">
      <h1 class="hj-inner-title">{{ $t('搜索') }}</h1>
      <div class="hj-search-box">
        <input v-model="keyword" class="hj-search-input" type="text" :placeholder="$t('输入关键词搜索文章标题和摘要…')" @keydown.enter="doSearch" />
        <button class="hj-btn hj-btn-amber" @click="doSearch">{{ $t('搜索') }}</button>
      </div>

      <!-- 状态处理：加载中 / 错误 -->
      <div v-if="loading" class="hj-state">
        <div class="hj-state-icon">⏳</div>
        <p>{{ $t('正在加载…') }}</p>
      </div>
      <div v-else-if="error" class="hj-state hj-state-error">
        <div class="hj-state-icon">⚠️</div>
        <p>{{ error }}</p>
      </div>
      <template v-else>
        <div v-if="!q" class="hj-state">
          <div class="hj-state-icon">🔍</div>
          <p>{{ $t('输入关键词开始检索') }}</p>
        </div>
        <div v-else-if="!results.length" class="hj-state">
          <div class="hj-state-icon">😕</div>
          <p>{{ $t('未找到与「') }}{{ q }}{{ $t('」相关的文章') }}</p>
        </div>
        <div v-else>
          <p class="hj-inner-count">{{ $t('找到') }} {{ results.length }} {{ $t('篇相关文章') }}</p>
          <div class="hj-post-list">
            <article v-for="(p, i) in results" :key="p.token" class="hj-post-card">
              <span class="hj-post-num">{{ String(i + 1).padStart(2, '0') }}</span>
              <div class="hj-post-body">
                <a :href="hrefOf(p)"><h2 class="hj-post-title">{{ titleOf(p) }}</h2></a>
                <p v-if="p.preview" class="hj-post-excerpt">{{ cleanPreview(p.preview) }}</p>
                <div class="hj-post-meta">
                  <span>{{ dateOf(p) }}</span>
                  <span v-if="catOf(p)" class="hj-meta-dot"></span>
                  <span v-if="catOf(p)">{{ catOf(p) }}</span>
                </div>
              </div>
            </article>
          </div>
        </div>
      </template>
    </main>

    <footer class="hj-ft">
      <div class="hj-ft-in">
        <div class="hj-ft-bottom" style="border-top:none;padding-top:0">
          <span>© {{ currentYear }} {{ siteName }}</span>
          <span>Powered by AiKlog</span>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup>
import { t } from '@/i18n'
import { ref, computed, inject, watch } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], page: {} })
const siteName = computed(() => ctx.value.siteName || '花笺')
const currentYear = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
const archiveHref = '/app#/blog/archive'
const tagHref = '/app#/blog/tag'
const rssHref = '/api/v1/blog/feed.xml'
const page = computed(() => ctx.value.page || {})
const q = computed(() => page.value.q || '')
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const keyword = ref(q.value || '')

watch(q, (v) => { keyword.value = v })

const searchLink = ref(null)
const searchHref = computed(() => {
  const k = keyword.value.trim()
  return k ? `/app#/blog/search?q=${encodeURIComponent(k)}` : '/app#/blog/search'
})

function doSearch() {
  const k = keyword.value.trim()
  if (k) searchLink.value?.click()
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

function titleOf(p) { const n = p.file?.name || p.path || ''; return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('common.unnamed') }
function catOf(p) { const path = p.path || ''; return path.includes('/') ? path.split('/')[0] : '' }
function dateOf(p) { const v = p.file?.updated_at; if (!v) return ''; const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v); if (!Number.isFinite(ms)) return ''; const d = new Date(ms); const z = (n) => String(n).padStart(2, '0'); return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}` }
function hrefOf(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '/app#/blog?view=public' }
function cleanPreview(s) { return String(s || '').replace(/^#+\s*/gm, '').replace(/[*_`~]/g, '').slice(0, 120) }
</script>
