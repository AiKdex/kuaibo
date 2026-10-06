<template>
  <div class="brutal-root">
    <BrutalHead :site-name="siteName" :blog-url="blogUrl" :slogan="$t('TAG · 标签')" />

    <div v-if="loading" class="bt-wrap"><div class="bt-state">{{ $t('正在加载文章…') }}</div></div>
    <div v-else-if="error" class="bt-wrap"><div class="bt-state bt-state-err">{{ error }}</div></div>

    <template v-else>
      <section class="bt-posts">
        <div class="bt-wrap">
          <div class="bt-sect">
            <h2>{{ tag || $t("全部") }}</h2>
            <span class="bt-rule"></span>
            <span class="bt-sect-note">{{ list.length }} {{ $t('篇') }}</span>
          </div>

          <p class="bt-lead">{{ $t('标签来自文章元数据：') }}<code>{{ tag || $t("（全部）") }}</code></p>

          <!-- 标签切换 -->
          <div v-if="tagList.length" class="bt-filters">
            <a class="bt-chipbtn" :href="blogUrl">{{ $t('全部') }}</a>
            <a
              v-for="t in tagList"
              :key="t.name"
              class="bt-chipbtn"
              :class="{ on: t.name === (tag || '') }"
              :href="'#/blog/tag/' + encodeURIComponent(t.name)"
              @click="picked = t.name"
            >
              {{ t.name }}<i>{{ t.count }}</i>
            </a>
          </div>

          <div class="bt-bar">
            <span class="bt-bar-info">{{ $t('共') }} {{ list.length }} {{ $t('篇文章') }}</span>
            <a class="bt-btn bt-btn-sm" :href="blogUrl">{{ $t('‹ 返回全部文章') }}</a>
          </div>

          <div v-if="list.length" class="bt-grid">
            <BrutalCard v-for="(p, i) in list" :key="p.token || p.path" :post="p" :index="i" :show-cat="false" />
          </div>
          <div v-else class="bt-state">{{ $t('该标签下暂无文章') }}</div>
        </div>
      </section>

      <section class="bt-sidewrap">
        <div class="bt-wrap">
          <BrutalSide @pick-cat="pickTag" />
        </div>
      </section>
    </template>

    <BrutalFoot :site-name="siteName" :blog-url="blogUrl" />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
import { computed, ref } from 'vue'
import BrutalHead from './BrutalHead.vue'
import BrutalCard from './BrutalCard.vue'
import BrutalSide from './BrutalSide.vue'
import BrutalFoot from './BrutalFoot.vue'
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
