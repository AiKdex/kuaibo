<template>
  <div class="brutal-root">
    <BrutalHead :site-name="siteName" :blog-url="blogUrl" :slogan="$t('SEARCH · 搜索')" />

    <div v-if="loading" class="bt-wrap"><div class="bt-state">{{ $t('正在加载文章…') }}</div></div>
    <div v-else-if="error" class="bt-wrap"><div class="bt-state bt-state-err">{{ error }}</div></div>

    <template v-else>
      <section class="bt-posts">
        <div class="bt-wrap">
          <div class="bt-sect">
            <h2>{{ $t('搜索') }}</h2>
            <span class="bt-rule"></span>
            <span class="bt-sect-note">{{ keyword ? hitCount : $t("输入关键词") }}</span>
          </div>

          <form class="bt-search bt-search-wide" @submit.prevent>
            <label class="bt-search-in">
              <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
                <circle cx="10.5" cy="10.5" r="7" fill="none" stroke="currentColor" stroke-width="3" />
                <path d="M16 16l5.5 5.5" fill="none" stroke="currentColor" stroke-width="3" />
              </svg>
              <input v-model="keyword" type="search" :placeholder="$t('搜索标题、摘要或路径…')" autofocus>
            </label>
            <button class="bt-btn bt-btn-accent" type="button" @click="keyword = ''">{{ $t('清空') }}</button>
          </form>

          <p class="bt-lead">{{ $t("在标题、摘要与文件路径中做不区分大小写的匹配，结果保持系统排序（置顶优先 + 时间倒序）。") }}
          </p>

          <div v-if="!keyword.trim()" class="bt-state">{{ $t('请输入关键词开始搜索') }}</div>

          <template v-else>
            <div v-if="list.length" class="bt-grid">
              <BrutalCard v-for="(p, i) in list" :key="p.token || p.path" :post="p" :index="i" />
            </div>
            <div v-else class="bt-state">{{ $t('没有找到匹配「') }}{{ keyword.trim() }}{{ $t('」的文章') }}</div>
          </template>
        </div>
      </section>

      <section class="bt-sidewrap">
        <div class="bt-wrap">
          <BrutalSide />
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
import { LIST_HREF, postSummary, postTitle, readPageParam, useBlog } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

/* 初始关键词：优先 ctx.page.q，回退解析 hash / ?q= */
const initial = readPageParam(ctx.value, ['q', 'keyword', 'search'], [/[?&]q=([^&#]+)/, /\/search\/([^/?#]+)/])
const keyword = ref(initial)

const list = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  if (!k) return []
  return posts.value.filter((p) =>
    (postTitle(p) + ' ' + postSummary(p, 400) + ' ' + (p.path || '')).toLowerCase().includes(k),
  )
})
const hitCount = computed(() => `${list.value.length} 条结果`)
</script>
