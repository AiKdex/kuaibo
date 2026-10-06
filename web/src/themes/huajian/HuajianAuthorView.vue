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
        <input v-model="searchQuery" class="hj-search-input" type="text" :placeholder="$t('输入关键词搜索文章…')" @keydown.enter="goSearch" />
        <button class="hj-btn hj-btn-amber" @click="goSearch">{{ $t('搜索') }}</button>
      </div>
    </div>

    <main class="hj-inner-main">
      <h1 class="hj-inner-title">{{ $t('作者：') }}{{ author }}</h1>
      <p class="hj-inner-count">{{ list.length }} {{ $t('篇') }}</p>

      <!-- 状态处理：加载中 / 错误 / 空态 -->
      <div v-if="loading" class="hj-state">
        <div class="hj-state-icon">⏳</div>
        <p>{{ $t('正在加载…') }}</p>
      </div>
      <div v-else-if="error" class="hj-state hj-state-error">
        <div class="hj-state-icon">⚠️</div>
        <p>{{ error }}</p>
      </div>
      <template v-else>
        <div v-if="list.length" class="hj-post-list">
          <article v-for="(p, i) in list" :key="p.token" class="hj-post-card">
            <span class="hj-post-num">{{ String(i + 1).padStart(2, '0') }}</span>
            <div class="hj-post-body">
              <a :href="hrefOf(p)"><h2 class="hj-post-title">{{ titleOf(p) }}</h2></a>
              <div class="hj-post-meta">
                <span>{{ dateOf(p) }}</span>
                <span v-if="catOf(p)" class="hj-meta-dot"></span>
                <span v-if="catOf(p)">{{ catOf(p) }}</span>
              </div>
            </div>
          </article>
        </div>
        <p v-else class="hj-empty">{{ $t('该作者下暂无文章') }}</p>
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
import { ref, computed, inject } from 'vue'
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
const author = computed(() => page.value.author || '')
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')

/* 作者页数据由宿主过滤后 provide；幂等兜底：仅当 posts 混入他人文章时才自行过滤 */
const list = computed(() => {
  if (!author.value) return ctx.value.posts || []
  const all = ctx.value.posts || []
  const hit = all.filter((p) => (p.file?.author || '') === author.value)
  return hit.length === all.length ? all : hit
})

const showSearch = ref(false)
const searchQuery = ref('')
const searchLink = ref(null)
const searchHref = computed(() => {
  const q = searchQuery.value.trim()
  return q ? `/app#/blog/search?q=${encodeURIComponent(q)}` : '/app#/blog/search'
})

function goSearch() {
  const q = searchQuery.value.trim()
  if (q) searchLink.value?.click()
}

function titleOf(p) { const n = p.file?.name || p.path || ''; return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('common.unnamed') }
function catOf(p) { const path = p.path || ''; return path.includes('/') ? path.split('/')[0] : '' }
function dateOf(p) { const v = p.file?.updated_at; if (!v) return ''; const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v); if (!Number.isFinite(ms)) return ''; const d = new Date(ms); const z = (n) => String(n).padStart(2, '0'); return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}` }
function hrefOf(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '/app#/blog?view=public' }
</script>
