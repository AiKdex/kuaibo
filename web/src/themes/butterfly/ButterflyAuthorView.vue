<template>
  <div class="bf-root" :class="{ 'is-dark': dark }">
    <ButterflyHead :site-name="siteName" :slogan="slogan" :blog-url="blogUrl" mode="list" :dark="dark" @toggle-dark="toggleDark" />

    <div class="bf-layout">
      <main class="bf-main">
        <div class="bf-bar">
          <div class="bf-bar-l">
            <h1 class="bf-bar-t">{{ author }}</h1>
            <span class="bf-bar-s">{{ hasAuthorField ? $t("按作者字段归集") : $t("本站为单一作者站点，展示全部公开内容") }}</span>
          </div>
          <div class="bf-bar-r"><span class="bf-bar-chip">{{ list.length }} {{ $t('篇') }}</span></div>
        </div>

        <!-- 作者卡（无头像字段 → 首字徽标，纯装饰） -->
        <section class="bf-authorcard">
          <span class="bf-authorcard-ava" aria-hidden="true">{{ author.slice(0, 1) }}</span>
          <span class="bf-authorcard-body">
            <span class="bf-authorcard-n">{{ author }}</span>
            <span class="bf-authorcard-d">{{ siteDesc || $t("目录即站点，文件即文章") }}</span>
            <span class="bf-authorcard-s">
              <span>{{ list.length }} {{ $t('篇公开内容') }}</span>
              <span v-if="cats.length">· {{ cats.length }} {{ $t('个分类') }}</span>
            </span>
          </span>
          <a class="bf-authorcard-btn" :href="blogUrl">{{ $t('返回全部') }}</a>
        </section>

        <div v-if="loading" class="bf-state"><span class="bf-state-ic">…</span><p class="bf-state-t">{{ $t('正在加载') }}</p></div>
        <div v-else-if="error" class="bf-state"><span class="bf-state-ic">!</span><p class="bf-state-t">{{ error }}</p></div>
        <div v-else-if="!list.length" class="bf-state">
          <span class="bf-state-ic">∅</span>
          <p class="bf-state-t">{{ $t('该名下暂无文章') }}</p>
          <a class="bf-state-btn" :href="blogUrl">{{ $t('查看全部文章') }}</a>
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
import { LIST_HREF, catCounts, readPageParam, useBlog, useDark } from './helpers.js'

const ctx = useBlog()
const { dark, toggleDark } = useDark()

const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => (ctx.value.tags || []).filter((t) => t && t.name))
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '爱库录')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)
const slogan = 'BUTTERFLY · 作者主页'
const cats = computed(() => catCounts(posts.value))

/* 作者：优先 ctx.page.author，回退 hash / ?author=，再回退站点名（单作者站点） */
const pageParam = computed(() =>
  readPageParam(ctx.value, ['author'], [/\/author\/([^/?#]+)/, /[?&]author=([^&#]+)/]))
const picked = ref('')
const author = computed(() => picked.value || pageParam.value || siteName.value)

/** 列表契约未强制含 file.author：有该字段才做归集，否则视为单一作者站点 */
const hasAuthorField = computed(() => posts.value.some((p) => p.file?.author))
const list = computed(() => {
  if (!hasAuthorField.value) return posts.value
  return posts.value.filter((p) => (p.file?.author || siteName.value) === author.value)
})
</script>
