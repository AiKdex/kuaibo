<template>
  <div class="ak-root">
    <header class="ak-top">
      <div class="ak-top-inner">
        <a class="ak-brand" :href="publicHref">{{ $t('爱库录') }}<em>.</em></a>
        <span class="ak-tag">AiKlog · aiklog.com</span>
        <nav class="ak-top-links">
          <ThemeSwitch />
          <a :href="publicHref">{{ $t('文章') }}</a>
          <a :href="rssHref" target="_blank" rel="noopener">RSS</a>
          <a href="https://github.com/AiKdex/AiKlog" target="_blank" rel="noopener">GitHub</a>
        </nav>
      </div>
    </header>

    <section class="ak-hero">
      <div class="ak-hero-kicker">AI Knowledge Blog System</div>
      <h1>{{ $t('目录即站点') }}<br />{{ $t('文件即文章') }}</h1>
      <p>
        爱库录（AiKlog）· 自部署 AI 知识库博客。
        拖拽即发、语义检索、双链互文；数据在自己手里。
      </p>
      <div class="ak-hero-meta">
        <span class="ak-chip">{{ $t('目录即博客') }}</span>
        <span class="ak-chip">{{ $t('插件协议') }}</span>
        <span class="ak-chip">{{ $t('自部署') }}</span>
        <span v-if="ctx.siteName">{{ ctx.siteName }}</span>
      </div>
    </section>

    <div class="ak-layout" :class="{ 'ak-layout--side-left': sideLeft }">
      <aside class="ak-side">
        <section class="ak-side-card">
          <h3 class="ak-side-title">
            {{ $t('关于') }}
            <button type="button" class="ak-side-toggle" :title="sideLeft ? $t('侧栏移到右侧') : $t('侧栏移到左侧')" @click="toggleSide">
              {{ sideLeft ? $t("侧栏 → 右") : $t("侧栏 ← 左") }}
            </button>
          </h3>
          <p class="ak-side-text">
            爱库录（AiKlog）· 自部署 AI 知识库博客系统。
            目录即站点，文件即文章。
          </p>
          <div class="ak-side-links">
            <a :href="rssHref" target="_blank" rel="noopener">RSS</a>
            <a href="https://github.com/AiKdex/AiKlog" target="_blank" rel="noopener">GitHub</a>
            <a href="https://aiklog.com">{{ $t('官网') }}</a>
          </div>
        </section>

        <section v-if="catList.length" class="ak-side-card">
          <h3 class="ak-side-title">{{ $t('分类') }}</h3>
          <ul class="ak-side-list">
            <li v-for="c in catList" :key="c.name">
              <button type="button" class="ak-side-btn" :class="{ on: filterCat === c.name }" @click="toggleCat(c.name)">
                <span>{{ c.name }}</span>
                <span class="ak-side-n">{{ c.n }}</span>
              </button>
            </li>
          </ul>
        </section>

        <section v-if="monthList.length" class="ak-side-card">
          <h3 class="ak-side-title">{{ $t('归档') }}</h3>
          <ul class="ak-side-list">
            <li v-for="m in monthList" :key="m.key">
              <button type="button" class="ak-side-btn" :class="{ on: filterMonth === m.key }" @click="toggleMonth(m.key)">
                <span>{{ m.label }}</span>
                <span class="ak-side-n">{{ m.n }}</span>
              </button>
            </li>
          </ul>
        </section>
      </aside>

      <main class="ak-list-wrap">
        <div class="ak-list-head">
          <h2>{{ listTitle }}</h2>
          <div class="ak-list-tools">
            <button v-if="filterCat || filterMonth" type="button" class="ak-filter-clear" @click="clearFilter">{{ $t('清除筛选') }}</button>
            <span class="ak-list-count">{{ shownPosts.length }} {{ $t('篇') }}</span>
          </div>
        </div>

        <div v-if="ctx.loading" class="ak-state">
          <strong>{{ $t('加载中') }}</strong>
          {{ $t('正在读取公开文章…') }}
        </div>

        <div v-else-if="ctx.error" class="ak-state">
          <strong>{{ $t('加载失败') }}</strong>
          {{ ctx.error }}
        </div>

        <div v-else-if="!shownPosts.length" class="ak-state">
          <strong>{{ $t('还没有公开文章') }}</strong>{{ $t("在知识库「博客」目录放入 Markdown 并发布后，这里会自动出现。") }}
        </div>

        <template v-else>
          <a
            v-for="(p, i) in shownPosts"
            :key="p.token + (p.path || '') + i"
            class="ak-card"
            :href="postHref(p)"
          >
            <div v-if="postCover(p)" class="ak-card-media">
              <img :src="postCover(p)" :alt="postTitle(p)" loading="lazy" />
            </div>
            <div v-else class="ak-card-media ak-card-media--ph" aria-hidden="true">
              <span>{{ (postCat(p) || $t("文")).slice(0, 1) }}</span>
            </div>
            <div class="ak-card-body">
              <div class="ak-card-meta">
                <span v-if="nodeTypeOf(p.file)" class="ak-card-type" :title="$t('内容类型：') + nodeTypeOf(p.file)">{{ nodeTypeLabel(nodeTypeOf(p.file)) }}</span>
                <span v-if="postCat(p)" class="ak-card-cat">{{ postCat(p) }}</span>
                <span v-if="postDate(p)">{{ postDate(p) }}</span>
                <span v-if="p.file?.size">{{ formatSize(p.file.size) }}</span>
              </div>
              <h3 class="ak-card-title">{{ postTitle(p) }}</h3>
              <p v-if="p.preview" class="ak-card-preview">{{ cleanPreview(p.preview) }}</p>
              <span class="ak-card-idx">{{ String(i + 1).padStart(2, '0') }}</span>
            </div>
          </a>
        </template>
      </main>
    </div>

    <footer class="ak-foot">
      <span>爱库录 AI 知识库博客系统 · 样板房</span>
      <span>
        <a href="https://aiklog.com">aiklog.com</a>
        ·
        <a :href="rssHref" target="_blank" rel="noopener">RSS</a>
      </span>
    </footer>
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, inject, ref } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import { nodeTypeOf, nodeTypeLabel } from '@/utils/contentState'
import './style.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || {
  siteName: '爱库录',
  posts: [],
  loading: true,
  error: '',
  postUrl: () => '#'
})

