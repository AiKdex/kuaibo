<template>
  <div class="vd-root">
    <VerdantHead :site-name="siteName" :blog-url="blogUrl">
      <template #stats>
        <span><b>{{ posts.length }}</b> {{ $t('篇文章') }}</span>
        <span v-if="cats.length"><b>{{ cats.length }}</b> {{ $t('个分类') }}</span>
        <span v-if="lastDate"><b>{{ lastDate }}</b> {{ $t('最近更新') }}</span>
      </template>
    </VerdantHead>

    <!-- ══ Hero ══ -->
    <section class="vd-hero">
      <div class="vd-wrap vd-hero-in">
        <div class="vd-hero-main">
          <span class="vd-pill"><i aria-hidden="true"></i>{{ $t('VERDANT · 明亮清爽主题') }}</span>
          <h2 class="vd-h1">
            {{ $t('把想法种下来，') }}<em>{{ $t('安静地长') }}</em>
          </h2>
          <p class="vd-lead">{{ siteDesc || $t('目录即站点，文件即文章。写下来，就有人看得见。') }}</p>

          <form class="vd-search" @submit.prevent="scrollToList">
            <label class="vd-search-in">
              <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
                <circle cx="10.5" cy="10.5" r="6.6" fill="none" stroke="currentColor" stroke-width="2" />
                <path d="M15.6 15.6l5 5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
              </svg>
              <input v-model="keyword" class="vd-search-field" type="search" :placeholder="$t('搜索标题、摘要或路径…')">
            </label>
            <button class="vd-btn vd-btn-p" type="submit">{{ $t('开始阅读') }}</button>
          </form>

          <div class="vd-hero-stats">
            <div class="vd-hero-stat"><b>{{ posts.length }}</b><span>{{ $t('篇文章') }}</span></div>
            <div class="vd-hero-stat"><b>{{ cats.length }}</b><span>{{ $t('个分类') }}</span></div>
            <div class="vd-hero-stat"><b>{{ lastDate || '—' }}</b><span>{{ $t('最近更新') }}</span></div>
          </div>
        </div>

        <!-- 圆形插图：全部由真实字段驱动（文章数 / 最近更新 / 分类数） -->
        <div class="vd-orb" aria-hidden="true">
          <div class="vd-orb-big"></div>
          <div class="vd-orb-leaf"></div>
          <div class="vd-orb-sun"></div>
          <div class="vd-orb-mid">
            <b class="vd-orb-mid-n">{{ posts.length }}</b>
            <span class="vd-orb-mid-l">{{ $t('篇文章已公开') }}</span>
          </div>
          <div class="vd-orb-card">
            <span class="vd-orb-card-ic">☀</span>
            <span>
              <span class="vd-orb-card-t">{{ $t('最近更新') }} {{ lastDate || '—' }}</span>
              <span class="vd-orb-card-s">{{ lastPost ? postTitle(lastPost) : $t("暂无文章") }}</span>
            </span>
          </div>
          <div class="vd-orb-card vd-orb-card-b">
            <span class="vd-orb-card-ic vd-orb-card-ic-s">✿</span>
            <span>
              <span class="vd-orb-card-t">{{ $t('共') }} {{ cats.length }} {{ $t('个分类') }}</span>
              <span class="vd-orb-card-s">{{ cats.length ? cats[0].name + ' · ' + cats[0].count + $t("篇") : $t("还没有分类") }}</span>
            </span>
          </div>
        </div>
      </div>
    </section>

    <!-- ══ 三态（规范 §4.1） ══ -->
    <div v-if="loading" class="vd-wrap"><div class="vd-state">{{ $t('正在加载文章…') }}</div></div>
    <div v-else-if="error" class="vd-wrap"><div class="vd-state vd-state-err">{{ error }}</div></div>
    <div v-else-if="!posts.length" class="vd-wrap"><div class="vd-state">{{ $t('还没有公开文章') }}</div></div>

    <template v-else>
      <!-- ══ 头条（版面位置，非品质评价；与网格池互斥，首篇不重复） ══ -->
      <section v-if="featured" class="vd-featured">
        <div class="vd-wrap">
          <div class="vd-sect">
            <h2 class="vd-sect-t">{{ $t('头条') }}</h2>
            <span class="vd-rule"></span>
            <span class="vd-sect-note">{{ $t('排在最前的一篇') }}</span>
          </div>

          <a class="vd-feat" :href="postHref(ctx, featured)">
            <span v-if="featCover" class="vd-feat-cover" aria-hidden="true">
              <img :src="featCover" :alt="postTitle(featured)" loading="lazy" @error="featCoverFailed = true" />
            </span>
            <span v-else class="vd-feat-art" :class="catTone(catName(featured))" aria-hidden="true">
              <span class="vd-feat-leaf"></span>
            </span>
            <span class="vd-feat-body">
              <span class="vd-feat-cat">{{ catName(featured) }}</span>
              <span class="vd-feat-title">{{ postTitle(featured) }}</span>
              <span class="vd-feat-ex">{{ postSummary(featured, 160) || $t("点击阅读全文") }}</span>
              <span class="vd-feat-meta">
                <span>{{ postDate(featured) }}</span>
                <span v-if="postSize(featured)">· {{ postSize(featured) }}</span>
                <span v-if="daysAgo(featured)">· {{ daysAgo(featured) }}</span>
                <span class="vd-btn vd-btn-p vd-btn-sm">{{ $t('阅读全文') }}</span>
              </span>
            </span>
          </a>
        </div>
      </section>

      <!-- ══ 列表 + 侧栏 ══ -->
      <section class="vd-body">
        <div class="vd-wrap vd-body-grid">
          <div ref="listEl" class="vd-list-col">
            <div class="vd-sect">
              <h2 class="vd-sect-t">{{ activeCatName }}</h2>
              <span class="vd-rule"></span>
              <span class="vd-sect-note">{{ sorted.length }} {{ $t('篇') }}</span>
            </div>

            <div class="vd-filters">
              <button
                type="button"
                class="vd-chipbtn"
                :class="{ on: activeCat === '__all__' }"
                @click="activeCat = '__all__'"
              >
                {{ $t('全部') }}<i>{{ posts.length }}</i>
              </button>
              <button
                v-for="c in cats"
                :key="c.name"
                type="button"
                class="vd-chipbtn"
                :class="{ on: activeCat === c.name }"
                @click="activeCat = c.name"
              >
                {{ c.name }}<i>{{ c.count }}</i>
              </button>
            </div>

            <div class="vd-bar">
              <span class="vd-bar-info">
                {{ keyword.trim() ? $t("关键词「{keyword}」命中 {sorted} 篇", { keyword: keyword.trim(), sorted: sorted.length }) : $t("共 {sorted} 篇文章", { sorted: sorted.length }) }}
              </span>
              <label class="vd-bar-sort">
                {{ $t('排序') }}
                <select v-model="sortBy">
                  <option value="system">{{ $t('默认（置顶优先）') }}</option>
                  <option value="newest">{{ $t('最新在前') }}</option>
                  <option value="oldest">{{ $t('最早在前') }}</option>
                  <option value="title">{{ $t('按标题') }}</option>
                </select>
              </label>
            </div>

            <div v-if="pageCards.length" class="vd-grid">
              <VerdantCard
                v-for="(p, i) in pageCards"
                :key="p.token || p.path"
                :post="p"
                :index="(page - 1) * PAGE_SIZE + i"
              />
            </div>
            <div v-else class="vd-state">{{ $t("没有匹配的文章，换个分类或关键词试试") }}</div>

            <div v-if="totalPages > 1" class="vd-pager">
              <button type="button" :disabled="page === 1" @click="goto(page - 1)">{{ $t('‹ 上一页') }}</button>
              <button
                v-for="n in pageList"
                :key="n"
                type="button"
                class="vd-pagenum"
                :class="{ on: n === page }"
                @click="goto(n)"
              >
                {{ n }}
              </button>
              <button type="button" :disabled="page === totalPages" @click="goto(page + 1)">{{ $t('下一页 ›') }}</button>
            </div>
          </div>

          <aside class="vd-aside">
            <VerdantSide @pick-cat="onPickCat" />
          </aside>
        </div>
      </section>

      <!-- ══ RSS 引导 ══ -->
      <section class="vd-cta">
        <div class="vd-wrap">
          <div class="vd-cta-box">
            <div>
              <div class="vd-cta-h">{{ $t('新文章发出时，第一个告诉你') }}</div>
              <p class="vd-cta-p">{{ $t("用 RSS 阅读器订阅，没有广告，也没有中间商。") }}</p>
            </div>
            <a class="vd-btn vd-btn-inv vd-btn-lg" :href="rssHref">{{ $t('订阅 RSS') }}</a>
          </div>
        </div>
      </section>
    </template>

    <VerdantFoot :site-name="siteName" :blog-url="blogUrl" :posts="posts" @pick-cat="onPickCat" />
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, ref, watch } from 'vue'
import VerdantHead from './VerdantHead.vue'
import VerdantCard from './VerdantCard.vue'
import VerdantSide from './VerdantSide.vue'
import VerdantFoot from './VerdantFoot.vue'
import './style.css'
import {
  LIST_HREF,
  RSS_HREF,
  catCounts,
  catName,
  catTone,
  daysAgo,
  postDate,
  postHref,
  postSize,
  postSummary,
  postTitle,
  useBlog,
} from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)
const rssHref = RSS_HREF

