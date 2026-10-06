<template>
  <div class="pa-post">
    <header class="pa-post-top">
      <a class="pa-post-brand" :href="listHref">{{ siteName }}</a>
      <nav class="pa-post-nav">
        <ThemeSwitch />
        <a :href="listHref">{{ $t('返回列表') }}</a>
      </nav>
    </header>

    <div v-if="loading" class="pa-state">{{ $t('加载中…') }}</div>
    <div v-else-if="error" class="pa-state pa-state-err">{{ error }}</div>

    <div v-else class="pa-pwrap">
      <div class="pa-page-head">
        <h1 class="pa-page-title">{{ $t('归档') }}</h1>
        <div class="pa-page-sub">{{ $t('按更新时间分组 · 共') }} {{ posts.length }} {{ $t('篇 /') }} {{ groups.length }} {{ $t('个月') }}</div>
      </div>

      <div class="pa-layout">
        <main class="pa-post-main">
          <nav v-if="years.length > 1" class="pa-arc-years">
            <button v-for="y in years" :key="y" type="button" class="pa-arc-year-btn" @click="jumpTo(y)">
              {{ y }} {{ $t('年') }}
            </button>
          </nav>

          <section v-for="g in groups" :id="'y-' + g.year" :key="g.ym" class="pa-arc-year">
            <div class="pa-arc-year-h">
              <span>{{ g.year }} {{ $t('年') }}</span>
              <span class="pa-arc-year-n">{{ g.items.length }} {{ $t('篇') }}</span>
            </div>
            <div v-for="m in g.months" :key="m.month" class="pa-arc-month">
              <div class="pa-arc-month-h">{{ m.month }} {{ $t('月 ·') }} {{ m.items.length }} {{ $t('篇') }}</div>
              <a
                v-for="p in m.items"
                :key="pKey(p)"
                class="pa-arc-row"
                :href="postHref(ctx.value, p)"
              >
                <span class="pa-arc-day">{{ postDate(p, 'short') }}</span>
                <span class="pa-arc-title">{{ postTitle(p) }}</span>
                <span v-if="postCat(p)" class="pa-arc-cat">{{ postCat(p) }}</span>
              </a>
            </div>
          </section>

          <p v-if="!groups.length" class="pa-state">{{ $t('还没有公开文章') }}</p>
        </main>

        <aside class="pa-aside">
          <SideWidget />
        </aside>
      </div>
    </div>

    <footer class="pa-post-foot-bar">
      <div class="pa-post-foot-in">
        <span>{{ siteName }}</span>
        <a :href="rssHref">RSS</a>
      </div>
    </footer>
  </div>
  <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
import SideWidget from '../SideWidget.vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import { computed } from 'vue'
import { useBlog, postTitle, postCat, postDate, postHref } from './helpers.js'
import './post.css'

const ctx = useBlog()
const posts = computed(() => ctx.value?.posts || [])
const loading = computed(() => !!ctx.value?.loading)
const error = computed(() => ctx.value?.error || '')
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '爱库录')
const listHref = '#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'

const groups = computed(() => {
  const map = new Map()
  for (const p of posts.value) {
    const ym = postDate(p, 'month')
    if (!ym) continue
    if (!map.has(ym)) map.set(ym, [])
    map.get(ym).push(p)
  }
  return [...map.entries()]
    .sort((a, b) => (a[0] < b[0] ? 1 : -1))
    .map(([ym, items]) => {
      const [year, month] = ym.split('-')
      return {
        ym,
        year,
        month: Number(month),
        items: items.sort((a, b) => (b.file?.updated_at || 0) - (a.file?.updated_at || 0)),
      }
    })
    .reduce((acc, g) => {
      let last = acc[acc.length - 1]
      if (!last || last.year !== g.year) {
        last = { year: g.year, months: [] }
        acc.push(last)
      }
      last.months.push({ month: g.month, items: g.items })
      return acc
    }, [])
})

const years = computed(() => groups.value.map((g) => g.year))

function jumpTo(y) {
  const el = document.getElementById('y-' + y)
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function pKey(p) {
  return p.path || p.file?.slug || p.token || Math.random()
}
</script>
