<template>
  <div class="bf-root" :class="{ 'is-dark': dark }">
    <ButterflyHead
      :site-name="siteName"
      :slogan="slogan"
      :blog-url="blogUrl"
      :mode="mode"
      :cat-active="!!catFilter"
      :dark="dark"
      @go-list="goList"
      @go-archive="goArchive"
      @go-search="goSearch"
      @toggle-dark="toggleDark"
    />

    <div class="bf-layout">
      <main class="bf-main">
        <!-- 当前视图条（无首屏 Banner，直接从内容区开始） -->
        <div class="bf-bar">
          <div class="bf-bar-l">
            <h1 class="bf-bar-t">{{ barTitle }}</h1>
            <span class="bf-bar-s">{{ barSub }}</span>
          </div>
          <div class="bf-bar-r">
            <span class="bf-bar-chip">{{ posts.length }} {{ $t('篇文章') }}</span>
            <span v-if="cats.length" class="bf-bar-chip">{{ cats.length }} {{ $t('个分类') }}</span>
          </div>
        </div>

        <!-- ================= 列表 ================= -->
        <template v-if="mode === 'list'">
          <div class="bf-tool">
            <div class="bf-chips">
              <button class="bf-chip" :class="{ 'is-on': !catFilter }" type="button" @click="catFilter = ''">{{ $t('全部') }}</button>
              <button
                v-for="c in cats"
                :key="c.name"
                class="bf-chip"
                :class="{ 'is-on': catFilter === c.name }"
                type="button"
                @click="catFilter = catFilter === c.name ? '' : c.name"
              >{{ c.name }}<i>{{ c.count }}</i></button>
            </div>
            <div class="bf-sorts">
              <button class="bf-sort" :class="{ 'is-on': sort === 'time' }" type="button" @click="sort = 'time'">{{ $t('最新更新') }}</button>
              <button class="bf-sort" :class="{ 'is-on': sort === 'title' }" type="button" @click="sort = 'title'">{{ $t('标题') }}</button>
            </div>
          </div>

          <div v-if="loading" class="bf-skel">
            <div v-for="i in 3" :key="i" class="bf-skel-i">
              <span class="bf-skel-art"></span>
              <span class="bf-skel-lines">
                <i class="w40"></i><i class="w80"></i><i class="w60"></i><i class="w30"></i>
              </span>
            </div>
          </div>

          <div v-else-if="error" class="bf-state">
            <span class="bf-state-ic">!</span>
            <p class="bf-state-t">{{ $t('内容加载失败') }}</p>
            <p class="bf-state-s">{{ error }}</p>
            <button class="bf-state-btn" type="button" @click="reload">{{ $t('重新加载') }}</button>
          </div>

          <div v-else-if="!shown.length" class="bf-state">
            <span class="bf-state-ic">∅</span>
            <p class="bf-state-t">{{ catFilter ? $t('「{cat}」下暂无文章', { cat: catFilter }) : $t('还没有公开文章') }}</p>
            <p class="bf-state-s">{{ $t('在控制台把文件加入分享后，这里会出现卡片。') }}</p>
            <button v-if="catFilter" class="bf-state-btn" type="button" @click="catFilter = ''">{{ $t('查看全部文章') }}</button>
          </div>

          <template v-else>
            <ButterflyCard
              v-for="(p, i) in paged"
              :key="p.token || p.path || i"
              :post="p"
              :index="i"
              :reversed="i % 2 === 1"
            />

            <nav v-if="pages > 1" class="bf-pager">
              <button class="bf-page" type="button" :disabled="page <= 1" @click="goPage(page - 1)">{{ $t('上一页') }}</button>
              <button
                v-for="n in pages"
                :key="n"
                class="bf-page"
                :class="{ 'is-on': n === page }"
                type="button"
                @click="goPage(n)"
              >{{ n }}</button>
              <button class="bf-page" type="button" :disabled="page >= pages" @click="goPage(page + 1)">{{ $t('下一页') }}</button>
            </nav>
          </template>
        </template>

        <!-- ================= 归档 ================= -->
        <template v-else-if="mode === 'archive'">
          <div v-if="loading" class="bf-skel">
            <div v-for="i in 2" :key="i" class="bf-skel-i"><span class="bf-skel-art"></span><span class="bf-skel-lines"><i class="w40"></i><i class="w80"></i><i class="w60"></i></span></div>
          </div>
          <div v-else-if="!years.length" class="bf-state">
            <span class="bf-state-ic">∅</span>
            <p class="bf-state-t">{{ $t('暂无归档内容') }}</p>
            <p class="bf-state-s">{{ $t("公开文章出现后，这里会按年月自动分组。") }}</p>
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
        </template>

        <!-- ================= 检索 ================= -->
        <template v-else>
          <div class="bf-search">
            <div class="bf-search-box">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round"><circle cx="11" cy="11" r="7.5" /><path d="M21 21l-4.4-4.4" /></svg>
              <input v-model="q" class="bf-search-in" type="search" :placeholder="$t('搜索标题、分类或摘要…')" />
              <button v-if="q" class="bf-search-clr" type="button" @click="q = ''">{{ $t('清空') }}</button>
            </div>
            <p class="bf-search-tip">
              <template v-if="!q">{{ $t('输入关键词开始检索，共') }} {{ posts.length }} {{ $t('篇可搜索内容。') }}</template>
              <template v-else>「{{ q }}{{ $t('」匹配到') }} {{ searched.length }} {{ $t('篇') }}</template>
            </p>
          </div>

          <div v-if="q && !searched.length" class="bf-state">
            <span class="bf-state-ic">∅</span>
            <p class="bf-state-t">{{ $t('没有匹配的内容') }}</p>
            <p class="bf-state-s">{{ $t("换个关键词，或到归档页按年月浏览。") }}</p>
            <button class="bf-state-btn" type="button" @click="goArchive">{{ $t('打开归档') }}</button>
          </div>

          <template v-else-if="q">
            <ButterflyCard
              v-for="(p, i) in searched"
              :key="p.token || p.path || i"
              :post="p"
              :index="i"
              :reversed="i % 2 === 1"
            />
          </template>
        </template>
      </main>

      <ButterflySide
        :posts="posts"
        :tags="tags"
        :site-name="siteName"
        :site-desc="siteDesc"
        :active-cat="catFilter"
        @pick-cat="onPickCat"
        @go-archive="goArchive"
        @go-search="goSearch"
      />
    </div>

    <ButterflyFoot :site-name="siteName" :blog-url="blogUrl" :count="posts.length" @go-archive="goArchive" @go-search="goSearch" />
    <ButterflyRight :dark="dark" @toggle-dark="toggleDark" />
  </div>
    <AskWidget />