const cats = computed(() => catCounts(posts.value))
const lastPost = computed(() => {
  let best = null
  for (const p of posts.value) {
    if (!best || (p.file?.updated_at || 0) > (best.file?.updated_at || 0)) best = p
  }
  return best
})
const lastDate = computed(() => (lastPost.value ? postDate(lastPost.value, 'short') : ''))

/* 过滤：分类（客户端）+ 关键词（读者显式输入） */
const activeCat = ref('__all__')
const keyword = ref('')
const sortBy = ref('system')

const activeCatName = computed(() => (activeCat.value === '__all__' ? t('全部文章') : activeCat.value))

const baseList = computed(() => {
  let arr = posts.value
  if (activeCat.value !== '__all__') arr = arr.filter((p) => catName(p) === activeCat.value)
  const k = keyword.value.trim().toLowerCase()
  if (k) {
    arr = arr.filter((p) =>
      (postTitle(p) + ' ' + (p.preview || '') + ' ' + (p.path || '')).toLowerCase().includes(k),
    )
  }
  return arr
})

/* 排序仅在读者显式选择时生效，默认保持系统顺序（规范 §2.4） */
const sorted = computed(() => {
  const arr = [...baseList.value]
  if (sortBy.value === 'newest') {
    arr.sort((a, b) => (b.file?.updated_at || 0) - (a.file?.updated_at || 0))
  } else if (sortBy.value === 'oldest') {
    arr.sort((a, b) => (a.file?.updated_at || 0) - (b.file?.updated_at || 0))
  } else if (sortBy.value === 'title') {
    arr.sort((a, b) => postTitle(a).localeCompare(postTitle(b), 'zh-CN'))
  }
  return arr
})

