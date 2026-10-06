<template>
  <div class="el-post">
    <header class="el-post-top">
      <a class="el-post-brand" :href="listHref">{{ siteName }}</a>
      <nav class="el-post-nav">
        <ThemeSwitch />
        <a :href="listHref">{{ $t('返回列表') }}</a>
      </nav>
    </header>

    <div v-if="loading" class="el-state">{{ $t('加载中…') }}</div>
    <div v-else-if="error" class="el-state el-state-err">{{ error }}</div>

    <div v-else class="el-pwrap">
      <div class="el-page-head">
        <h1 class="el-page-title">{{ $t('搜索') }}</h1>
        <div class="el-page-sub">{{ $t('仅检索已加载的公开文章') }}</div>
      </div>

      <div class="el-layout">
        <main class="el-post-main">
          <div class="el-search-bar">
            <div class="el-search-input">
              <input
                v-model="q"
                type="search"
                :placeholder="$t('输入关键词，检索标题与摘要…')"
                @keyup.enter="go"
              />
              <button type="button" :disabled="!q.trim()" @click="go">{{ $t('搜索') }}</button>
            </div>
            <span class="el-search-hint">
              {{ searched ? $t("命中 {list} 篇 / 共 {posts} 篇", { list: list.length, posts: posts.length }) : $t("输入关键词开始检索") }}
            </span>
          </div>

          <ul v-if="shown.length" class="el-list">
            <li v-for="p in shown" :key="pKey(p)">
              <a class="el-list-link" :href="postHref(ctx.value, p)">
                <h3 class="el-list-title">{{ postTitle(p) }}</h3>
                <div class="el-list-meta">
                  <span v-if="postCat(p)">{{ postCat(p) }}</span>
                  <span v-if="postDate(p)">{{ postDate(p) }}</span>
                </div>
              </a>
            </li>
          </ul>
          <p v-else-if="searched" class="el-state">{{ $t('没有匹配「') }}{{ keyword }}{{ $t('」的文章，换个关键词试试') }}</p>
          <p v-else class="el-state">{{ $t("最新更新 · 输入关键词开始检索") }}</p>
        </main>

        <aside class="el-aside">
          <SideWidget />
        </aside>
      </div>
    </div>

    <footer class="el-post-foot-bar">
      <div class="el-post-foot-in">
        <span>{{ siteName }}</span>
        <a :href="rssHref">RSS</a>
      </div>
    </footer>
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
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '爱库录')
const listHref = '#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'

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
