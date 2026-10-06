<template>
  <div class="vd-post">
    <!-- 面包屑：站内跳转一律用 postUrl / 相对路径，不手拼 hash -->
    <nav class="vd-crumb">
      <a :href="listHref">{{ $t('首页') }}</a>
      <span class="vd-crumb-sep">/</span>
      <template v-if="post.cat">
        <a :href="listHref">{{ post.cat }}</a>
        <span class="vd-crumb-sep">/</span>
      </template>
      <span class="vd-crumb-now">{{ post.title }}</span>
    </nav>

    <header class="vd-post-head">
      <span v-if="post.cat" class="vd-post-tag">{{ post.cat }}</span>
      <h1 class="vd-post-h1">{{ post.title }}</h1>
      <div class="vd-post-meta">
        <span class="vd-ava">{{ avatarText }}</span>
        <span class="vd-post-meta-tx">
          <span class="vd-post-author">{{ authorName }}</span>
          <span class="vd-post-date">{{ dateText }}</span>
        </span>
      </div>
    </header>

    <div class="vd-post-grid">
      <div class="vd-post-main">
        <article class="vd-article" v-html="post.html"></article>

        <div class="vd-post-share">
          <button class="vd-post-sbtn" type="button" @click="copyLink">{{ $t('复制链接') }}</button>
          <button class="vd-post-sbtn" type="button" @click="shareWeibo">{{ $t('分享到微博') }}</button>
          <a class="vd-post-sbtn" :href="ssrHref" target="_blank" rel="noopener">{{ $t('打开静态页') }}</a>
        </div>

        <div v-if="post.author" class="vd-post-authorcard">
          <span class="vd-ava vd-ava-lg">{{ avatarText }}</span>
          <div>
            <div class="vd-post-authorcard-n">{{ post.author }}</div>
            <div class="vd-post-authorcard-s">{{ $t('本文作者') }}</div>
          </div>
        </div>

        <!-- 上下篇 -->
        <div v-if="prevPost || nextPost" class="vd-post-pn">
          <a v-if="prevPost" class="vd-pn-card" :href="postHref(prevPost)">
            <span class="vd-pn-dir">{{ $t('← 上一篇') }}</span>
            <span class="vd-pn-ttl">{{ postTitle(prevPost) }}</span>
          </a>
          <a v-if="nextPost" class="vd-pn-card" :href="postHref(nextPost)">
            <span class="vd-pn-dir">{{ $t('下一篇 →') }}</span>
            <span class="vd-pn-ttl">{{ postTitle(nextPost) }}</span>
          </a>
        </div>

        <!-- 评论：宿主插件槽（真实后端） -->
        <section class="vd-post-comments">
          <BlogPluginSlot mount="post_bottom" :ctx="pluginCtx" />
        </section>
      </div>

      <aside class="vd-post-aside">
        <section v-if="tocList.length" class="vd-post-sb">
          <h3 class="vd-post-sb-h">{{ $t('本篇目录') }}</h3>
          <ul class="vd-toc">
            <li v-for="(h, i) in tocList" :key="i" class="vd-toc-item" :class="{ sub: h.level === 3 }">
              <!-- 目录跳转用 button + scrollIntoView（规范 §3.2） -->
              <button class="vd-toc-btn" type="button" @click="scrollToHeading(i)">{{ h.text }}</button>
            </li>
          </ul>
        </section>

        <section v-if="sameCatPosts.length" class="vd-post-sb">
          <h3 class="vd-post-sb-h">{{ $t('同类文章') }}</h3>
          <ul class="vd-toc">
            <li v-for="p in sameCatPosts" :key="p.token || p.path" class="vd-toc-item">
              <a class="vd-toc-btn" :href="postHref(p)">{{ postTitle(p) }}</a>
            </li>
          </ul>
        </section>

        <section class="vd-post-cta">
          <h3 class="vd-post-cta-h">{{ $t('收藏这个站点') }}</h3>
          <p class="vd-post-cta-p">{{ $t("用 RSS 订阅，新文章发出时第一个收到。") }}</p>
          <a class="vd-post-cta-btn" :href="rssHref">{{ $t('订阅 RSS') }}</a>
        </section>
      </aside>
    </div>

    <!-- 接着读（按同分类优先 + 其余补齐，纯时间序，不表达热度） -->
    <section v-if="related.length" class="vd-post-rel">
      <div class="vd-sect">
        <h2 class="vd-sect-t">{{ $t('接着读') }}</h2>
        <span class="vd-rule"></span>
        <span class="vd-sect-note">{{ $t('按系统顺序取其余文章') }}</span>
      </div>
      <div class="vd-rel-grid">
        <a v-for="p in related" :key="p.token || p.path" class="vd-rel-card" :href="postHref(p)">
          <span v-if="catName(p)" class="vd-rel-cat">{{ catName(p) }}</span>
          <span class="vd-rel-ttl">{{ postTitle(p) }}</span>
          <span class="vd-rel-date">{{ postDate(p) }}</span>
        </a>
      </div>
    </section>
  </div>
    <AskWidget />
