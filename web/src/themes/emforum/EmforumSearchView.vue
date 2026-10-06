<template>
  <div class="eforum-root">
    <EmforumHead :site-name="siteName" :site-desc="siteDesc" :blog-url="blogUrl" :slogan="$t('SEARCH · 站内搜索')" />

    <div v-if="loading" class="ef-state">{{ $t('正在加载文章…') }}</div>
    <div v-else-if="error" class="ef-state ef-state-err">{{ error }}</div>

    <div v-else class="ef-wrap">
      <!-- 搜索条 -->
      <div class="ef-board-info ef-search-bar">
        <div class="ef-search-input">
          <input
            v-model="q"
            type="search"
            :placeholder="$t('输入关键词，检索标题与摘要…')"
            @keyup.enter="go"
          />
          <button class="ef-search-btn" type="button" :disabled="!q.trim()" @click="go">{{ $t('搜 索') }}</button>
        </div>
        <span class="ef-search-hint">
          {{ searched ? $t("命中 {list} 篇 / 共 {posts} 篇公开文章", { list: list.length, posts: posts.length }) : $t("仅检索已加载的公开文章") }}
        </span>
      </div>

      <div class="ef-layout">
        <main class="ef-list">
          <div class="ef-list-head">
            <span class="ef-feed">
              {{ searched ? $t('关键词「{keyword}」的搜索结果', { keyword }) : $t("最新更新 · 输入关键词开始检索") }}
            </span>
          </div>

          <EmforumThread
            v-for="p in shown"
            :key="p.token || p.path"
            :post="p"
          />

          <p v-if="searched && !list.length" class="ef-empty">
            {{ $t('没有匹配「') }}{{ keyword }}{{ $t('」的文章，换个关键词试试') }}
          </p>
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
import { postSummary, postTitle, readPageParam, useBlog, LIST_HREF } from './helpers.js'

/**
 * 搜索页 —— 规范 §4.8：ctx.page.q + 在 ctx.posts 内做客户端匹配。
 * 现状：系统侧尚未注入 ctx.page，回退解析 ?q= / hash，并记录最近关键词。
 */
const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

const q = ref(readPageParam(ctx.value, ['q', 'keyword'], [/[?&]q=([^&#]+)/, /\/search\/([^/?#]+)/]))
const keyword = ref(q.value.trim())
const searched = computed(() => !!keyword.value)

function go() {
  keyword.value = q.value.trim()
}

const list = computed(() => {
  const kw = keyword.value.toLowerCase()
  if (!kw) return []
  return posts.value.filter((p) => {
    const hay = (postTitle(p) + ' ' + postSummary(p, 300)).toLowerCase()
    return hay.includes(kw)
  })
})

/* 无关键词时展示最新 8 篇，避免空白页 */
const fallback = computed(() =>
  [...posts.value].sort((a, b) => (b.file?.updated_at || 0) - (a.file?.updated_at || 0)).slice(0, 8),
)
const shown = computed(() => (searched.value ? list.value : fallback.value))
</script>
