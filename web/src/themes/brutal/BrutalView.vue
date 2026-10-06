<template>
  <div class="brutal-root">
    <BrutalHead :site-name="siteName" :blog-url="blogUrl">
      <template #stats>
        <span><b>{{ posts.length }}</b> {{ $t('篇文章') }}</span>
        <span v-if="cats.length"><b>{{ cats.length }}</b> {{ $t('个分类') }}</span>
        <span v-if="lastDate"><b>{{ lastDate }}</b> {{ $t('最近更新') }}</span>
      </template>
    </BrutalHead>

    <!-- ══ Hero ══ -->
    <section class="bt-hero">
      <div class="bt-wrap bt-hero-grid">
        <div class="bt-hero-main">
          <span class="bt-tag">{{ $t('AIKLOG · 新粗野主题') }}</span>
          <h2 class="bt-hero-h">
            {{ $t('把想法') }}<br>
            <em>{{ $t('砸') }}</em>{{ $t('成') }}<br>
            {{ $t('看得见的形状') }}
          </h2>
          <p class="bt-hero-lead">{{ siteDesc || $t('目录即站点，文件即文章。写下来，就有人看得见。') }}</p>

          <form class="bt-search" @submit.prevent="scrollToList">
            <label class="bt-search-in">
              <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
                <circle cx="10.5" cy="10.5" r="7" fill="none" stroke="currentColor" stroke-width="3" />
                <path d="M16 16l5.5 5.5" fill="none" stroke="currentColor" stroke-width="3" />
              </svg>
              <input v-model="keyword" type="search" :placeholder="$t('搜索标题或正文摘要…')">
            </label>
            <button class="bt-btn bt-btn-accent" type="submit">{{ $t('开始阅读') }}</button>
          </form>

          <div class="bt-hero-stats">
            <div><b>{{ posts.length }}</b><small>{{ $t('篇文章') }}</small></div>
            <div><b>{{ cats.length }}</b><small>{{ $t('个分类') }}</small></div>
            <div><b>{{ lastDate || '—' }}</b><small>{{ $t('最近更新') }}</small></div>
          </div>
        </div>

        <!-- 拼贴区：数据驱动的装饰块 -->
        <div class="bt-collage">
          <div class="bt-collage-big">
            <span class="bt-collage-no">{{ posts.length }}</span>
            <span class="bt-collage-cap">POSTS<br>ONLINE</span>
          </div>
          <div class="bt-collage-sticker">RAW<br>&amp;<br>BOLD</div>
          <div class="bt-collage-note">
            <b>{{ $t('最近更新') }}</b>
            <span>{{ lastPost ? postTitle(lastPost) : $t('暂无文章') }}</span>
          </div>
        </div>
      </div>
    </section>

    <!-- ══ 三态（规范 §4.1） ══ -->
    <div v-if="loading" class="bt-wrap"><div class="bt-state">{{ $t('正在加载文章…') }}</div></div>
    <div v-else-if="error" class="bt-wrap"><div class="bt-state bt-state-err">{{ error }}</div></div>
    <div v-else-if="!posts.length" class="bt-wrap"><div class="bt-state">{{ $t('还没有公开文章') }}</div></div>

    <template v-else>
      <!-- ══ 精选（仅首页、默认排序、无筛选时展示第一篇，不与网格重复） ══ -->
      <section v-if="featured" class="bt-featured">
        <div class="bt-wrap">
          <div class="bt-sect">
            <h2>{{ $t('头条') }}</h2>
            <span class="bt-rule"></span>
            <span class="bt-sect-note">Top story</span>
          </div>

          <a class="bt-feat" :href="postHref(ctx, featured)">
            <span class="bt-feat-body">
              <span class="bt-tag bt-tag-ink">{{ catName(featured) }}</span>
              <span class="bt-feat-title">{{ postTitle(featured) }}</span>
              <span class="bt-feat-ex">{{ postSummary(featured, 150) || $t("点击阅读全文") }}</span>
              <span class="bt-feat-meta">
                <span class="bt-meta">{{ postDate(featured) }}</span>
                <span v-if="postSize(featured)" class="bt-meta">{{ postSize(featured) }}</span>
                <span v-if="daysAgo(featured)" class="bt-meta">{{ daysAgo(featured) }}</span>
                <span class="bt-btn bt-btn-accent bt-btn-sm">{{ $t('阅读全文') }}</span>
              </span>
            </span>
            <span class="bt-feat-visual">
              <span class="bt-feat-corner">Featured</span>
              <b>{{ catName(featured) }}</b>
            </span>
          </a>
        </div>
      </section>

      <!-- ══ 列表 + 侧栏（左文章 / 右侧栏） ══ -->
      <section class="bt-body">
        <div class="bt-wrap bt-body-grid">
          <div class="bt-list-col" ref="listEl">
            <div class="bt-sect">
              <h2>{{ activeCatName }}</h2>
              <span class="bt-rule"></span>
              <span class="bt-sect-note">{{ sorted.length }} {{ $t('篇') }}</span>
            </div>

            <!-- 分类 chips（path 首段派生，客户端过滤） -->
            <div class="bt-filters">
              <button
                type="button"
                class="bt-chipbtn"
                :class="{ on: activeCat === '__all__' }"
                @click="activeCat = '__all__'"
              >
                {{ $t('全部') }}<i>{{ posts.length }}</i>
              </button>
              <button
                v-for="c in cats"
                :key="c.name"
                type="button"
                class="bt-chipbtn"
                :class="{ on: activeCat === c.name }"
                @click="activeCat = c.name"
              >
                {{ c.name }}<i>{{ c.count }}</i>
              </button>
            </div>

            <div class="bt-bar">
              <span class="bt-bar-info">
                {{ keyword.trim() ? $t("关键词「{keyword}」命中 {sorted} 篇", { keyword: keyword.trim(), sorted: sorted.length }) : $t("共 {sorted} 篇文章", { sorted: sorted.length }) }}
              </span>
              <label class="bt-bar-sort">
                {{ $t('排序') }}
                <select v-model="sortBy">
                  <option value="system">{{ $t('默认（置顶优先）') }}</option>
                  <option value="newest">{{ $t('最新在前') }}</option>
                  <option value="oldest">{{ $t('最早在前') }}</option>
                  <option value="title">{{ $t('按标题') }}</option>
                </select>
              </label>
            </div>

            <!-- 网格（精选已占用的首篇不重复出现） -->
            <div v-if="pageCards.length" class="bt-grid">
              <BrutalCard
                v-for="(p, i) in pageCards"
                :key="p.token || p.path"
                :post="p"
                :index="(page - 1) * PAGE_SIZE + i"
              />
            </div>
            <div v-else class="bt-state">{{ $t("没有匹配的文章，换个分类或关键词试试") }}</div>

            <!-- 分页 -->
            <div v-if="totalPages > 1" class="bt-pager">
              <button type="button" :disabled="page === 1" @click="goto(page - 1)">{{ $t('‹ 上一页') }}</button>
              <button
                v-for="n in pageList"
                :key="n"
                type="button"
                class="bt-pagenum"
                :class="{ on: n === page }"
                @click="goto(n)"
              >
                {{ n }}
              </button>
              <button type="button" :disabled="page === totalPages" @click="goto(page + 1)">{{ $t('下一页 ›') }}</button>
            </div>
          </div>

          <aside class="bt-aside">
            <BrutalSide @pick-cat="onPickCat" />
          </aside>
        </div>
      </section>

      <!-- ══ RSS 引导条 ══ -->
      <section class="bt-cta">
        <div class="bt-wrap">
          <div class="bt-cta-box">
            <div>
              <div class="bt-cta-h">{{ $t('新文章发出时，第一个告诉你') }}</div>
              <p class="bt-cta-p">{{ $t("用 RSS 阅读器订阅，没有广告，也没有中间商。") }}</p>
            </div>
            <a class="bt-btn bt-btn-ink bt-btn-lg" :href="rssHref">{{ $t('订阅 RSS') }}</a>
          </div>
        </div>
      </section>
    </template>

    <BrutalFoot :site-name="siteName" :blog-url="blogUrl" />
  </div>
    <AskWidget />
</template>

<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, ref, watch } from 'vue'
import BrutalHead from './BrutalHead.vue'
import BrutalCard from './BrutalCard.vue'
import BrutalSide from './BrutalSide.vue'
import BrutalFoot from './BrutalFoot.vue'
import './style.css'
import {
  RSS_HREF,
  LIST_HREF,
  catCounts,
  catName,
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

/* 头条区只在「首页 + 默认排序 + 无关键词 + 全部文章」出现，且从网格池中剔除，避免首篇重复 */
const showFeatured = computed(
  () => page.value === 1 && sortBy.value === 'system' && !keyword.value.trim(),
)
const featured = computed(() => (showFeatured.value && sorted.value.length ? sorted.value[0] : null))
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

function scrollToList() {
  listEl.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

/* 筛选条件变化时回到第 1 页 */
watch([activeCat, keyword, sortBy], () => {
  page.value = 1
})
</script>
