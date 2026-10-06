<template>
  <div class="qz-author">
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
        <h1 class="qz-p-page-title">{{ $t('作者：') }}{{ author || $t("未知作者") }}</h1>
        <div class="qz-p-page-sub">{{ author ? (list.length + $t("篇")) : authorHint }}</div>
      </div>

      <div class="qz-layout">
        <main class="qz-p-main">
          <ul v-if="list.length" class="qz-p-list">
            <li v-for="p in list" :key="pKey(p)">
              <a class="qz-p-link" :href="postHref(ctx.value, p)">
                <h3 class="qz-p-list-title">{{ postTitle(p) }}</h3>
                <div class="qz-p-meta">
                  <span v-if="postCat(p)">{{ postCat(p) }}</span>
                  <span v-if="postDate(p)">{{ postDate(p) }}</span>
                </div>
              </a>
            </li>
          </ul>
          <p v-else class="qz-state">{{ author ? $t("该作者下暂无公开文章") : authorHint }}</p>
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
import { t } from '@/i18n'
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
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '求智')
const listHref = '#/blog?view=public'

const author = computed(() =>
  readPageParam(ctx.value, ['author', 'username'], [/\/author\/([^/?#]+)/, /[?&]author=([^&#]+)/]),
)

function authorOf(p) {
  const a = p.file?.author
  if (!a) return ''
  return typeof a === 'string' ? a : a.name || a.username || ''
}

const hasAuthorField = computed(() => posts.value.some((p) => !!authorOf(p)))
const list = computed(() =>
  author.value ? posts.value.filter((p) => authorOf(p) === author.value) : [],
)
const authorHint = computed(() =>
  hasAuthorField.value
    ? t('作者数据尚未由系统侧透出（规范 §10-10），敬请期待')
    : t('作者数据尚未由系统侧透出（规范 §10-10），敬请期待'),
)

function pKey(p) {
  return p.path || p.file?.slug || p.token || Math.random()
}
</script>
