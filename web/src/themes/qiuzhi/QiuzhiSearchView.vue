<template>
  <div class="qz-search">
    <header class="qz-topbar qz-p-topbar">
      <div class="qz-container qz-p-bar-in">
        <a class="qz-p-brand" :href="listHref">{{ siteName }}</a>
        <nav class="qz-p-nav">
          <ThemeSwitch />
          <a :href="listHref">{{ $t('返回列表') }}</a>
        </nav>
      </div>
    </header>

    <div v-if="loading" class="qz-state">{{ $t('加载中…') }}</div>
    <div v-else-if="error" class="qz-state qz-state-err">{{ error }}</div>

    <div v-else class="qz-container qz-p-wrap">
      <div class="qz-p-page-head">
        <h1 class="qz-p-page-title">{{ $t('站内搜索') }}</h1>
      </div>

      <div class="qz-p-search-bar">
        <input
          v-model="q"
          type="search"
          :placeholder="$t('输入关键词，检索标题与摘要…')"
          @keyup.enter="go"
        />
        <button type="button" class="qz-p-search-btn" :disabled="!q.trim()" @click="go">{{ $t('搜 索') }}</button>
      </div>
      <span class="qz-p-search-hint">
        {{ searched ? $t("命中 {list} 篇 / 共 {posts} 篇公开文章", { list: list.length, posts: posts.length }) : $t("仅检索已加载的公开文章") }}
      </span>

      <div class="qz-layout">
        <main class="qz-p-main">
          <div class="qz-p-list-head">
            {{ searched ? $t('关键词「{kw}」的搜索结果', { kw: keyword }) : $t("最新更新 · 输入关键词开始检索") }}
          </div>

          <ul v-if="shown.length" class="qz-p-list">
            <li v-for="p in shown" :key="pKey(p)">
              <a class="qz-p-link" :href="postHref(ctx.value, p)">
                <h3 class="qz-p-list-title">{{ postTitle(p) }}</h3>
                <div class="qz-p-meta">
                  <span v-if="postCat(p)">{{ postCat(p) }}</span>
                  <span v-if="postDate(p)">{{ postDate(p) }}</span>
                </div>
              </a>
            </li>
          </ul>
          <p v-else class="qz-state">
            {{ searched ? $t("没有匹配的文章，换个关键词试试") : $t("暂无文章") }}
          </p>
        </main>
        <aside class="qz-aside">
          <SideWidget />
        </aside>
      </div>
    </div>
  </div>
  <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
import SideWidget from '../SideWidget.vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import { computed, ref } from 'vue'
import { useBlog, postTitle, postCat, postDate, postHref, postSummary, readPageParam } from './helpers.js'
import './post.css'

const ctx = useBlog()
const posts = computed(() => ctx.value?.posts || [])
const loading = computed(() => !!ctx.value?.loading)
const error = computed(() => ctx.value?.error || '')
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '求智')
const listHref = '#/blog?view=public'

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

const fallback = computed(() =>
  [...posts.value].sort((a, b) => (b.file?.updated_at || 0) - (a.file?.updated_at || 0)).slice(0, 8),
)
const shown = computed(() => (searched.value ? list.value : fallback.value))

function pKey(p) {
  return p.path || p.file?.slug || p.token || Math.random()
}
</script>
