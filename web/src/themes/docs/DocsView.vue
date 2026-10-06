<template>
  <div class="dx-root">
    <header class="dx-top">
      <a class="dx-brand" :href="publicHref">{{ ctx.siteName || '爱库录' }}</a>
      <span class="dx-sub">{{ $t('文档') }}</span>
      <nav class="dx-nav">
        <ThemeSwitch />
        <a :href="rssHref" target="_blank" rel="noopener">RSS</a>
        <a href="https://github.com/AiKdex/AiKlog" target="_blank" rel="noopener">GitHub</a>
      </nav>
    </header>

    <div class="dx-layout">
      <aside class="dx-side">
        <div class="dx-side-h">{{ $t('目录') }}</div>
        <button
          type="button"
          class="dx-toc-item"
          :class="{ on: !activePath }"
          @click="activePath = ''"
        >{{ $t('全部文章') }}</button>
        <div v-for="c in cats" :key="c.name" class="dx-toc-group">
          <div class="dx-toc-cat">{{ c.name }}</div>
          <button
            v-for="p in c.items"
            :key="p.path"
            type="button"
            class="dx-toc-item"
            :class="{ on: activePath === p.path }"
            @click="activePath = p.path"
          >{{ title(p) }}</button>
        </div>
      </aside>

      <main class="dx-main">
        <template v-if="!activePath">
          <h1 class="dx-h1">{{ ctx.siteName || '爱库录' }}</h1>
          <p class="dx-lead">{{ ctx.siteDesc || $t("目录即站点，文件即文章") }}</p>
          <div v-if="ctx.loading" class="dx-state">{{ $t('加载中…') }}</div>
          <div v-else-if="!posts.length" class="dx-state">{{ $t('暂无文章') }}</div>
          <ul v-else class="dx-list">
            <li v-for="(p, i) in posts" :key="i">
              <a :href="href(p)">{{ title(p) }}</a>
              <span class="dx-meta">{{ date(p) }}</span>
            </li>
          </ul>
        </template>
        <template v-else>
          <button type="button" class="dx-back" @click="activePath = ''">{{ $t('← 返回目录') }}</button>
          <article class="dx-doc">
            <h1 class="dx-h1">{{ title(current) }}</h1>
            <div class="dx-meta">{{ date(current) }}{{ cat(current) ? ' · ' + cat(current) : '' }}</div>
            <a class="dx-open" :href="href(current)">{{ $t('阅读全文 →') }}</a>
          </article>
        </template>
      </main>
    </div>
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, inject, ref } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'

const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [], loading: true })
const posts = computed(() => ctx.value.posts || [])
const publicHref = '#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'
const activePath = ref('')

const cats = computed(() => {
  const m = new Map()
  for (const p of posts.value) {
    const c = cat(p) || '未分类'
    if (!m.has(c)) m.set(c, [])
    m.get(c).push(p)
  }
  return [...m.entries()].map(([name, items]) => ({ name, items }))
})

const current = computed(() => posts.value.find((p) => p.path === activePath.value) || posts.value[0])

function title(p) {
  const n = p?.file?.name || p?.name || p?.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('common.unnamed')
}
function cat(p) {
  const path = p?.path || ''
  return path.includes('/') ? path.split('/')[0] : ''
}
function date(p) {
  const v = p?.file?.updated_at || p?.created_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
function href(p) {
  try {
    return ctx.value.postUrl ? ctx.value.postUrl(p) : `#/p/${p.token}`
  } catch {
    return '#'
  }
}
</script>