/* 分页 */
const PAGE_SIZE = 9
const page = ref(1)
const listEl = ref(null)

/* 头条区只在「首页 + 默认排序 + 无关键词」出现，且从网格池中剔除，避免首篇重复 */
const showFeatured = computed(
  () => page.value === 1 && sortBy.value === 'system' && !keyword.value.trim(),
)
const featured = computed(() => (showFeatured.value && sorted.value.length ? sorted.value[0] : null))

// 头条封面：优先文章封面字段，回退青野有机图形占位；加载失败回退占位
const featCoverFailed = ref(false)
const featCover = computed(() => {
  if (featCoverFailed.value) return ''
  const p = featured.value || {}
  const c = p.file?.cover
  if (typeof c === 'string' && c.trim()) return c.trim()
  const s = p.preview || ''
  const m = /!\[[^\]]*\]\((https?:\/\/[^)\s]+|\/[^)\s]+)\)/.exec(s)
  return m ? m[1] : ''
})
const gridPool = computed(() => (featured.value ? sorted.value.slice(1) : sorted.value))

const totalPages = computed(() => Math.max(1, Math.ceil(gridPool.value.length / PAGE_SIZE)))
const pageCards = computed(() => {
  const p = Math.min(page.value, totalPages.value)
  return gridPool.value.slice((p - 1) * PAGE_SIZE, p * PAGE_SIZE)
})
const pageList = computed(() => {
  const total = totalPages.value
  const cur = Math.min(page.value, total)
  const from = Math.max(1, Math.min(cur - 2, total - 4))
  return Array.from({ length: Math.min(5, total) }, (_, i) => from + i)
})

function goto(n) {
  page.value = Math.min(Math.max(1, n), totalPages.value)
  listEl.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function onPickCat(name) {
  activeCat.value = name
  scrollToList()
}

/* 页内跳转一律 scrollIntoView，绝不改动 location.hash（规范 §3.2） */
function scrollToList() {
  listEl.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

watch([activeCat, keyword, sortBy], () => {
  page.value = 1
})
</script>

<style scoped>
.vd-feat-cover {
  position: relative;
  min-height: 220px;
  overflow: hidden;
  background: var(--th-leaf, #eef5e9);
}
.vd-feat-cover img {
  display: block;
  width: 100%;
  height: 100%;
  min-height: 220px;
  object-fit: cover;
}
</style>
