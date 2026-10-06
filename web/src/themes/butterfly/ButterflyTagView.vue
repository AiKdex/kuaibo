<template>
  <div class="bf-root" :class="{ 'is-dark': dark }">
    <ButterflyHead :site-name="siteName" :slogan="slogan" :blog-url="blogUrl" mode="list" :cat-active="!!tag" :dark="dark" @toggle-dark="toggleDark" />

    <div class="bf-layout">
      <main class="bf-main">
        <div class="bf-bar">
          <div class="bf-bar-l">
            <h1 class="bf-bar-t">{{ tag || $t("全部") }}</h1>
            <span class="bf-bar-s">{{ $t('标签来自文章元数据') }}</span>
          </div>
          <div class="bf-bar-r"><span class="bf-bar-chip">{{ list.length }} {{ $t('篇') }}</span></div>
        </div>

        <div class="bf-tool">
          <div class="bf-chips">
            <a class="bf-chip" :class="{ 'is-on': !picked }" :href="blogUrl">{{ $t('全部') }}</a>
            <a
              v-for="t in tagList"
              :key="t.name"
              class="bf-chip"
              :class="{ 'is-on': (picked || tag) === t.name }"
              :href="'#/blog/tag/' + encodeURIComponent(t.name)"
              @click="picked = t.name"
            >{{ t.name }}<i>{{ t.count }}</i></a>
          </div>
          <div class="bf-sorts">
            <a class="bf-sort" :href="blogUrl">{{ $t('‹ 返回全部') }}</a>
          </div>
        </div>

        <div v-if="loading" class="bf-state"><span class="bf-state-ic">…</span><p class="bf-state-t">{{ $t('正在加载') }}</p></div>
        <div v-else-if="error" class="bf-state"><span class="bf-state-ic">!</span><p class="bf-state-t">{{ error }}</p></div>
        <div v-else-if="!list.length" class="bf-state">
          <span class="bf-state-ic">∅</span>
          <p class="bf-state-t">{{ $t('该标签下暂无文章') }}</p>
          <a class="bf-state-btn" :href="blogUrl">{{ $t('查看全部文章') }}</a>
        </div>
        <template v-else>
          <ButterflyCard v-for="(p, i) in list" :key="p.token || p.path || i" :post="p" :index="i" :reversed="i % 2 === 1" />
        </template>
      </main>

      <ButterflySide
        :posts="posts" :tags="tags" :site-name="siteName" :site-desc="siteDesc" :active-cat="picked || tag"
        @pick-cat="picked = $event"
      />
    </div>

    <ButterflyFoot :site-name="siteName" :blog-url="blogUrl" :count="posts.length" />
    <ButterflyRight :dark="dark" @toggle-dark="toggleDark" />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
import { computed, ref } from 'vue'
import ButterflyHead from './ButterflyHead.vue'
import ButterflyCard from './ButterflyCard.vue'
import ButterflySide from './ButterflySide.vue'
import ButterflyFoot from './ButterflyFoot.vue'
import ButterflyRight from './ButterflyRight.vue'
import { LIST_HREF, readPageParam, useBlog, useDark } from './helpers.js'

const ctx = useBlog()
const { dark, toggleDark } = useDark()

const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => (ctx.value.tags || []).filter((t) => t && t.name))
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)
const slogan = 'BUTTERFLY · 标签浏览'

const tagList = computed(() => {
  const map = new Map()
  for (const p of posts.value) {
    for (const t of (p.file?.tags || [])) {
      const name = typeof t === 'string' ? t : (t && t.name)
      if (!name) continue
      map.set(name, (map.get(name) || 0) + 1)
    }
  }
  return [...map.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count || String(a.name).localeCompare(String(b.name), 'zh'))
})

/* 当前标签：优先 ctx.page.tag（系统注入），回退解析 hash / ?tag= */
const pageParam = computed(() =>
  readPageParam(ctx.value, ['tag'], [/\/tag\/([^/?#]+)/, /[?&]tag=([^&#]+)/]))
const picked = ref('')
const tag = computed(() => picked.value || pageParam.value)
const list = computed(() => {
  if (!tag.value) return posts.value
  return posts.value.filter((p) =>
    (p.file?.tags || []).some((t) => (typeof t === 'string' ? t : (t && t.name)) === tag.value)
  )
})
</script>
