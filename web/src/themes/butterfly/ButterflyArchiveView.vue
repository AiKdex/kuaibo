<template>
  <div class="bf-root" :class="{ 'is-dark': dark }">
    <ButterflyHead :site-name="siteName" :slogan="slogan" :blog-url="blogUrl" mode="archive" :dark="dark" @toggle-dark="toggleDark" />

    <div class="bf-layout">
      <main class="bf-main">
        <div class="bf-bar">
          <div class="bf-bar-l">
            <h1 class="bf-bar-t">{{ $t('文章归档') }}</h1>
            <span class="bf-bar-s">{{ $t("按更新时间倒序，自动分年归组") }}</span>
          </div>
          <div class="bf-bar-r">
            <span class="bf-bar-chip">{{ posts.length }} {{ $t('篇') }}</span>
            <span v-if="years.length" class="bf-bar-chip">{{ years.length }} {{ $t('个年份') }}</span>
          </div>
        </div>

        <div v-if="loading" class="bf-state"><span class="bf-state-ic">…</span><p class="bf-state-t">{{ $t('正在加载') }}</p></div>
        <div v-else-if="error" class="bf-state"><span class="bf-state-ic">!</span><p class="bf-state-t">{{ error }}</p></div>
        <div v-else-if="!years.length" class="bf-state">
          <span class="bf-state-ic">∅</span>
          <p class="bf-state-t">{{ $t('暂无归档内容') }}</p>
          <a class="bf-state-btn" :href="blogUrl">{{ $t('查看全部文章') }}</a>
        </div>
        <div v-else class="bf-archive">
          <section v-for="g in years" :key="g.year" class="bf-arc-y">
            <h2 class="bf-arc-yh">
              <span class="bf-arc-ydot"></span>
              <span>{{ g.year }}</span>
              <span class="bf-arc-ynum">{{ g.list.length }} {{ $t('篇') }}</span>
            </h2>
            <ul class="bf-arc-list">
              <li v-for="p in g.list" :key="p.token || p.path" class="bf-arc-i">
                <span class="bf-arc-date">{{ postDate(p, 'short') }}</span>
                <span class="bf-arc-dot"></span>
                <a class="bf-arc-t" :href="postHref(ctx, p)">{{ postTitle(p) }}</a>
                <span class="bf-arc-cat">{{ catName(p) }}</span>
              </li>
            </ul>
          </section>
        </div>
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
import { computed } from 'vue'
import ButterflyHead from './ButterflyHead.vue'
import ButterflySide from './ButterflySide.vue'
import ButterflyFoot from './ButterflyFoot.vue'
import ButterflyRight from './ButterflyRight.vue'
import { LIST_HREF, catName, postDate, postHref, postTitle, useBlog, useDark, yearGroups } from './helpers.js'

const ctx = useBlog()
const { dark, toggleDark } = useDark()

const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => (ctx.value.tags || []).filter((t) => t && t.name))
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)
const slogan = 'BUTTERFLY · 归档'
const years = computed(() => yearGroups(posts.value))
</script>
