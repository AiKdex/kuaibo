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
        <h1 class="el-page-title">{{ $t('标签：') }}{{ tag || $t("全部标签") }}</h1>
        <div class="el-page-sub">{{ tag ? (list.length + $t("篇")) : (tags.length + $t("个标签")) }}</div>
      </div>

      <nav v-if="tags.length" class="el-chips">
        <a
          v-for="t in tags"
          :key="tName(t)"
          class="el-chip"
          :class="{ on: tName(t) === (tag || '') }"
          :href="'#/blog/tag/' + encodeURIComponent(tName(t))"
        >{{ tName(t) }}</a>
      </nav>

      <template v-if="tag">
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
            <p v-else class="el-state">{{ $t('该标签下暂无文章') }}</p>
          </main>

          <aside class="el-aside">
            <SideWidget />
          </aside>
        </div>
      </template>
      <p v-else class="el-tip">{{ $t("点击上方任一标签，即可筛选该标签下的文章。") }}</p>
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
const tags = computed(() => ctx.value?.tags || [])
const loading = computed(() => !!ctx.value?.loading)
const error = computed(() => ctx.value?.error || '')
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '爱库录')
const listHref = '#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'

const tag = computed(
  () =>
    ctx.value?.page?.tag ||
    readPageParam(ctx.value, ['tag'], [/\/tag\/([^/?#]+)/, /[?&]tag=([^&#]+)/]) ||
    '',
)
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

<style scoped>
.el-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 22px;
}
.el-chip {
  display: inline-block;
  padding: 5px 14px;
  border: 1px solid var(--th-line, #d0d7de);
  border-radius: 999px;
  background: var(--th-card, #fff);
  color: var(--th-ink-2, #3d4a5c);
  font-size: 13px;
  text-decoration: none;
  transition: 0.15s;
}
.el-chip:hover {
  border-color: var(--th-accent, #0d7a6a);
  color: var(--th-accent, #0d7a6a);
}
.el-chip.on {
  background: var(--th-accent, #0d7a6a);
  border-color: var(--th-accent, #0d7a6a);
  color: #fff;
}
.el-tip {
  margin-bottom: 18px;
  font-size: 13px;
  color: var(--th-ink-3, #6b7a8d);
}
</style>
