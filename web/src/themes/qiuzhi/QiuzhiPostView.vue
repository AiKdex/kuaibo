<template>
  <div class="qz-post">
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
      <nav class="qz-p-crumb">
        <a :href="listHref">{{ $t('首页') }}</a>
        <span class="qz-p-sep">/</span>
        <span v-if="cat" class="qz-p-cat">{{ cat }}</span>
        <span v-if="cat" class="qz-p-sep">/</span>
        <span class="qz-p-now">{{ title }}</span>
      </nav>

      <div class="qz-layout">
        <main class="qz-p-main">
          <h1 class="qz-p-title">{{ title }}</h1>
          <div class="qz-p-meta">
            <span v-if="author">{{ author }}</span>
            <span v-if="dateText">{{ dateText }}</span>
            <span v-if="cat">{{ cat }}</span>
          </div>
          <article class="qz-article" v-html="postHtml"></article>
          <div v-if="tags.length" class="qz-p-tax">
            <span class="qz-p-tax-lbl">{{ $t('标签') }}</span>
            <a
              v-for="t in tags"
              :key="tName(t)"
              class="qz-p-tag"
              :href="'#/blog/tag/' + encodeURIComponent(tName(t))"
            >{{ tName(t) }}</a>
          </div>
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
import { useBlog, postTitle, postCat, postDate } from './helpers.js'
import './post.css'

const ctx = useBlog()
const post = computed(() => ctx.value?.page?.post || {})
const loading = computed(() => !!ctx.value?.loading)
const error = computed(() => ctx.value?.error || '')
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '求智')
const listHref = '#/blog?view=public'

const title = computed(() => postTitle(post.value) || t('common.unnamed'))
const cat = computed(() => postCat(post.value))
function authorOf(p) {
  const a = p.file?.author
  if (!a) return ''
  return typeof a === 'string' ? a : a.name || a.username || ''
}
const author = computed(() => authorOf(post.value))
const dateText = computed(() => postDate(post.value, 'ymd'))
const tags = computed(() => post.value.file?.tags || [])
const postHtml = computed(() => post.value.html || post.value.content || '')

function tName(t) {
  return t && t.name ? t.name : String(t)
}
</script>