</template>

<script setup>
import { t as i18t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
/**
 * VerdantPostView —— 青野主题「文章内页」（entries.post）。
 * 宿主 BlogPostView 在主题声明 entries.post 时整页交给本组件渲染，
 * 数据来自 inject('themeContext').page.post（标题 / 正文 HTML / 分类 / 作者 / 日期）。
 * 版式对照 企业主题设计稿/05-verdant-green.html 的文章内页。
 * ⚠️ 文章专属样式全部在 post.css（构建插件加 .th-verdant.th-view-post 作用域，零泄漏）。
 */
import { computed, ref } from 'vue'
import BlogPluginSlot from '@/components/BlogPluginSlot.vue'
import {
  LIST_HREF,
  RSS_HREF,
  catName,
  postDate,
  postTitle,
  useBlog,
} from './helpers.js'

const ctx = useBlog()
const post = computed(() => ctx.value?.page?.post || {})
const posts = computed(() => ctx.value?.posts || [])
const siteName = computed(() => ctx.value?.siteName || '爱库录')
const postUrlFn = computed(() => ctx.value?.postUrl)
const ssrUrlFn = computed(() => ctx.value?.postSsrUrl)

const listHref = LIST_HREF
const rssHref = RSS_HREF

const authorName = computed(() => post.value.author || siteName.value)
const avatarText = computed(() => (authorName.value || '?').slice(0, 1))
const dateText = computed(() => {
  const d = postDate({ file: { updated_at: post.value.updatedAt } }, 'ymd')
  return d ? d + i18t('发布') : ''
})
/* 分享 / SEO 场景用 SSR 静态页地址（规范 §2.3） */
const ssrHref = computed(() => (ssrUrlFn.value ? ssrUrlFn.value(post.value) : '/blog'))

/* 当前文章唯一键：博客为单分享，token 可能相同，用 path/slug 区分 */
const curKey = computed(() => post.value.path || post.value.slug || '')
const idx = computed(() =>
  posts.value.findIndex((p) => (p.path || p.file?.slug || '') === curKey.value),
)
const prevPost = computed(() => (idx.value > 0 ? posts.value[idx.value - 1] : null))
const nextPost = computed(() =>
  idx.value >= 0 && idx.value < posts.value.length - 1 ? posts.value[idx.value + 1] : null,
)

const sameCatPosts = computed(() =>
  posts.value
    .filter((p) => (p.path || p.file?.slug || '') !== curKey.value && catName(p) === post.value.cat)
    .slice(0, 5),
)

const related = computed(() => {
  const cur = curKey.value
  const rest = posts.value.filter((p) => (p.path || p.file?.slug || '') !== cur)
  const sameCat = rest.filter((p) => catName(p) === post.value.cat)
  const others = rest.filter((p) => catName(p) !== post.value.cat)
  return [...sameCat, ...others].slice(0, 3)
})

function postHref(p) {
  return postUrlFn.value ? postUrlFn.value(p) : '#'
}

/* 目录：从正文 HTML 解析 h2/h3，点击平滑滚动（不碰 location.hash） */
const art = ref(null)
const tocList = computed(() => {
  const html = post.value.html || ''
  const re = /<h([23])[^>]*>([\s\S]*?)<\/h\1>/gi
  const out = []
  let m
  while ((m = re.exec(html))) {
    const text = m[2].replace(/<[^>]+>/g, '').trim()
    if (text) out.push({ level: +m[1], text })
  }
  return out
})
function scrollToHeading(i) {
  const els = art.value?.querySelectorAll('h2, h3')
  if (els && els[i]) els[i].scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function copyLink() {
  try {
    if (navigator.clipboard) navigator.clipboard.writeText(location.href)
  } catch (e) {
    /* 剪贴板不可用时静默忽略 */
  }
}
function shareWeibo() {
  const u = encodeURIComponent(location.href)
  const t = encodeURIComponent(post.value.title || '')
  window.open(`http://service.weibo.com/share/share.php?url=${u}&title=${t}`, '_blank')
}

/* 评论插件上下文（与宿主一致） */
const pluginCtx = computed(() => ({
  title: post.value.title,
  path: post.value.path,
  slug: post.value.slug,
  category: post.value.cat,
  updatedAt: post.value.updatedAt,
  size: post.value.size,
  token: post.value.token,
}))
</script>
