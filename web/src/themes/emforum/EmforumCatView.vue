<template>
  <div class="eforum-root">
    <EmforumHead :site-name="siteName" :site-desc="siteDesc" :blog-url="blogUrl" :slogan="$t('CATEGORY · 分类版块')" />

    <div v-if="loading" class="ef-state">{{ $t('正在加载文章…') }}</div>
    <div v-else-if="error" class="ef-state ef-state-err">{{ error }}</div>

    <div v-else class="ef-wrap">
      <!-- 页面信息条 -->
      <div class="ef-board-info">
        <div>
          <div class="ef-board-name">{{ icon }} {{ $t('分类：') }}{{ cat || $t("未分类") }}</div>
          <div class="ef-board-desc">{{ $t('共') }} {{ list.length }} {{ $t('篇 · 分类 = 目录首段') }}</div>
        </div>
        <div class="ef-board-actions">
          <a class="ef-ghost-btn" :href="blogUrl">{{ $t('‹ 返回全部版块') }}</a>
        </div>
      </div>

      <!-- 分类切换（客户端过滤；系统路由 /blog/cat/:cat 就绪后由 URL 驱动） -->
      <nav v-if="cats.length > 1" class="ef-chips">
        <button
          v-for="c in cats"
          :key="c.name"
          type="button"
          class="ef-chip"
          :class="{ on: c.name === (cat || $t('未分类')) }"
          @click="pickCat(c.name)"
        >
          {{ c.icon }} {{ c.name }}<i>{{ c.count }}</i>
        </button>
      </nav>

      <div class="ef-layout">
        <main class="ef-list">
          <div class="ef-list-head">
            <span class="ef-feed">{{ $t('本版共') }} {{ list.length }} {{ $t('个主题') }}</span>
          </div>
          <EmforumThread v-for="p in list" :key="p.token || p.path" :post="p" :show-cat="false" />
          <p v-if="!list.length" class="ef-empty">{{ $t('该分类下暂无文章') }}</p>
        </main>

        <EmforumSide />
      </div>
    </div>

    <EmforumFoot :site-name="siteName" :blog-url="blogUrl" />
    <EmforumAsk />
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import EmforumHead from './EmforumHead.vue'
import EmforumSide from './EmforumSide.vue'
import EmforumFoot from './EmforumFoot.vue'
import EmforumAsk from './EmforumAsk.vue'
import EmforumThread from './EmforumThread.vue'
import './style.css'
import { boardCounts, postCat, readPageParam, useBlog, LIST_HREF } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

const cats = computed(() => boardCounts(posts.value))

/* 当前分类：优先 ctx.page.cat，回退解析 #/blog/cat/:cat 或 ?cat= */
const pageParam = computed(() =>
  readPageParam(ctx.value, ['cat', 'category'], [/\/cat\/([^/?#]+)/, /[?&]cat=([^&#]+)/]),
)
const activeCat = ref(pageParam.value)
const cat = computed(() => activeCat.value || pageParam.value)

function pickCat(name) {
  activeCat.value = name
}

const icon = computed(() => cats.value.find((c) => c.name === cat.value)?.icon || '✦')

const list = computed(() =>
  posts.value.filter((p) => (postCat(p) || '未分类') === (cat.value || '未分类')),
)
</script>
