<template>
  <div class="qz-cat">
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
        <h1 class="qz-p-page-title">{{ $t('分类：') }}{{ cat || $t("未分类") }}</h1>
        <div class="qz-p-page-sub">{{ $t('共') }} {{ list.length }} {{ $t('篇') }}</div>
      </div>

      <nav v-if="cats.length > 1" class="qz-p-chips">
        <a
          v-for="c in cats"
          :key="c.name"
          class="qz-p-chip"
          :class="{ on: c.name === (cat || $t('未分类')) }"
          :href="'#/blog/cat/' + encodeURIComponent(c.name)"
        >{{ c.name }}</a>
      </nav>

      <div class="qz-layout">
        <main class="qz-p-main">
          <ul v-if="list.length" class="qz-p-list">
            <li v-for="p in list" :key="pKey(p)">
              <a class="qz-p-link" :href="postHref(ctx.value, p)">
                <h3 class="qz-p-list-title">{{ postTitle(p) }}</h3>
                <div class="qz-p-meta">
                  <span v-if="postCat(p)">{{ postCat(p) }}</span>
                  <span v-if="postDate(p)">{{ postDate(p) }}</span>
                </div>
              </a>
            </li>
          </ul>
          <p v-else class="qz-state">{{ $t('该分类下暂无文章') }}</p>
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
import { useBlog, postTitle, postCat, postDate, postHref, readPageParam } from './helpers.js'
import './post.css'

const ctx = useBlog()
const posts = computed(() => ctx.value?.posts || [])
const loading = computed(() => !!ctx.value?.loading)
const error = computed(() => ctx.value?.error || '')
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '求智')
const listHref = '#/blog?view=public'

const cat = computed(
  () =>
    ctx.value?.page?.cat ||
    readPageParam(ctx.value, ['cat', 'category'], [/\/cat\/([^/?#]+)/, /[?&]cat=([^&#]+)/]) ||
    '未分类',
)
const cats = computed(() => {
  const map = new Map()
  for (const p of posts.value) {
    const name = postCat(p) || '未分类'
    map.set(name, (map.get(name) || 0) + 1)
  }
  return [...map.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, count]) => ({ name, count }))
})
const list = computed(() => posts.value.filter((p) => (postCat(p) || '未分类') === cat.value))

function pKey(p) {
  return p.path || p.file?.slug || p.token || Math.random()
}
</script>
