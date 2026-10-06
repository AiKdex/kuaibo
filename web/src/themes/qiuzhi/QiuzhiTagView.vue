<template>
  <div class="qz-tag">
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
        <h1 class="qz-p-page-title">{{ $t('标签：') }}{{ tag || $t("全部标签") }}</h1>
        <div class="qz-p-page-sub">{{ tag ? (list.length + $t("篇")) : (tags.length + $t("个标签")) }}</div>
      </div>

      <nav v-if="tagList.length" class="qz-p-chips">
        <a
          v-for="t in tagList"
          :key="tName(t)"
          class="qz-p-chip"
          :class="{ on: tName(t) === (tag || '') }"
          :href="'#/blog/tag/' + encodeURIComponent(tName(t))"
        >{{ tName(t) }}</a>
      </nav>

      <div v-if="tag" class="qz-layout">
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
          <p v-else class="qz-state">{{ $t('该标签下暂无文章') }}</p>
        </main>
        <aside class="qz-aside">
          <SideWidget />
        </aside>
      </div>
      <p v-else class="qz-state">{{ $t("点击上方任一标签，即可筛选该标签下的文章。") }}</p>
    </div>
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
const tags = computed(() => ctx.value?.tags || [])
const loading = computed(() => !!ctx.value?.loading)
const error = computed(() => ctx.value?.error || '')
const siteName = computed(() => ctx.value?.site?.siteName || ctx.value?.site?.name || '求智')
const listHref = '#/blog?view=public'

const tag = computed(
  () =>
    ctx.value?.page?.tag ||
    readPageParam(ctx.value, ['tag'], [/\/tag\/([^/?#]+)/, /[?&]tag=([^&#]+)/]) ||
    '',
)
const tagList = computed(() => (tags.value || []).map((t) => ({ name: tName(t) })))
const list = computed(() => {
  if (!tag.value) return []
  const t = String(tag.value).toLowerCase()
  return posts.value.filter((p) =>
    (p.file?.tags || []).some((x) => normTag(x) === t),
  )
})

function normTag(x) {
  const n = typeof x === 'string' ? x : (x && x.name) || ''
  return String(n).toLowerCase()
}
function tName(t) {
  return t && t.name ? t.name : String(t)
}
function pKey(p) {
  return p.path || p.file?.slug || p.token || Math.random()
}
</script>
