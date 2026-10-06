<template>
  <div class="elevated-root">
    <div class="bg-ambient"></div>
    <div class="bg-noise"></div>

    <!-- Top Bar -->
    <header class="el-topbar">
      <div class="el-topbar-in">
        <div class="el-logo"><b>●</b> AiKlog <span class="el-logo-sub">{{ $t('爱库录') }}</span></div>
        <nav class="el-nav">
          <ThemeSwitch />
          <a :href="blogUrl">{{ $t('博客') }}</a>
          <a href="/blog?tag=">{{ $t('标签') }}</a>
          <a href="/api/v1/blog/feed.xml">RSS</a>
        </nav>
      </div>
    </header>

    <!-- Hero -->
    <section class="el-hero">
      <h1>{{ $t('目录即站点') }}<br>{{ $t('文件即') }}<em>{{ $t('文章') }}</em></h1>
      <p>{{ siteDesc || $t("拖个文件进目录就是一篇博客。发条 IM 消息，AI 自动整理发布。") }}</p>
      <div class="el-hero-stats">
        <span>📝 {{ posts.length }} {{ $t('篇') }}</span>
        <span>🏷️ {{ (tags || []).length }} {{ $t('标签') }}</span>
      </div>
    </section>

    <!-- Content Grid -->
    <div class="el-wrap">
      <main class="el-main">
        <!-- Featured Post -->
        <a v-if="featured" :href="postUrl(featured)" class="el-card el-feat">
          <div class="el-feat-img">
            <div class="el-feat-ph" :style="getGradient(featured)">{{ getEmoji(featured) }}</div>
          </div>
          <div class="el-feat-body">
            <div class="el-tags">
              <span v-if="getCategory(featured)" class="tag">{{ getCategory(featured) }}</span>
            </div>
            <h2>{{ getTitle(featured) }}</h2>
            <p>{{ getSummary(featured) }}</p>
            <div class="el-card-ft">
              <span>📅 {{ formatDate(featured) }}</span>
            </div>
          </div>
        </a>

        <!-- Post Cards -->
        <a
          v-for="(post, i) in restPosts"
          :key="post.token || i"
          :href="postUrl(post)"
          class="el-card"
        >
          <div class="el-card-img">
            <div class="el-card-ph" :style="getGradient(post)">{{ getEmoji(post) }}</div>
          </div>
          <div class="el-card-body">
            <div class="el-tags">
              <span v-if="getCategory(post)" class="tag">{{ getCategory(post) }}</span>
            </div>
            <h2>{{ getTitle(post) }}</h2>
            <p>{{ getSummary(post) }}</p>
            <div class="el-card-ft">
              <span>📅 {{ formatDate(post) }}</span>
            </div>
          </div>
        </a>
      </main>

      <!-- Sidebar -->
      <aside class="el-side">
        <!-- About -->
        <div class="sb sb-about">
          <div class="sb-avatar">{{ (siteName || 'A')[0] }}</div>
          <h3>{{ siteName || '爱库录' }}</h3>
          <p>{{ siteDesc || $t("目录即站点，文件即文章") }}</p>
          <div class="sb-stats">
            <div class="sb-stat"><strong>{{ posts.length }}</strong><span>{{ $t('文章') }}</span></div>
            <div class="sb-stat"><strong>{{ (tags || []).length }}</strong><span>{{ $t('标签') }}</span></div>
          </div>
        </div>

        <!-- AI Ask -->
        <div class="sb sb-ai">
          <div class="sb-title">{{ $t('🤖 AI 问答') }}</div>
          <div class="ai-box">
            <input
              v-model="aiQuery"
              class="ai-input"
              :placeholder="$t('问点什么...')"
              @keyup.enter="askAI"
            />
            <button class="ai-btn" @click="askAI">{{ $t('问') }}</button>
          </div>
          <div v-if="aiAnswer" class="ai-answer">{{ aiAnswer }}</div>
          <div class="ai-features">
            <span><span class="dot"></span>{{ $t('基于全站文章语义理解') }}</span>
            <span><span class="dot"></span>{{ $t('自动引用相关文章') }}</span>
            <span><span class="dot"></span>{{ $t('限流保护，每 5 分钟 8 次') }}</span>
          </div>
        </div>

        <!-- Tag Cloud -->
        <div v-if="tags && tags.length" class="sb">
          <div class="sb-title">{{ $t('🏷️ 标签云') }}</div>
          <!-- tags 契约为 {name,count} 对象数组；当前 BlogView 未填充 tags（恒空），
               区块由上方 v-if 隐藏。标签过滤（/blog?tag=）系统层尚未实现，先渲染为非链接，
               待系统层支持后再恢复 :href="blogUrl + '?tag=' + encodeURIComponent(t.name)" -->
          <div class="tag-cloud">
            <span v-for="t in tags" :key="t.name">{{ t.name }}</span>
          </div>
        </div>

        <!-- Links -->
        <div class="sb">
          <div class="sb-title">{{ $t('🔗 链接') }}</div>
          <div class="sb-links">
            <a href="/api/v1/blog/feed.xml">📡 RSS</a>
            <a :href="blogUrl">{{ $t('📖 博客') }}</a>
          </div>
        </div>
      </aside>
    </div>

    <!-- Footer -->
    <footer class="el-footer">
      <div class="el-footer-brand"><b>●</b> {{ $t('AiKlog · 爱库录') }}</div>
      <div class="el-footer-links">
        <a :href="blogUrl">{{ $t('博客') }}</a>
        <a href="/api/v1/blog/feed.xml">RSS</a>
      </div>
      <p>{{ $t('© 2026 AiKlog · 目录即站点，文件即文章 · Powered by Go + SQLite + AI') }}</p>
    </footer>
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, ref, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
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
  return name.replace(/\.(md|markdown)$/i, '').split('/').pop() || t('common.unnamed')
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
  return d.toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' })
}

const emojiMap = { 'Docker': '🐳', 'Go': '🐹', 'AI': '🤖', 'RAG': '🧠', 'IM': '📱', 'Telegram': '✈️', '安全': '🔐', '双链': '🔗', 'DevOps': '⚙️', 'SQLite': '🗄️', '拖拽': '📂', '自部署': '🏠' }
const gradientMap = {
  'Docker': 'linear-gradient(135deg,#1a3a4a,#0d2233)',
  'AI': 'linear-gradient(135deg,#2a1a3a,#1a0d33)',
  'IM': 'linear-gradient(135deg,#1a2a1a,#0d330d)',
  'Go': 'linear-gradient(135deg,#1a2a3a,#0d1a33)',
  '安全': 'linear-gradient(135deg,#2a1a1a,#330d0d)',
}

function getEmoji(p) {
  const cat = getCategory(p)
  return emojiMap[cat] || '📝'
}

function getGradient(p) {
  const cat = getCategory(p)
  const g = gradientMap[cat] || 'linear-gradient(135deg,#243049,#1a2236)'
  return { background: g }
}

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
