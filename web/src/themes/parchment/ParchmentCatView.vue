<template>
  <div class="pa-post">
    <header class="pa-post-top">
      <a class="pa-post-brand" :href="listHref">{{ siteName }}</a>
      <nav class="pa-post-nav">
        <ThemeSwitch />
        <a :href="listHref">{{ $t('返回列表') }}</a>
      </nav>
    </header>

    <div v-if="loading" class="pa-state">{{ $t('加载中…') }}</div>
    <div v-else-if="error" class="pa-state pa-state-err">{{ error }}</div>

    <div v-else class="pa-pwrap">
      <div class="pa-page-head">
        <h1 class="pa-page-title">{{ $t('分类：') }}{{ cat || $t("未分类") }}</h1>
        <div class="pa-page-sub">{{ $t('共') }} {{ list.length }} {{ $t('篇') }}</div>
      </div>

      <div class="pa-layout">
        <main class="pa-post-main">
          <ul v-if="list.length" class="pa-list">
            <li v-for="p in list" :key="pKey(p)">
              <a class="pa-list-link" :href="postHref(ctx.value, p)">
                <h3 class="pa-list-title">{{ postTitle(p) }}</h3>
                <div class="pa-list-meta">
                  <span v-if="postCat(p)">{{ postCat(p) }}</span>
                  <span v-if="postDate(p)">{{ postDate(p) }}</span>
                </div>
              </a>
            </li>
          </ul>
          <p v-else class="pa-state">{{ $t('该分类下暂无文章') }}</p>
        </main>

        <aside class="pa-aside">
          <SideWidget />
        </aside>
      </div>
    </div>

    <footer class="pa-post-foot-bar">
      <div class="pa-post-foot-in">
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
import { computed } from 'vue'
import { useBlog, postTitle, postCat, postDate, postHref, readPageParam } from './helpers.js'
import './post.css'

const ctx = useBlog()
const posts = computed(() => ctx.value?.posts || [])
const loading = computed(() => !!ctx.value?.loading)
const error = computed(() => ctx.value?.error || '')
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '爱库录')
const listHref = '#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'

const cat = computed(
  () =>
    ctx.value?.page?.cat ||
    readPageParam(ctx.value, ['cat', 'category'], [/\/cat\/([^/?#]+)/, /[?&]cat=([^&#]+)/]) ||
    '未分类',
)
const list = computed(() => posts.value.filter((p) => (postCat(p) || '未分类') === cat.value))

function pKey(p) {
  return p.path || p.file?.slug || p.token || Math.random()
}
</script>
