<template>
  <div class="vd-root">
    <VerdantHead :site-name="siteName" :blog-url="blogUrl" :slogan="$t('SEARCH · 检索')" />

    <section class="vd-sub-hero">
      <div class="vd-wrap">
        <div class="vd-sect">
          <h2 class="vd-sect-t">{{ searched ? $t("关键词「{keyword}」", { keyword: keyword.trim() }) : $t("站内检索") }}</h2>
          <span class="vd-rule"></span>
          <span class="vd-sect-note">{{ list.length }} {{ $t('篇') }}</span>
        </div>

        <form class="vd-search vd-search-wide" @submit.prevent>
          <label class="vd-search-in">
            <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
              <circle cx="10.5" cy="10.5" r="6.6" fill="none" stroke="currentColor" stroke-width="2" />
              <path d="M15.6 15.6l5 5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
            </svg>
            <input v-model="keyword" class="vd-search-field" type="search" :placeholder="$t('输入标题、摘要或路径关键词…')">
          </label>
          <button v-if="keyword" class="vd-btn vd-btn-g" type="button" @click="keyword = ''">{{ $t('清空') }}</button>
        </form>

        <p class="vd-sub-d">
          {{ searched ? $t("在已加载的公开文章内匹配标题、摘要与路径。") : $t("最新更新 · 输入关键词开始检索") }}
        </p>
      </div>
    </section>

    <section class="vd-body">
      <div class="vd-wrap">
        <div v-if="loading" class="vd-state">{{ $t('正在加载文章…') }}</div>
        <div v-else-if="error" class="vd-state vd-state-err">{{ error }}</div>
        <template v-else>
          <div v-if="list.length" class="vd-grid">
            <VerdantCard v-for="(p, i) in list" :key="p.token || p.path" :post="p" :index="i" />
          </div>
          <div v-else-if="searched" class="vd-state">{{ $t('没有匹配「') }}{{ keyword.trim() }}{{ $t('」的文章，换个关键词试试') }}</div>
          <div v-else class="vd-state">{{ $t('还没有公开文章') }}</div>
        </template>
      </div>
    </section>

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
import { LIST_HREF, postTitle, readPageParam, useBlog } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

const pageKeyword = computed(() =>
  readPageParam(ctx.value, ['q'], [/[?&]q=([^&#]+)/, /\/search\/([^/?#]+)/]),
)
const typed = ref('')
const keyword = computed({
  get: () => typed.value || pageKeyword.value || '',
  set: (v) => {
    typed.value = v
  },
})
const searched = computed(() => !!keyword.value.trim())

/* 检索范围为已加载的公开文章（规范 §4.8 允许的客户端派生） */
const list = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  if (!k) return posts.value.slice(0, 9)
  return posts.value.filter((p) =>
    (postTitle(p) + ' ' + (p.preview || '') + ' ' + (p.path || '')).toLowerCase().includes(k),
  )
})
</script>
