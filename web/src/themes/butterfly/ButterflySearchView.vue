<template>
  <div class="bf-root" :class="{ 'is-dark': dark }">
    <ButterflyHead :site-name="siteName" :slogan="slogan" :blog-url="blogUrl" mode="search" :dark="dark" @toggle-dark="toggleDark" />

    <div class="bf-layout">
      <main class="bf-main">
        <div class="bf-bar">
          <div class="bf-bar-l">
            <h1 class="bf-bar-t">{{ $t('全文检索') }}</h1>
            <span class="bf-bar-s">{{ $t("在标题、分类与摘要中即时匹配") }}</span>
          </div>
          <div class="bf-bar-r"><span class="bf-bar-chip">{{ posts.length }} {{ $t('篇可搜索') }}</span></div>
        </div>

        <div class="bf-search">
          <div class="bf-search-box">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round"><circle cx="11" cy="11" r="7.5" /><path d="M21 21l-4.4-4.4" /></svg>
            <input v-model="q" class="bf-search-in" type="search" :placeholder="$t('搜索标题、分类或摘要…')" />
            <button v-if="q" class="bf-search-clr" type="button" @click="q = ''">{{ $t('清空') }}</button>
          </div>
          <p class="bf-search-tip">
            <template v-if="!q">{{ $t('输入关键词开始检索，共') }} {{ posts.length }} {{ $t('篇可搜索内容。') }}</template>
            <template v-else>「{{ q }}{{ $t('」匹配到') }} {{ list.length }} {{ $t('篇') }}</template>
          </p>
        </div>

        <div v-if="loading" class="bf-state"><span class="bf-state-ic">…</span><p class="bf-state-t">{{ $t('正在加载') }}</p></div>
        <div v-else-if="error" class="bf-state"><span class="bf-state-ic">!</span><p class="bf-state-t">{{ error }}</p></div>
        <div v-else-if="!q" class="bf-state">
          <span class="bf-state-ic">⌕</span>
          <p class="bf-state-t">{{ $t('等待输入关键词') }}</p>
          <p class="bf-state-s">{{ $t("也可以到归档页按年月浏览全部内容。") }}</p>
        </div>
        <div v-else-if="!list.length" class="bf-state">
          <span class="bf-state-ic">∅</span>
          <p class="bf-state-t">{{ $t('没有匹配的内容') }}</p>
          <p class="bf-state-s">{{ $t("换个关键词再试，或到归档页浏览。") }}</p>
        </div>
        <template v-else>
          <ButterflyCard v-for="(p, i) in list" :key="p.token || p.path || i" :post="p" :index="i" :reversed="i % 2 === 1" />
        </template>
      </main>

      <ButterflySide :posts="posts" :tags="tags" :site-name="siteName" :site-desc="siteDesc" />
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
import { LIST_HREF, catName, postSummary, postTitle, readPageParam, useBlog, useDark } from './helpers.js'

const ctx = useBlog()
const { dark, toggleDark } = useDark()

const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => (ctx.value.tags || []).filter((t) => t && t.name))
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)
const slogan = 'BUTTERFLY · 检索'

/* 关键词：优先 ctx.page.q，回退 hash / ?q= */
const pageParam = computed(() => readPageParam(ctx.value, ['q'], [/[?&]q=([^&#]+)/]))
const q = ref(pageParam.value || '')

const list = computed(() => {
  const k = String(q.value || '').trim().toLowerCase()
  if (!k) return []
  return posts.value.filter((p) =>
    `${postTitle(p)} ${catName(p)} ${postSummary(p, 200)}`.toLowerCase().includes(k))
})
</script>
