<template>
  <div class="bf-root" :class="{ 'is-dark': dark }">
    <ButterflyHead :site-name="siteName" :slogan="slogan" :blog-url="blogUrl" mode="list" :cat-active="!!cat" :dark="dark" @toggle-dark="toggleDark" />

    <div class="bf-layout">
      <main class="bf-main">
        <div class="bf-bar">
          <div class="bf-bar-l">
            <h1 class="bf-bar-t">{{ cat || $t("未分类") }}</h1>
            <span class="bf-bar-s">{{ $t('分类由文件路径首段决定') }}</span>
          </div>
          <div class="bf-bar-r"><span class="bf-bar-chip">{{ list.length }} {{ $t('篇') }}</span></div>
        </div>

        <div class="bf-tool">
          <div class="bf-chips">
            <button class="bf-chip" :class="{ 'is-on': !picked }" type="button" @click="picked = ''">{{ $t('全部') }}</button>
            <button
              v-for="c in cats"
              :key="c.name"
              class="bf-chip"
              :class="{ 'is-on': (picked || cat) === c.name }"
              type="button"
              @click="picked = c.name"
            >{{ c.name }}<i>{{ c.count }}</i></button>
          </div>
          <div class="bf-sorts">
            <a class="bf-sort" :href="blogUrl">{{ $t('‹ 返回全部') }}</a>
          </div>
        </div>

        <div v-if="loading" class="bf-state"><span class="bf-state-ic">…</span><p class="bf-state-t">{{ $t('正在加载') }}</p></div>
        <div v-else-if="error" class="bf-state"><span class="bf-state-ic">!</span><p class="bf-state-t">{{ error }}</p></div>
        <div v-else-if="!list.length" class="bf-state">
          <span class="bf-state-ic">∅</span>
          <p class="bf-state-t">{{ $t('该分类下暂无文章') }}</p>
          <a class="bf-state-btn" :href="blogUrl">{{ $t('查看全部文章') }}</a>
        </div>
        <template v-else>
          <ButterflyCard v-for="(p, i) in list" :key="p.token || p.path || i" :post="p" :index="i" :reversed="i % 2 === 1" />
        </template>
      </main>

      <ButterflySide
        :posts="posts" :tags="tags" :site-name="siteName" :site-desc="siteDesc" :active-cat="picked || cat"
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
import { LIST_HREF, catCounts, catName, readPageParam, useBlog, useDark } from './helpers.js'

const ctx = useBlog()
const { dark, toggleDark } = useDark()

const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => (ctx.value.tags || []).filter((t) => t && t.name))
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)
const slogan = 'BUTTERFLY · 分类浏览'
const cats = computed(() => catCounts(posts.value))

/* 当前分类：优先 ctx.page.cat（系统注入），回退解析 hash / ?cat= */
const pageParam = computed(() =>
  readPageParam(ctx.value, ['cat', 'category'], [/\/cat\/([^/?#]+)/, /[?&]cat=([^&#]+)/]))
const picked = ref('')
const cat = computed(() => picked.value || pageParam.value)
const list = computed(() => posts.value.filter((p) => catName(p) === (picked.value || cat.value || '未分类')))
</script>
