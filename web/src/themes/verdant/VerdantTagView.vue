<template>
  <div class="vd-root">
    <VerdantHead :site-name="siteName" :blog-url="blogUrl" :slogan="$t('TAG · 标签')" />

    <div v-if="loading" class="vd-wrap"><div class="vd-state">{{ $t('正在加载文章…') }}</div></div>
    <div v-else-if="error" class="vd-wrap"><div class="vd-state vd-state-err">{{ error }}</div></div>

    <template v-else>
      <section class="vd-sub-hero">
        <div class="vd-wrap">
          <div class="vd-sect">
            <h2 class="vd-sect-t">{{ tag || $t("全部") }}</h2>
            <span class="vd-rule"></span>
            <span class="vd-sect-note">{{ list.length }} {{ $t('篇') }}</span>
          </div>
          <p class="vd-sub-d">{{ $t('标签来自文章元数据：') }}<code>{{ tag || $t("（全部）") }}</code></p>
          <div v-if="tagList.length" class="vd-filters">
            <a class="vd-chipbtn" :href="blogUrl">{{ $t('全部') }}</a>
            <a
              v-for="t in tagList"
              :key="t.name"
              class="vd-chipbtn"
              :class="{ on: t.name === (tag || '') }"
              :href="'#/blog/tag/' + encodeURIComponent(t.name)"
              @click="picked = t.name"
            >
              {{ t.name }}<i>{{ t.count }}</i>
            </a>
          </div>
        </div>
      </section>

      <section class="vd-body">
        <div class="vd-wrap">
          <div class="vd-bar">
            <span class="vd-bar-info">{{ $t('共') }} {{ list.length }} {{ $t('篇文章') }}</span>
            <a class="vd-btn vd-btn-g vd-btn-sm" :href="blogUrl">{{ $t('‹ 返回全部文章') }}</a>
          </div>

          <div v-if="list.length" class="vd-grid">
            <VerdantCard v-for="(p, i) in list" :key="p.token || p.path" :post="p" :index="i" :show-cat="false" />
          </div>
          <div v-else class="vd-state">{{ $t('该标签下暂无文章') }}</div>
        </div>
      </section>
    </template>

    <VerdantFoot :site-name="siteName" :blog-url="blogUrl" :posts="posts" />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
import { computed, ref } from 'vue'
import VerdantHead from './VerdantHead.vue'
import VerdantCard from './VerdantCard.vue'
import VerdantFoot from './VerdantFoot.vue'
import './style.css'
import { LIST_HREF, readPageParam, useBlog } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

const tagList = computed(() => {
  const map = new Map()
  for (const p of posts.value) {
    for (const t of (p.file?.tags || [])) {
      const name = typeof t === 'string' ? t : (t && t.name)
      if (!name) continue
      map.set(name, (map.get(name) || 0) + 1)
    }
  }
  return [...map.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count || String(a.name).localeCompare(String(b.name), 'zh'))
})

/* 当前标签：优先 ctx.page.tag（系统注入），回退解析 hash / ?tag= */
const pageParam = computed(() =>
  readPageParam(ctx.value, ['tag'], [/\/tag\/([^/?#]+)/, /[?&]tag=([^&#]+)/]),
)
const picked = ref('')
const tag = computed(() => picked.value || pageParam.value)

const list = computed(() => {
  if (!tag.value) return posts.value
  return posts.value.filter((p) =>
    (p.file?.tags || []).some((t) => (typeof t === 'string' ? t : (t && t.name)) === tag.value)
  )
})

function pickTag(name) {
  picked.value = name
}
</script>
