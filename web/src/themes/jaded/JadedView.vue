<template>
  <div class="jd-root">
    <!-- Ambient Background -->
    <div class="jd-bg-ambient"></div>
    <div class="jd-bg-noise"></div>

    <!-- Top Bar -->
    <header class="jd-topbar">
      <div class="jd-topbar-in">
        <a class="jd-logo" :href="homeHref">
          <span class="jd-logo-dot">●</span>
          <span>{{ ctx.siteName || 'AiKlog' }}</span>
          <span class="jd-logo-sub">{{ $t('爱库录') }}</span>
        </a>
        <nav class="jd-nav">
          <a :href="homeHref" class="jd-nav-a jd-active">{{ $t('博客') }}</a>
          <a :href="archiveHref" class="jd-nav-a">{{ $t('归档') }}</a>
          <a :href="rssHref" class="jd-nav-a">RSS</a>
        </nav>
        <div class="jd-nav-right">
          <div class="jd-search-trigger" @click="showSearch = true">
            {{ $t('🔍 搜索文章...') }}
          </div>
          <ThemeSwitch />
        </div>
      </div>
    </header>

    <!-- Search Modal -->
    <div v-if="showSearch" class="jd-search-overlay" @click.self="showSearch = false">
      <div class="jd-search-modal">
        <input
          ref="searchInputRef"
          v-model="searchQ"
          class="jd-search-input"
          :placeholder="$t('搜索文章标题、摘要...')"
          @keyup.enter="doSearch"
          @keyup.escape="showSearch = false"
        />
        <div v-if="searchResults.length" class="jd-search-results">
          <a
            v-for="p in searchResults"
            :key="p.token"
            class="jd-search-item"
            :href="postHref(p)"
          >
            <span class="jd-search-item-title">{{ postTitle(p) }}</span>
            <span class="jd-search-item-cat">{{ postCat(p) || $t("未分类") }}</span>
          </a>
        </div>
        <p v-else-if="searchQ.trim()" class="jd-search-empty">{{ $t('未找到匹配文章') }}</p>
      </div>
    </div>

    <!-- Hero -->
    <section class="jd-hero">
      <h1 class="jd-hero-h1">
        <span class="jd-hero-line">{{ $t('目录即站点') }}</span>
        <span class="jd-hero-line">{{ $t('文件即文章') }}</span>
      </h1>
      <p class="jd-hero-p">{{ ctx.siteDesc || $t("发条消息就是一篇文章。AI 自动整理发布。") }}</p>
      <div class="jd-hero-meta">
        <span>📝 <strong>{{ ctx.posts?.length || 0 }}</strong> {{ $t('篇文章') }}</span>
        <span>📂 <strong>{{ cats.length }}</strong> {{ $t('个分类') }}</span>
      </div>
    </section>

    <!-- Loading / Error / Empty -->
    <div v-if="loading" class="jd-status">
      <p>{{ $t('⏳ 加载中...') }}</p>
    </div>
    <div v-else-if="error" class="jd-status">
      <p>❌ {{ error }}</p>
    </div>
    <div v-else-if="!ctx.posts?.length" class="jd-status">
      <p>{{ $t('📭 还没有公开文章') }}</p>
    </div>

    <!-- Content -->
    <div v-else class="jd-content">
      <div class="jd-wrapper">

        <!-- Main -->
        <main class="jd-main">

          <!-- Featured (first post) -->
          <a
            v-if="featured"
            class="jd-featured"
            :href="postHref(featured)"
          >
            <PostCover :post="featured" round>
              <div class="jd-featured-img" :style="featStyle(featured)">
                <span class="jd-featured-emoji">{{ catEmoji(postCat(featured)) }}</span>
              </div>
            </PostCover>
            <div class="jd-featured-body">
              <div class="jd-card-tags">
                <span class="jd-chip">{{ postCat(featured) || $t("未分类") }}</span>
              </div>
              <h2 class="jd-featured-h2">{{ postTitle(featured) }}</h2>
              <p class="jd-featured-desc">{{ featured.preview || $t("暂无摘要") }}</p>
              <div class="jd-card-foot">
                <span>📅 {{ postDate(featured) }}</span>
              </div>
            </div>
          </a>

          <!-- Post Cards -->
          <a
            v-for="p in restPosts"
            :key="p.token"
            class="jd-card"
            :href="postHref(p)"
          >
            <PostCover :post="p" round>
              <div class="jd-card-img" :style="cardStyle(p)">
                <span class="jd-card-emoji">{{ catEmoji(postCat(p)) }}</span>
              </div>
            </PostCover>
            <div class="jd-card-body">
              <div class="jd-card-tags">
                <span class="jd-chip">{{ postCat(p) || $t("未分类") }}</span>
              </div>
              <h2 class="jd-card-h2">{{ postTitle(p) }}</h2>
              <p class="jd-card-desc">{{ p.preview || $t("暂无摘要") }}</p>
              <div class="jd-card-foot">
                <span>📅 {{ postDate(p) }}</span>
              </div>
            </div>
          </a>

        </main>

        <!-- Sidebar -->
        <aside class="jd-side">

          <!-- About -->
          <div class="jd-sb jd-sb-about">
            <div class="jd-avatar">{{ avatarLetter }}</div>
            <h3 class="jd-sb-name">{{ ctx.siteName || '爱库录' }}</h3>
            <p class="jd-sb-desc">{{ ctx.siteDesc || $t('目录即站点，文件即文章。') }}</p>
            <div class="jd-sb-stats">
              <div class="jd-stat">
                <strong>{{ ctx.posts?.length || 0 }}</strong>
                <span>{{ $t('文章') }}</span>
              </div>
              <div class="jd-stat">
                <strong>{{ cats.length }}</strong>
                <span>{{ $t('分类') }}</span>
              </div>
            </div>
          </div>

          <!-- AI Search -->
          <div class="jd-sb jd-sb-ai">
            <div class="jd-sb-title">{{ $t('🤖 AI 问答') }}</div>
            <div class="jd-ai-box">
              <input
                v-model="aiQ"
                class="jd-ai-input"
                :placeholder="$t('问点什么...')"
                @keyup.enter="askAI"
              />
              <button class="jd-ai-btn" @click="askAI" :disabled="aiLoading || !aiQ.trim()">
                {{ aiLoading ? '...' : $t("问") }}
              </button>
            </div>
            <div v-if="aiReply" class="jd-ai-reply">{{ aiReply }}</div>
            <div v-if="aiError" class="jd-ai-error">{{ aiError }}</div>
            <div class="jd-ai-features">
              <div class="jd-ai-feat"><span class="jd-ai-dot"></span> {{ $t('基于全站文章语义理解') }}</div>
              <div class="jd-ai-feat"><span class="jd-ai-dot"></span> {{ $t('自动引用相关文章') }}</div>
            </div>
          </div>

          <!-- Tags (hidden when empty per spec §4.6) -->
          <div v-if="ctx.tags && ctx.tags.length" class="jd-sb">
            <div class="jd-sb-title">{{ $t('🏷️ 标签云') }}</div>
            <div class="jd-tag-cloud">
              <span v-for="t in ctx.tags" :key="t.name" class="jd-tag-item">{{ t.name }}</span>
            </div>
          </div>

          <!-- Archives -->
          <div v-if="archives.length" class="jd-sb">
            <div class="jd-sb-title">{{ $t('📅 归档') }}</div>
            <ul class="jd-archive-list">
              <li v-for="a in archives" :key="a.label">
                <button type="button" class="jd-archive-link" @click="jumpArchive(a.label)">
                  {{ a.label }}
                </button>
                <span class="jd-archive-cnt">{{ a.count }}</span>
              </li>
            </ul>
          </div>

          <!-- Links -->
          <div class="jd-sb">
            <div class="jd-sb-title">{{ $t('🔗 链接') }}</div>
            <div class="jd-sb-links">
              <a :href="rssHref">{{ $t('📡 RSS 订阅') }}</a>
              <a :href="homeHref">{{ $t('🏠 首页') }}</a>
            </div>
          </div>

        </aside>

      </div>
    </div>

    <!-- Footer -->
    <footer class="jd-footer">
      <div class="jd-footer-brand">
        <span class="jd-logo-dot">●</span> {{ ctx.siteName || 'AiKlog' }}
      </div>
      <div class="jd-footer-links">
        <a :href="homeHref">{{ $t('首页') }}</a>
        <a :href="rssHref">RSS</a>
      </div>
      <p class="jd-footer-copy">© {{ year }} {{ ctx.siteName || 'AiKlog' }} · Powered by AiKlog</p>
    </footer>
  </div>
    <AskWidget />
