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
        <h1 class="el-page-title">{{ $t('分类：') }}{{ cat || $t("未分类") }}</h1>
        <div class="el-page-sub">{{ $t('共') }} {{ list.length }} {{ $t('篇') }}</div>
      </div>

      <div class="el-layout">
        <main class="el-post-main">
          <ul v-if="list.length" class="el-list">
            <li v-for="p in list" :key="pKey(p)">
              <a class="el-list-link" :href="postHref(ctx.value, p)">
                <h3 class="el-list-title">{{ postTitle(p) }}</h3>
                <div class="el-list-meta">
                  <span v-if="postCat(p)">{{ postCat(p) }}</span>
                  <span v-if="postDate(p)">{{ postDate(p) }}</span>
                </div>
              </a>
            </li>
          </ul>
          <p v-else class="el-state">{{ $t('该分类下暂无文章') }}</p>
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
