<template>
  <div class="dx-post">
    <header class="dx-post-top">
      <a class="dx-post-brand" :href="listHref">{{ siteName }}</a>
      <nav class="dx-post-nav">
        <ThemeSwitch />
        <a :href="listHref">{{ $t('返回列表') }}</a>
      </nav>
    </header>

    <div v-if="loading" class="dx-state">{{ $t('加载中…') }}</div>
    <div v-else-if="error" class="dx-state dx-state-err">{{ error }}</div>
    <div v-else-if="!post.title && !post.html && !post.path" class="dx-state">{{ $t('未找到文章') }}</div>

    <div v-else class="dx-post-wrap">
      <nav class="dx-crumb">
        <a :href="listHref">{{ $t('全部文章') }}</a>
        <span class="dx-crumb-sep">/</span>
        <span v-if="post.cat" class="dx-crumb-mid">{{ post.cat }}</span>
        <span v-if="post.cat" class="dx-crumb-sep">/</span>
        <span class="dx-crumb-now">{{ post.title }}</span>
      </nav>

      <div class="dx-layout">
        <article class="dx-post-main">
          <header class="dx-post-head">
            <span v-if="post.cat" class="dx-post-cat">{{ post.cat }}</span>
            <h1 class="dx-post-title">{{ post.title }}</h1>
            <div class="dx-post-meta">
              <span v-if="post.author">{{ post.author }}</span>
              <span v-if="postDate(post)">{{ postDate(post) }}</span>
            </div>
          </header>

          <div class="dx-article" ref="art" v-html="postHtml"></div>

          <div class="dx-post-foot">
            <button type="button" class="dx-btn" @click="copyLink">
              {{ copied ? $t("链接已复制") : $t("复制分享链接") }}
            </button>
          </div>
        </article>

        <aside class="dx-aside">
          <SideWidget />
        </aside>
      </div>

      <section v-if="related.length" class="dx-related">
        <h2 class="dx-related-h">{{ $t('相关文章') }}</h2>
        <div class="dx-rel-grid">
          <a v-for="p in related" :key="pKey(p)" class="dx-rel-card" :href="postHref(ctx.value, p)">
            <span v-if="postCat(p)" class="dx-rel-cat">{{ postCat(p) }}</span>
            <h4 class="dx-rel-title">{{ postTitle(p) }}</h4>
            <div class="dx-rel-date">{{ postDate(p) }}</div>
          </a>
        </div>
      </section>
    </div>

    <footer class="dx-post-foot-bar">
      <div class="dx-post-foot-in">
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