</template>

<script setup>
import { t as i18t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { ref, computed, inject, nextTick } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import PostCover from '@/components/PostCover.vue'
import './style.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], tags: [], loading: true })
const loading = computed(() => ctx.value.loading)
const error = computed(() => ctx.value.error || '')

const year = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'
const archiveHref = '/app#/blog?view=public'

/* ── helpers ─────────────────────────────── */
function postTitle(p) {
  const n = p.file?.name || p.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || i18t('common.unnamed')
}
function postCat(p) {
  const path = p.path || ''
  return path.includes('/') ? path.split('/')[0] : ''
}
function postDate(p) {
  const v = p.file?.updated_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  const z = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}`
}
function postHref(p) {
  return ctx.value.postUrl ? ctx.value.postUrl(p) : '#'
}

/* ── categories ──────────────────────────── */
const cats = computed(() => {
  const map = {}
  for (const p of (ctx.value.posts || [])) {
    const c = postCat(p)
    map[c] = (map[c] || 0) + 1
  }
  return Object.entries(map).map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count)
})

const avatarLetter = computed(() => (ctx.value.siteName || 'A')[0].toUpperCase())

/* ── post lists ──────────────────────────── */
const featured = computed(() => ctx.value.posts?.[0] || null)
const restPosts = computed(() => (ctx.value.posts || []).slice(1))

/* ── archives ────────────────────────────── */
const archives = computed(() => {
  const map = {}
  for (const p of (ctx.value.posts || [])) {
    const d = postDate(p)
    if (!d) continue
    const ym = d.slice(0, 7)
    map[ym] = (map[ym] || 0) + 1
  }
  return Object.entries(map)
    .map(([label, count]) => ({ label: label.replace('-', ' 年 ') + ' 月', count, raw: label }))
    .sort((a, b) => b.raw.localeCompare(a.raw))
})
function jumpArchive(label) {
  // scroll to section if we had archive view — for now just a placeholder
}

/* ── gradient styles per category ────────── */
const gradients = [
  'linear-gradient(135deg, #1a3a4a, #0d2233)',
  'linear-gradient(135deg, #2a1a3a, #1a0d33)',
  'linear-gradient(135deg, #1a2a1a, #0d330d)',
  'linear-gradient(135deg, #2a2a1a, #332a0d)',
  'linear-gradient(135deg, #1a1a2a, #0d0d33)',
  'linear-gradient(135deg, #2a1a1a, #330d0d)',
  'linear-gradient(135deg, #1a2a2a, #0d3333)',
]
function gradientFor(name) {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = ((hash << 5) - hash + name.charCodeAt(i)) | 0
  return gradients[Math.abs(hash) % gradients.length]
}
const catEmojis = {}
const emojis = ['📁', '🔧', '🤖', '🎨', '⚡', '📝', '🚀', '💡', '📊', '🐳', '📱', '🔗', '🔐', '🎯']
function catEmoji(name) {
  if (!catEmojis[name]) {
    const idx = cats.value.findIndex((c) => c.name === name)
    catEmojis[name] = emojis[(idx >= 0 ? idx : 0) % emojis.length]
  }
  return catEmojis[name]
}
function featStyle(p) {
  return { background: gradientFor(postCat(p)) }
}
function cardStyle(p) {
  return { background: gradientFor(postCat(p)) }
}

/* ── search ──────────────────────────────── */
const showSearch = ref(false)
const searchQ = ref('')
const searchInputRef = ref(null)
const searchResults = computed(() => {
  const q = searchQ.value.trim().toLowerCase()
  if (!q) return []
  return (ctx.value.posts || []).filter((p) => {
    const t = postTitle(p).toLowerCase()
    const prev = (p.preview || '').toLowerCase()
    return t.includes(q) || prev.includes(q)
  }).slice(0, 10)
})
function doSearch() { /* results are reactive */ }

/* ── AI Q&A ──────────────────────────────── */
const aiQ = ref('')
const aiReply = ref('')
const aiError = ref('')
const aiLoading = ref(false)
async function askAI() {
  const q = aiQ.value.trim()
  if (!q || aiLoading.value) return
  aiLoading.value = true
  aiReply.value = ''
  aiError.value = ''
  try {
    const resp = await fetch('/api/v1/public/blog/ask', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question: q }),
    })
    if (resp.status === 429) {
      aiError.value = i18t('提问太频繁，请稍后再试（每 5 分钟 8 次）')
      return
    }
    const data = await resp.json()
    aiReply.value = data.reply || i18t('暂无回答')
  } catch (e) {
    aiError.value = i18t('请求失败，请稍后重试')
  } finally {
    aiLoading.value = false
  }
}

/* ── watch search modal ──────────────────── */
import { watch } from 'vue'
watch(showSearch, (v) => {
  if (v) nextTick(() => searchInputRef.value?.focus())
})
</script>