const posts = computed(() => ctx.value.posts || [])
const publicHref = '#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'

const filterCat = ref('')
const filterMonth = ref('')

// 侧栏位置：默认右侧；localStorage 记忆（aiklog.sidebar=left|right）
const SIDE_KEY = 'aiklog.sidebar'
const sideLeft = ref(false)
try {
  sideLeft.value = localStorage.getItem(SIDE_KEY) === 'left'
} catch (_) { /* ignore */ }

function toggleSide() {
  sideLeft.value = !sideLeft.value
  try {
    localStorage.setItem(SIDE_KEY, sideLeft.value ? 'left' : 'right')
  } catch (_) { /* ignore */ }
}

function monthKey(p) {
  const v = p.file?.updated_at || p.created_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

const catList = computed(() => {
  const m = new Map()
  for (const p of posts.value) {
    const c = postCat(p)
    if (!c) continue
    m.set(c, (m.get(c) || 0) + 1)
  }
  return [...m.entries()].map(([name, n]) => ({ name, n })).sort((a, b) => b.n - a.n)
})

const monthList = computed(() => {
  const m = new Map()
  for (const p of posts.value) {
    const k = monthKey(p)
    if (!k) continue
    m.set(k, (m.get(k) || 0) + 1)
  }
  return [...m.entries()]
    .map(([key, n]) => ({ key, n, label: key }))
    .sort((a, b) => (a.key < b.key ? 1 : -1))
    .slice(0, 12)
})

const shownPosts = computed(() => {
  return posts.value.filter((p) => {
    if (filterCat.value && postCat(p) !== filterCat.value) return false
    if (filterMonth.value && monthKey(p) !== filterMonth.value) return false
    return true
  })
})

const listTitle = computed(() => {
  if (filterCat.value) return filterCat.value
  if (filterMonth.value) return filterMonth.value
  return '已发布'
})

function toggleCat(name) {
  filterCat.value = filterCat.value === name ? '' : name
}
function toggleMonth(key) {
  filterMonth.value = filterMonth.value === key ? '' : key
}
function clearFilter() {
  filterCat.value = ''
  filterMonth.value = ''
}

function postHref(p) {
  try {
    return ctx.value.postUrl ? ctx.value.postUrl(p) : `#/p/${p.token}`
  } catch {
    return `#/p/${p?.token || ''}`
  }
}

function postCover(p) {
  const s = p.preview || ''
  const m = /!\[[^\]]*\]\((https?:\/\/[^)\s]+|\/[^)\s]+)\)/.exec(s)
  return m ? m[1] : ''
}

function postTitle(p) {
  const name = p.file?.name || p.name || ''
  if (name) return name.replace(/\.(md|markdown)$/i, '')
  const path = p.path || ''
  if (path) {
    const base = path.split('/').pop() || path
    return base.replace(/\.(md|markdown)$/i, '')
  }
  return p.title || t('common.unnamed')
}

function postCat(p) {
  const path = p.path || p.file?.path || ''
  return path.includes('/') ? path.split('/')[0] : ''
}

function postDate(p) {
  const v = p.file?.updated_at || p.created_at || p.file?.created_at
  return formatDate(v)
}

function cleanPreview(s) {
  return String(s || '')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/^（降级摘要）文件名：[^\n#]+/m, '')
    .replace(/^#\s+[^\n]+\n?/m, '')
    .trim()
}

function formatDate(v) {
  if (!v) return ''
  // 秒或毫秒时间戳 / ISO
  let ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return String(v)
  const d = new Date(ms)
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function formatSize(n) {
  const x = Number(n)
  if (!Number.isFinite(x) || x <= 0) return ''
  if (x < 1024) return `${x} B`
  if (x < 1024 * 1024) return `${(x / 1024).toFixed(1)} KB`
  return `${(x / 1024 / 1024).toFixed(1)} MB`
}
</script>
