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
    <div v-else-if="!post.title && !post.html && !post.path" class="el-state">{{ $t('未找到文章') }}</div>

    <div v-else class="el-pwrap">
      <nav class="el-crumb">
        <a :href="listHref">{{ $t('全部文章') }}</a>
        <span class="el-crumb-sep">/</span>
        <span v-if="post.cat" class="el-crumb-mid">{{ post.cat }}</span>
        <span v-if="post.cat" class="el-crumb-sep">/</span>
        <span class="el-crumb-now">{{ post.title }}</span>
      </nav>

      <div class="el-layout">
        <article class="el-post-main">
          <header class="el-post-head">
            <span v-if="post.cat" class="el-post-cat">{{ post.cat }}</span>
            <h1 class="el-post-title">{{ post.title }}</h1>
            <div class="el-post-meta">
              <span v-if="post.author">{{ post.author }}</span>
              <span v-if="postDate(post)">{{ postDate(post) }}</span>
            </div>
          </header>

          <div class="el-article" ref="art" v-html="postHtml"></div>

          <div class="el-post-foot">
            <button type="button" class="el-btn" @click="copyLink">
              {{ copied ? $t("链接已复制") : $t("复制分享链接") }}
            </button>
          </div>
        </article>

        <aside class="el-aside">
          <SideWidget />
        </aside>
      </div>

      <section v-if="related.length" class="el-related">
        <h2 class="el-related-h">{{ $t('相关文章') }}</h2>
        <div class="el-rel-grid">
          <a v-for="p in related" :key="pKey(p)" class="el-rel-card" :href="postHref(ctx.value, p)">
            <span v-if="postCat(p)" class="el-rel-cat">{{ postCat(p) }}</span>
            <h4 class="el-rel-title">{{ postTitle(p) }}</h4>
            <div class="el-rel-date">{{ postDate(p) }}</div>
          </a>
        </div>
      </section>
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
