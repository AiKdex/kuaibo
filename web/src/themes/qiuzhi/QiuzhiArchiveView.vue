<template>
  <div class="qz-archive">
    <header class="qz-topbar qz-p-topbar">
      <div class="qz-container qz-p-bar-in">
        <a class="qz-p-brand" :href="listHref">{{ siteName }}</a>
        <nav class="qz-p-nav">
          <ThemeSwitch />
          <a :href="listHref">{{ $t('返回列表') }}</a>
        </nav>
      </div>
    </header>

    <div v-if="loading" class="qz-state">{{ $t('加载中…') }}</div>
    <div v-else-if="error" class="qz-state qz-state-err">{{ error }}</div>

    <div v-else class="qz-container qz-p-wrap">
      <div class="qz-p-page-head">
        <h1 class="qz-p-page-title">{{ $t('归档') }}</h1>
        <div class="qz-p-page-sub">{{ $t('按更新时间分组 · 共') }} {{ posts.length }} {{ $t('篇 /') }} {{ groups.length }} {{ $t('个月') }}</div>
      </div>

      <nav v-if="years.length > 1" class="qz-p-chips">
        <button
          v-for="y in years"
          :key="y"
          type="button"
          class="qz-p-chip"
          @click="jumpTo(y)"
        >{{ y }} {{ $t('年') }}</button>
      </nav>

      <div class="qz-layout">
        <main class="qz-p-main">
          <section v-for="y in years" :id="'qz-y-' + y" :key="y" class="qz-p-year">
            <h2 class="qz-p-year-h">{{ y }} {{ $t('年') }} <span class="qz-p-year-n">{{ yearCount(y) }} {{ $t('篇') }}</span></h2>
            <div v-for="g in groupsByYear(y)" :key="g.month" class="qz-p-month">
              <h3 class="qz-p-month-h">{{ g.month }} {{ $t('月') }} <span>{{ g.items.length }} {{ $t('篇') }}</span></h3>
              <a
                v-for="p in g.items"
                :key="pKey(p)"
                class="qz-p-arc-row"
                :href="postHref(ctx.value, p)"
              >
                <span class="qz-p-arc-day">{{ postDate(p).slice(-2) }}</span>
                <span class="qz-p-arc-title">{{ postTitle(p) }}</span>
                <span v-if="postCat(p)" class="qz-p-arc-cat">{{ postCat(p) }}</span>
              </a>
            </div>
          </section>
          <p v-if="!groups.length" class="qz-state">{{ $t('还没有公开文章') }}</p>
        </main>
        <aside class="qz-aside">
          <SideWidget />
        </aside>
      </div>
    </div>
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
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '求智')
const listHref = '#/blog?view=public'

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
      const [y, m] = ym.split('-')
      return {
        ym,
        year: y,
        month: Number(m),
        items: items.sort((a, b) => (b.file?.updated_at || 0) - (a.file?.updated_at || 0)),
      }
    })
})

const years = computed(() =>
  [...new Set(groups.value.map((g) => g.year))].sort((a, b) => Number(b) - Number(a)),
)
const groupsByYear = (y) => groups.value.filter((g) => g.year === y)
const yearCount = (y) => groupsByYear(y).reduce((n, g) => n + g.items.length, 0)

function pKey(p) {
  return p.path || p.file?.slug || p.token || Math.random()
}

/* 年份跳转：滚动到对应 section，不改动 location.hash */
function jumpTo(y) {
  const el = document.getElementById('qz-y-' + y)
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
</script>
