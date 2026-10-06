<template>
  <div class="eforum-root">
    <EmforumHead :site-name="siteName" :site-desc="siteDesc" :blog-url="blogUrl">
      <template #stats>
        <span><b>{{ posts.length }}</b> {{ $t('主题') }}</span>
        <span><b>{{ boards.length }}</b> {{ $t('版块') }}</span>
        <span v-if="lastDate"><b>{{ lastDate }}</b> {{ $t('最近更新') }}</span>
      </template>
      <template #boardbar>
        <!-- 版块页签条：客户端按分类（path 首段）过滤 -->
        <nav class="ef-boardbar">
          <div class="ef-wrap">
            <button
              v-for="b in boardTabs"
              :key="b.key"
              type="button"
              class="ef-btab"
              :class="{ on: b.key === activeBoard }"
              @click="pickBoard(b.key)"
            >
              <span class="ef-ico">{{ b.icon }}</span>{{ b.name }}<span class="ef-cnt">{{ b.count }}</span>
            </button>
          </div>
        </nav>
      </template>
    </EmforumHead>

    <!-- 三态：加载中 / 错误 / 空态（规范 §4.1） -->
    <div v-if="loading" class="ef-state">{{ $t('正在加载文章…') }}</div>
    <div v-else-if="error" class="ef-state ef-state-err">{{ error }}</div>
    <div v-else-if="!posts.length" class="ef-state">{{ $t('还没有公开文章') }}</div>

    <div v-else class="ef-wrap">
      <!-- 当前版块信息条 -->
      <div class="ef-board-info">
        <div>
          <div class="ef-board-name">{{ activeName.icon }} {{ activeName.name }}</div>
          <div class="ef-board-desc">{{ $t('共') }} {{ filtered.length }} {{ $t('个主题 · 点击标题阅读全文') }}</div>
        </div>
        <div class="ef-board-actions">
          <label class="ef-sort-label">
            {{ $t('排序') }}
            <select v-model="sortBy" class="ef-sortsel" @change="page = 1">
              <option value="system">{{ $t('默认（置顶优先）') }}</option>
              <option value="newest">{{ $t('按更新时间') }}</option>
              <option value="oldest">{{ $t('按时间正序') }}</option>
              <option value="title">{{ $t('按标题') }}</option>
            </select>
          </label>
        </div>
      </div>

      <div class="ef-layout">
        <main class="ef-list">
          <div class="ef-list-head">
            <span class="ef-feed">{{ activeName.name }} {{ $t('· 第') }} {{ page }} / {{ totalPages }} {{ $t('页') }}</span>
            <span v-if="totalPages > 1" class="ef-pager">
              <button type="button" :disabled="page === 1" @click="page--">{{ $t('‹ 上一页') }}</button>
              <span class="ef-page-on">{{ page }}</span>
              <button type="button" :disabled="page === totalPages" @click="page++">{{ $t('下一页 ›') }}</button>
            </span>
          </div>

          <a
            v-for="post in pagePosts"
            :key="post.token || post.path"
            :href="postHref(ctx, post)"
            class="ef-thread"
          >
            <span class="ef-avatar" :class="boardColor(postCat(post) || $t('未分类'))">{{ avaText(post) }}</span>
            <span class="ef-thread-main">
              <span class="ef-row1">
                <span v-if="postCat(post)" class="ef-tagline">{{ postCat(post) }}</span>
                <span class="ef-title">{{ postTitle(post) }}</span>
              </span>
              <span class="ef-row2">
                <span>{{ postDate(post) }}</span>
                <span v-if="daysAgo(post)" class="ef-dot">·</span>
                <span>{{ daysAgo(post) }}</span>
                <span v-if="postSize(post)" class="ef-dot">·</span>
                <span>{{ postSize(post) }}</span>
              </span>
              <span v-if="postSummary(post)" class="ef-excerpt">{{ postSummary(post) }}</span>
            </span>
            <span class="ef-thread-side">{{ postDate(post, 'short') }}</span>
          </a>

          <div v-if="totalPages > 1" class="ef-pagebar">
            <button
              v-for="n in pageList"
              :key="n"
              type="button"
              class="ef-pagenum"
              :class="{ on: n === page }"
              @click="page = n"
            >
              {{ n }}
            </button>
          </div>
        </main>

        <EmforumSide />
      </div>
    </div>

    <EmforumFoot :site-name="siteName" :blog-url="blogUrl" />
    <EmforumAsk />
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import EmforumHead from './EmforumHead.vue'
import EmforumSide from './EmforumSide.vue'
import EmforumFoot from './EmforumFoot.vue'
import EmforumAsk from './EmforumAsk.vue'
import './style.css'
import {
  boardColor,
  boardCounts,
  daysAgo,
  postCat,
  postDate,
  postHref,
  postSize,
  postSummary,
  postTitle,
  useBlog,
  LIST_HREF,
} from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

/* 版块页签（分类 → 篇数） */
const boards = computed(() => boardCounts(posts.value))
const boardTabs = computed(() => [
  { key: '__all__', name: '全部版块', count: posts.value.length, icon: '◎' },
  ...boards.value.map((b) => ({ key: b.name, name: b.name, count: b.count, icon: b.icon })),
])

const activeBoard = ref('__all__')
const activeName = computed(
  () => boardTabs.value.find((x) => x.key === activeBoard.value) || boardTabs.value[0],
)
function pickBoard(key) {
  activeBoard.value = key
  page.value = 1
}

/* 过滤 + 可选排序（默认保持系统顺序：置顶优先 + 时间倒序，规范 §2.4） */
const filtered = computed(() =>
  activeBoard.value === '__all__'
    ? posts.value
    : posts.value.filter((p) => (postCat(p) || '未分类') === activeBoard.value),
)

const sortBy = ref('system')
const sorted = computed(() => {
  const arr = [...filtered.value]
  if (sortBy.value === 'newest') arr.sort((a, b) => (b.file?.updated_at || 0) - (a.file?.updated_at || 0))
  else if (sortBy.value === 'oldest') arr.sort((a, b) => (a.file?.updated_at || 0) - (b.file?.updated_at || 0))
  else if (sortBy.value === 'title') arr.sort((a, b) => postTitle(a).localeCompare(postTitle(b), 'zh-CN'))
  return arr
})

/* 分页 */
const PAGE_SIZE = 10
const page = ref(1)
const totalPages = computed(() => Math.max(1, Math.ceil(sorted.value.length / PAGE_SIZE)))
const pagePosts = computed(() => {
  const p = Math.min(page.value, totalPages.value)
  return sorted.value.slice((p - 1) * PAGE_SIZE, p * PAGE_SIZE)
})
const pageList = computed(() => {
  const total = totalPages.value
  const cur = Math.min(page.value, total)
  const from = Math.max(1, Math.min(cur - 2, total - 4))
  return Array.from({ length: Math.min(5, total) }, (_, i) => from + i)
})
watch(totalPages, (t) => {
  if (page.value > t) page.value = 1
})

const lastDate = computed(() => {
  const t = Math.max(0, ...posts.value.map((p) => p.file?.updated_at || 0))
  return t ? postDate({ file: { updated_at: t } }, 'short') : ''
})

function avaText(p) {
  return postTitle(p).trim()[0] || '文'
}
</script>
