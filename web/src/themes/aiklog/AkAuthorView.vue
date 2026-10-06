<template>
  <div class="ak-post">
    <header class="ak-post-top">
      <a class="ak-post-brand" :href="listHref">{{ siteName }}</a>
      <nav class="ak-post-nav">
        <ThemeSwitch />
        <a :href="listHref">{{ $t('返回列表') }}</a>
      </nav>
    </header>

    <div v-if="loading" class="ak-state">{{ $t('加载中…') }}</div>
    <div v-else-if="error" class="ak-state ak-state-err">{{ error }}</div>

    <div v-else class="ak-wrap">
      <div class="ak-page-head">
        <h1 class="ak-page-title">{{ $t('作者：') }}{{ author || $t("未知作者") }}</h1>
        <div class="ak-page-sub">{{ $t('共') }} {{ list.length }} {{ $t('篇') }}</div>
      </div>

      <div class="ak-layout">
        <main class="ak-post-main">
          <ul v-if="list.length" class="ak-list">
            <li v-for="p in list" :key="pKey(p)">
              <a class="ak-list-link" :href="postHref(ctx.value, p)">
                <h3 class="ak-list-title">{{ postTitle(p) }}</h3>
                <div class="ak-list-meta">
                  <span v-if="postCat(p)">{{ postCat(p) }}</span>
                  <span v-if="postDate(p)">{{ postDate(p) }}</span>
                </div>
              </a>
            </li>
          </ul>
          <p v-else class="ak-state">{{ authorHint }}</p>
        </main>

        <aside class="ak-aside">
          <SideWidget />
        </aside>
      </div>
    </div>

    <footer class="ak-post-foot-bar">
      <div class="ak-post-foot-in">
        <span>{{ siteName }}</span>
        <a :href="rssHref">RSS</a>
      </div>
    </footer>
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
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '爱库录')
const listHref = '#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'

const author = computed(
  () => ctx.value?.page?.author || readPageParam(ctx.value, ['author', 'username'], [/\/author\/([^/?#]+)/, /[?&]author=([^&#]+)/]),
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
  hasAuthorField.value ? t('该作者下暂无公开文章') : t('作者数据尚未由系统侧透出，敬请期待'),
)

function pKey(p) {
  return p.path || p.file?.slug || p.token || Math.random()
}
</script>
