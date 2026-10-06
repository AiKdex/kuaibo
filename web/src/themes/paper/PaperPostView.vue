<template>
  <div class="pp-post">
    <header class="pp-post-top">
      <a class="pp-post-brand" :href="listHref">{{ siteName }}</a>
      <nav class="pp-post-nav">
        <ThemeSwitch />
        <a :href="listHref">{{ $t('返回列表') }}</a>
      </nav>
    </header>

    <div v-if="loading" class="pp-state">{{ $t('加载中…') }}</div>
    <div v-else-if="error" class="pp-state pp-state-err">{{ error }}</div>
    <div v-else-if="!post.title && !post.html && !post.path" class="pp-state">{{ $t('未找到文章') }}</div>

    <div v-else class="pp-pwrap">
      <nav class="pp-crumb">
        <a :href="listHref">{{ $t('全部文章') }}</a>
        <span class="pp-crumb-sep">/</span>
        <span v-if="post.cat" class="pp-crumb-mid">{{ post.cat }}</span>
        <span v-if="post.cat" class="pp-crumb-sep">/</span>
        <span class="pp-crumb-now">{{ post.title }}</span>
      </nav>

      <div class="pp-layout">
        <article class="pp-post-main">
          <header class="pp-post-head">
            <span v-if="post.cat" class="pp-post-cat">{{ post.cat }}</span>
            <h1 class="pp-post-title">{{ post.title }}</h1>
            <div class="pp-post-meta">
              <span v-if="post.author">{{ post.author }}</span>
              <span v-if="postDate(post)">{{ postDate(post) }}</span>
            </div>
          </header>

          <div class="pp-article" ref="art" v-html="postHtml"></div>

          <div class="pp-post-foot">
            <button type="button" class="pp-btn" @click="copyLink">
              {{ copied ? $t("链接已复制") : $t("复制分享链接") }}
            </button>
          </div>
        </article>

        <aside class="pp-aside">
          <SideWidget />
        </aside>
      </div>

      <section v-if="related.length" class="pp-related">
        <h2 class="pp-related-h">{{ $t('相关文章') }}</h2>
        <div class="pp-rel-grid">
          <a v-for="p in related" :key="pKey(p)" class="pp-rel-card" :href="postHref(ctx.value, p)">
            <span v-if="postCat(p)" class="pp-rel-cat">{{ postCat(p) }}</span>
            <h4 class="pp-rel-title">{{ postTitle(p) }}</h4>
            <div class="pp-rel-date">{{ postDate(p) }}</div>
          </a>
        </div>
      </section>
    </div>

    <footer class="pp-post-foot-bar">
      <div class="pp-post-foot-in">
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
import { computed, ref } from 'vue'
import { useBlog, postTitle, postCat, postDate, postHref } from './helpers.js'
import './post.css'

const ctx = useBlog()
const post = computed(() => ctx.value?.page?.post || {})
const posts = computed(() => ctx.value?.posts || [])
const loading = computed(() => !!ctx.value?.loading)
const error = computed(() => ctx.value?.error || '')
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '爱库录')
const listHref = '#/blog?view=public'
const rssHref = '/api/v1/blog/feed.xml'

const postHtml = computed(() => post.value?.html || '')

const curKey = computed(() => post.value?.path || post.value?.slug || '')
const related = computed(() => {
  const cur = curKey.value
  const cat = post.value?.cat || post.value?.category || ''
  const same = posts.value.filter((p) => (p.path || p.file?.slug || '') !== cur && postCat(p) === cat)
  const others = posts.value.filter((p) => (p.path || p.file?.slug || '') !== cur && postCat(p) !== cat)
  return [...same, ...others].slice(0, 6)
})

const art = ref(null)
const copied = ref(false)
async function copyLink() {
  const url = typeof window !== 'undefined' ? window.location.href : listHref
  try {
    await navigator.clipboard.writeText(url)
    copied.value = true
    setTimeout(() => (copied.value = false), 2000)
  } catch {
    /* 忽略 */
  }
}

function pKey(p) {
  return p.path || p.file?.slug || p.token || Math.random()
}
</script>
