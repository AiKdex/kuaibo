<template>
  <div class="pp-root">
    <header class="pp-topbar">
      <div class="pp-container pp-topbar-inner">
        <a class="pp-brand" :href="publicHref">
          <span class="pp-logo">{{ $t('爱') }}</span>
          <span class="pp-brand-text">
            <strong>{{ ctx.siteName || '爱库录' }}</strong>
            <em>{{ $t('AI 知识库博客') }}</em>
          </span>
        </a>
        <nav class="pp-nav">
          <ThemeSwitch />
          <a :href="publicHref">{{ $t('情报板') }}</a>
          <a :href="rssHref" target="_blank" rel="noopener">RSS</a>
          <a href="/">{{ $t('官网') }}</a>
        </nav>
      </div>
    </header>

    <div class="pp-container pp-main">
      <section class="pp-hero">
        <p class="pp-kicker">{{ $t('城市级 · 知识库 · 公开站') }}</p>
        <h1>{{ $t('把散落在目录里的') }}<br />{{ $t('知识，拼成你能用的站') }}</h1>
        <p class="pp-lead">
          {{ ctx.siteDesc || $t("目录即站点，文件即文章。拖拽即发、语义检索、双链互文。") }}
        </p>
        <div class="pp-stats">
          <div class="pp-stat">
            <div class="pp-stat-n">{{ posts.length }}</div>
            <div class="pp-stat-l">{{ $t('已发布文章') }}</div>
          </div>
          <div class="pp-stat">
            <div class="pp-stat-n">{{ catCount }}</div>
            <div class="pp-stat-l">{{ $t('分类目录') }}</div>
          </div>
          <div class="pp-stat">
            <div class="pp-stat-n">SSR</div>
            <div class="pp-stat-l">{{ $t('可收录静态页') }}</div>
          </div>
          <div class="pp-stat">
            <div class="pp-stat-n">{{ $t('本地') }}</div>
            <div class="pp-stat-l">{{ $t('数据主权') }}</div>
          </div>
        </div>
      </section>

      <section class="pp-grid">
        <div class="pp-features">
          <article class="pp-feat">
            <h3>{{ $t('渠道差') }}</h3>
            <p>{{ $t("知识不在聊天记录里，在可发布的目录里。一份 Markdown，就是一个可检索节点。") }}</p>
          </article>
          <article class="pp-feat">
            <h3>{{ $t('拼图差') }}</h3>
            <p>{{ $t("全文 + 语义检索，把碎片知识拼成可导航的公开站，而不是一堆附件。") }}</p>
          </article>
          <article class="pp-feat">
            <h3>{{ $t('可行动') }}</h3>
            <p>{{ $t("拖进博客目录即发布；slug 不碎链；RSS / sitemap 一并就绪。") }}</p>
          </article>
          <article class="pp-feat">
            <h3>{{ $t('溯源可信') }}</h3>
            <p>{{ $t("每篇文章可点回原文路径；AI 不替代事实，发布权在站长手里。") }}</p>
          </article>
        </div>

        <aside class="pp-side">
          <div class="pp-panel">
            <div class="pp-panel-h">{{ $t('本月情报流') }}</div>
            <p class="pp-panel-tip">{{ $t('按分类浏览 · 点标题阅读') }}</p>
            <div v-if="ctx.loading" class="pp-empty">{{ $t('加载中…') }}</div>
            <div v-else-if="!posts.length" class="pp-empty">{{ $t('暂无公开文章') }}</div>
            <ul v-else class="pp-feed">
              <li v-for="(p, i) in posts" :key="i">
                <a :href="href(p)">{{ title(p) }}</a>
                <span class="pp-feed-meta">
                  <b v-if="cat(p)">{{ cat(p) }}</b>
                  <span>{{ date(p) }}</span>
                </span>
              </li>
            </ul>
          </div>
        </aside>
      </section>

      <section class="pp-list-wrap">
        <h2 class="pp-h2">{{ $t('全部文章') }}</h2>
        <a
          v-for="(p, i) in posts"
          :key="'c' + i"
          class="pp-card"
          :href="href(p)"
        >
          <div class="pp-card-badge" :class="badgeClass(p)">{{ cat(p) || $t("文") }}</div>
          <div class="pp-card-body">
            <h3>{{ title(p) }}</h3>
            <p v-if="p.preview" class="pp-card-ex">{{ clean(p.preview) }}</p>
            <div class="pp-card-m">
              <span>{{ date(p) }}</span>
              <span v-if="p.file?.size">· {{ size(p.file.size) }}</span>
            </div>
          </div>
        </a>
      </section>
    </div>

    <footer class="pp-foot">
      <div class="pp-container">
        <span>{{ ctx.siteName || '爱库录' }} {{ $t('· 目录即站点，文件即文章') }}</span>
        <span>
          <a :href="rssHref">RSS</a>
          ·
          <a href="/blog">{{ $t('静态页') }}</a>
        </span>
      </div>
    </footer>
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
const ctx = computed(() => raw?.value || raw || { posts: [], loading: true })
const posts = computed(() => ctx.value.posts || [])
const publicHref = '#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'

const catCount = computed(() => {
  const s = new Set()
  for (const p of posts.value) {
    const c = cat(p)
    if (c) s.add(c)
  }
  return s.size
})

function title(p) {
  const n = p.file?.name || p.name || p.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('common.unnamed')
}
function cat(p) {
  const path = p.path || ''
  return path.includes('/') ? path.split('/')[0] : ''
}
function date(p) {
  const v = p.file?.updated_at || p.created_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
function size(n) {
  if (n < 1024) return n + ' B'
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1048576).toFixed(1) + ' MB'
}
function clean(s) {
  return String(s || '')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/^（降级摘要）文件名：[^\n#]+/m, '')
    .replace(/^#{1,6}\s+[^\n]+$/gm, '')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 140)
}
function href(p) {
  try {
    return ctx.value.postUrl ? ctx.value.postUrl(p) : `#/p/${p.token}`
  } catch {
    return '#'
  }
}
function badgeClass(p) {
  const c = cat(p)
  if (c.includes('产品')) return 'is-product'
  if (c.includes('理念')) return 'is-idea'
  if (c.includes('教程')) return 'is-howto'
  return ''
}
</script>