</template>

<script setup>
import { t as i18t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, ref, watch } from 'vue'
import ButterflyHead from './ButterflyHead.vue'
import ButterflyCard from './ButterflyCard.vue'
import ButterflySide from './ButterflySide.vue'
import ButterflyFoot from './ButterflyFoot.vue'
import ButterflyRight from './ButterflyRight.vue'
import {
  catCounts, catName, postDate, postHref, postSummary, postTitle,
  readPageParam, useBlog, useDark, yearGroups,
} from './helpers.js'

const ctx = useBlog()
const { dark, toggleDark } = useDark()

const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => (ctx.value.tags || []).filter((t) => t && t.name))
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')

const slogan = 'BUTTERFLY · ' + i18t('简单卡片，优雅阅读')
const blogUrl = '#/blog?view=public'

/* ===== 视图状态（无 hash 改动，规范 §3.2） ===== */
const mode = ref('list')
const catFilter = ref('')
const sort = ref('time')
const q = ref('')
const page = ref(1)
const PAGE_SIZE = 8

const cats = computed(() => catCounts(posts.value))

const barTitle = computed(() => {
  if (mode.value === 'archive') return i18t('文章归档')
  if (mode.value === 'search') return i18t('全文检索')
  return catFilter.value || i18t('全部文章')
})
const barSub = computed(() => {
  if (mode.value === 'archive') return i18t('按更新时间倒序，自动分年归组')
  if (mode.value === 'search') return i18t('在标题、分类与摘要中即时匹配')
  if (catFilter.value) return i18t('分类「{cat}」下的全部内容', { cat: catFilter.value })
  return i18t('目录即站点，文件即文章')
})

/** 只做「读者显式操作」的排序；默认保持系统下发顺序（置顶优先 + 时间倒序） */
const shown = computed(() => {
  const list = catFilter.value ? posts.value.filter((p) => catName(p) === catFilter.value) : posts.value.slice()
  if (sort.value === 'title') {
    return list.sort((a, b) => postTitle(a).localeCompare(postTitle(b), 'zh-Hans-CN'))
  }
  return list
})

const pages = computed(() => Math.max(1, Math.ceil(shown.value.length / PAGE_SIZE)))
const paged = computed(() => {
  const start = (page.value - 1) * PAGE_SIZE
  return shown.value.slice(start, start + PAGE_SIZE)
})

const years = computed(() => yearGroups(posts.value))

const searched = computed(() => {
  const k = q.value.trim().toLowerCase()
  if (!k) return []
  return posts.value.filter((p) => {
    const hay = `${postTitle(p)} ${catName(p)} ${postSummary(p, 200)}`.toLowerCase()
    return hay.includes(k)
  })
})

/* ===== 交互 ===== */
function goPage(n) {
  page.value = Math.min(Math.max(1, n), pages.value)
  if (typeof window !== 'undefined') window.scrollTo({ top: 0, behavior: 'smooth' })
}
function goList() {
  mode.value = 'list'
  if (typeof window !== 'undefined') window.scrollTo({ top: 0, behavior: 'smooth' })
}
function goArchive() {
  mode.value = 'archive'
  if (typeof window !== 'undefined') window.scrollTo({ top: 0, behavior: 'smooth' })
}
function goSearch() {
  mode.value = 'search'
  if (typeof window !== 'undefined') window.scrollTo({ top: 0, behavior: 'smooth' })
}
function onPickCat(name) {
  mode.value = 'list'
  catFilter.value = name
  page.value = 1
  if (typeof window !== 'undefined') window.scrollTo({ top: 0, behavior: 'smooth' })
}
function reload() {
  if (typeof window !== 'undefined') window.location.reload()
}

watch([catFilter, sort], () => { page.value = 1 })

/* 深链兼容：#/blog?view=public&cat=xxx&archive=2026-09&q=xxx（宿主未注入 ctx.page 时回退解析 hash） */
const initCat = readPageParam(ctx.value, ['cat', 'category'], [/[?&]cat=([^&]+)/, /[?&]category=([^&]+)/])
const initArchive = readPageParam(ctx.value, ['archive'], [/[?&]archive=([^&]+)/])
const initQ = readPageParam(ctx.value, ['q'], [/[?&]q=([^&]+)/])
if (initCat) catFilter.value = initCat
if (initQ) { mode.value = 'search'; q.value = initQ }
else if (initArchive) mode.value = 'archive'
</script>
