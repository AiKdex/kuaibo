<template>
  <div class="parchment-root">
    <!-- Header -->
    <header class="pa-header">
      <div class="pa-header-in">
        <div class="pa-brand">
          <div class="pa-name">{{ siteName || '爱库录' }}</div>
          <p>{{ siteDesc || $t("目录即站点，文件即文章") }}</p>
        </div>
        <nav class="pa-nav">
          <ThemeSwitch />
          <a :href="blogUrl" class="active">{{ $t('文章') }}</a>
          <a href="/api/v1/blog/feed.xml">RSS</a>
        </nav>
      </div>
    </header>

    <!-- Featured Post -->
    <a v-if="featured" :href="postUrl(featured)" class="pa-feat">
      <PostCover :post="featured" round>
        <div class="pa-feat-img">
          <div class="pa-feat-ph" :style="getGradient(featured)">{{ getEmoji(featured) }}</div>
        </div>
      </PostCover>
      <div class="pa-feat-body">
        <span v-if="getCategory(featured)" class="pa-cat">{{ getCategory(featured) }}</span>
        <h2>{{ getTitle(featured) }}</h2>
        <p>{{ getSummary(featured) }}</p>
        <div class="pa-meta">
          <span>{{ siteName || '爱库录' }}</span>
          <span class="pa-dot">·</span>
          <span>{{ formatDate(featured) }}</span>
        </div>
      </div>
    </a>

    <!-- Content Grid -->
    <div class="pa-wrap">
      <main class="pa-main">
        <div class="pa-grid">
          <a
            v-for="(post, i) in restPosts"
            :key="post.token || i"
            :href="postUrl(post)"
            class="pa-card"
          >
            <PostCover :post="post" round>
              <div class="pa-card-img">
                <div class="pa-card-ph" :style="getGradient(post)">{{ getEmoji(post) }}</div>
              </div>
            </PostCover>
            <div class="pa-card-body">
              <span v-if="getCategory(post)" class="pa-cat-sm">{{ getCategory(post) }}</span>
              <h3>{{ getTitle(post) }}</h3>
              <p>{{ getSummary(post) }}</p>
              <div class="pa-card-meta">
                <span>{{ formatDate(post) }}</span>
              </div>
            </div>
          </a>
        </div>
      </main>

      <!-- Sidebar -->
      <aside class="pa-side">
        <div class="sb">
          <div class="sb-about">
            <div class="sb-avatar">{{ (siteName || 'A')[0] }}</div>
            <h3>{{ siteName || '爱库录' }}</h3>
            <p>{{ siteDesc || $t("目录即站点，文件即文章") }}</p>
          </div>
        </div>

        <div class="sb sb-ai">
          <h4>{{ $t('🤖 AI 问答') }}</h4>
          <div class="sb-ai-box">
            <input v-model="aiQuery" :placeholder="$t('问点什么...')" @keyup.enter="askAI" />
            <button @click="askAI">{{ $t('问') }}</button>
          </div>
          <p v-if="aiAnswer" class="sb-ai-ans">{{ aiAnswer }}</p>
        </div>

        <div v-if="tags && tags.length" class="sb">
          <h4>{{ $t('标签') }}</h4>
          <div class="sb-tags">
            <a v-for="t in tags" :key="t" :href="blogUrl + '?tag=' + encodeURIComponent(t)">{{ t }}</a>
          </div>
        </div>
      </aside>
    </div>

    <!-- Footer -->
    <footer class="pa-footer">
      <p>© 2026 {{ siteName || '爱库录' }} {{ $t('· 目录即站点，文件即文章') }}</p>
      <div class="pa-footer-links">
        <a href="/api/v1/blog/feed.xml">RSS</a>
        <a :href="blogUrl">{{ $t('博客') }}</a>
      </div>
    </footer>
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, ref, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import PostCover from '@/components/PostCover.vue'
import './style.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], loading: true })

const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => ctx.value.tags || [])
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || '/blog')

const featured = computed(() => posts.value[0] || null)
const restPosts = computed(() => posts.value.slice(featured.value ? 1 : 0))

const aiQuery = ref('')
const aiAnswer = ref('')

function postUrl(p) {
  if (ctx.value.postUrl) return ctx.value.postUrl(p)
  if (p.file?.slug) return '/' + p.file.slug
  if (p.path) return '/blog?path=' + encodeURIComponent(p.path)
  return '#'
}

function getTitle(p) {
  const name = p.file?.name || p.path || ''
  return name.replace(/\.md$/i, '').split('/').pop() || t('common.unnamed')
}

function getSummary(p) {
  const raw = p.preview || ''
  return raw.replace(/^#+\s.*$/gm, '').replace(/!?\[.*?\]\(.*?\)/g, '').replace(/[#*`>_~]/g, '').trim().slice(0, 120)
}

function getCategory(p) {
  const parts = (p.path || '').split('/')
  return parts.length > 1 ? parts[0] : ''
}

function formatDate(p) {
  const ts = p.file?.updated_at || p.created_at
  if (!ts) return ''
  const d = new Date(typeof ts === 'number' && ts < 1e12 ? ts * 1000 : ts)
  return d.toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' })
}

const emojiMap = { 'Docker': '🐳', 'Go': '🐹', 'AI': '🤖', 'RAG': '🧠', 'IM': '📱', '安全': '🔐', '双链': '🔗', 'DevOps': '⚙️' }
// 分类占位渐变：低饱和暖色系，贴合暖纸底色（避免暗色块突兀）
const gradientMap = {
  'Docker': 'linear-gradient(135deg,#e8dcc8,#dccbb0)',
  'AI': 'linear-gradient(135deg,#e5d8c4,#d4bfa0)',
  'IM': 'linear-gradient(135deg,#dde5d2,#c8d4b8)',
  'Go': 'linear-gradient(135deg,#d8e0e5,#c2ced8)',
  '安全': 'linear-gradient(135deg,#ecd6ce,#ddb8ac)',
}

function getEmoji(p) { return emojiMap[getCategory(p)] || '📝' }
function getGradient(p) { return { background: gradientMap[getCategory(p)] || 'linear-gradient(135deg,#ede8e0,#f5f0ea)' } }

async function askAI() {
  if (!aiQuery.value.trim()) return
  aiAnswer.value = t('思考中...')
  try {
    const res = await fetch('/api/v1/public/blog/ask', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question: aiQuery.value })
    })
    if (res.status === 429) { aiAnswer.value = t('提问太频繁，请 5 分钟后再试'); return }
    const data = await res.json()
    aiAnswer.value = data.reply || data.answer || t('暂无回答')
  } catch { aiAnswer.value = t('请求失败，请稍后重试') }
}
</script>
