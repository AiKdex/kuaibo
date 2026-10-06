<template>
  <div class="eforum-root">
    <EmforumHead :site-name="siteName" :site-desc="siteDesc" :blog-url="blogUrl" :slogan="$t('TAG · 标签版块')" />

    <div v-if="loading" class="ef-state">{{ $t('正在加载文章…') }}</div>
    <div v-else-if="error" class="ef-state ef-state-err">{{ error }}</div>

    <div v-else class="ef-wrap">
      <!-- 页面信息条 -->
      <div class="ef-board-info">
        <div>
          <div class="ef-board-name">{{ $t('🏷 标签：') }}{{ tag || $t("全部") }}</div>
          <div class="ef-board-desc">{{ $t('共') }} {{ list.length }} {{ $t('篇 · 标签来自文章元数据') }}</div>
        </div>
        <div class="ef-board-actions">
          <a class="ef-ghost-btn" :href="blogUrl">{{ $t('‹ 返回全部版块') }}</a>
        </div>
      </div>

      <!-- 标签切换（客户端过滤；系统路由 /blog/tag/:tag 就绪后由 URL 驱动） -->
      <nav v-if="tags.length" class="ef-chips">
        <button
          v-for="t in tags"
          :key="t.name"
          type="button"
          class="ef-chip"
          :class="{ on: t.name === (tag || '') }"
          @click="pickTag(t.name)"
        >
          {{ t.name }}<i>{{ t.count }}</i>
        </button>
      </nav>

      <div class="ef-layout">
        <main class="ef-list">
          <div class="ef-list-head">
            <span class="ef-feed">{{ $t('本标签共') }} {{ list.length }} {{ $t('个主题') }}</span>
          </div>
          <EmforumThread v-for="p in list" :key="p.token || p.path" :post="p" :show-cat="true" />
          <p v-if="!list.length" class="ef-empty">{{ $t('该标签下暂无文章') }}</p>
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
import EmforumThread from './EmforumThread.vue'
import EmforumAsk from './EmforumAsk.vue'
import './style.css'
import { postTitle, readPageParam, useBlog, LIST_HREF } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => ctx.value.tags || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

/* 当前标签：优先 ctx.page.tag，回退解析 #/blog/tag/:tag 或 ?tag= */
const pageParam = computed(() =>
  readPageParam(ctx.value, ['tag'], [/\/tag\/([^/?#]+)/, /[?&]tag=([^&#]+)/]),
)
const activeTag = ref(pageParam.value)
const tag = computed(() => activeTag.value || pageParam.value)

function pickTag(name) {
  activeTag.value = name
}

const list = computed(() => {
  if (!tag.value) return posts.value
  const t = String(tag.value).toLowerCase()
  return posts.value.filter((p) =>
    (p.file?.tags || []).some((x) => String(x).toLowerCase() === t),
  )
})
</script>
