<template>
  <div class="ef-post">
    <!-- 面包屑 -->
    <nav class="ef-crumb">
      <a :href="listHref">{{ $t('版块首页') }}</a>
      <span class="sep">/</span>
      <template v-if="post.cat">
        <a :href="listHref">{{ post.cat }}</a>
        <span class="sep">/</span>
      </template>
      <span class="now">{{ post.title }}</span>
    </nav>

    <div class="ef-pl">
      <!-- 主栏 -->
      <div class="ef-main">
        <header class="ef-ph">
          <span v-if="post.cat" class="ef-board-tag">{{ boardIconOf(post.cat) }} {{ post.cat }}</span>
          <h1>{{ post.title }}</h1>
          <div class="ef-meta-row">
            <span class="ef-avatar" :class="boardColorCls">{{ avatarText }}</span>
            <span class="ef-meta-id">
              <span class="ef-m-name">{{ authorName }}</span>
              <span class="ef-m-sub">{{ dateText }} · {{ post.cat || $t('默认版块') }}</span>
            </span>
            <span class="ef-floor">{{ $t('楼主') }}</span>
          </div>
        </header>

        <figure class="ef-cover">
          <span class="ef-cover-ico">{{ boardIconOf(post.cat || $t("文")) }}</span>
          <figcaption>{{ post.cat || $t('综合') }} {{ $t('版块 · 主题帖') }}</figcaption>
        </figure>

        <article ref="art" class="ef-article" v-html="post.html"></article>

        <!-- 标签 + 分享 -->
        <div v-if="post.cat" class="ef-tax">
          <span class="ef-lbl">{{ $t('标签') }}</span>
          <span class="ef-tag">{{ post.cat }}</span>
        </div>
        <div class="ef-share">
          <button class="ef-btn ef-btn-sm" :class="{ liked }" type="button" @click="toggleLike">
            ♥ {{ liked ? $t("已赞") : $t("赞") }}
          </button>
          <button class="ef-btn ef-btn-sm" type="button" @click="copyLink">{{ $t('复制链接') }}</button>
          <button class="ef-btn ef-btn-sm" type="button" @click="shareWeibo">{{ $t('分享到微博') }}</button>
          <button class="ef-btn ef-btn-sm" :class="{ fav: favorited }" type="button" @click="favorited = !favorited">
            {{ favorited ? $t("已收藏") : $t("收藏") }}
          </button>
        </div>

        <!-- 作者卡 -->
        <div v-if="post.author" class="ef-author">
          <span class="ef-avatar ef-avatar-lg" :class="boardColorCls">{{ avatarText }}</span>
          <div>
            <h4>{{ post.author }}</h4>
            <div class="ef-role">{{ $t('本文作者') }}</div>
          </div>
        </div>

        <!-- 上下篇 -->
        <div v-if="prevPost || nextPost" class="ef-pn">
          <a v-if="prevPost" class="prev" :href="postHref(prevPost)">
            <span class="dir">{{ $t('← 上一篇') }}</span>
            <div class="ttl">{{ titleOf(prevPost) }}</div>
          </a>
          <a v-if="nextPost" class="next" :href="postHref(nextPost)">
            <span class="dir">{{ $t('下一篇 →') }}</span>
            <div class="ttl">{{ titleOf(nextPost) }}</div>
          </a>
        </div>

        <!-- 评论：宿主插件槽（真实后端，emforum 化容器） -->
        <section class="ef-comments">
          <BlogPluginSlot mount="post_bottom" :ctx="pluginCtx" />
        </section>
      </div>

      <!-- 侧栏 -->
      <aside class="ef-side">
        <div v-if="tocList.length" class="ef-sb">
          <h3>{{ $t('本文目录') }}</h3>
          <ul class="ef-toc">
            <li v-for="(h, i) in tocList" :key="i" :class="{ sub: h.level === 3 }">
              <button type="button" @click="scrollToHeading(i)">{{ h.text }}</button>
            </li>
          </ul>
        </div>
        <div v-if="hotPosts.length" class="ef-sb">
          <h3>{{ $t('最新主题') }}</h3>
          <ol class="ef-hot">
            <li v-for="p in hotPosts" :key="p.token || p.path">
              <span>
                <a :href="postHref(p)">{{ titleOf(p) }}</a>
                <span class="views">{{ dateOf(p) }}</span>
              </span>
            </li>
          </ol>
        </div>
        <div v-if="cats.length" class="ef-sb">
          <h3>{{ $t('版块标签云') }}</h3>
          <div class="ef-cloud">
            <span v-for="c in cats" :key="c.name" class="ef-cloud-tag">{{ c.icon }} {{ c.name }}</span>
          </div>
        </div>
        <div class="ef-cta">
          <b>{{ $t('订阅本版') }}<br>{{ $t('更新不错过') }}</b>
          <p>{{ $t("新主题发布时通过 RSS 第一时间通知你。") }}</p>
          <a class="ef-btn" :href="rssHref">{{ $t('订阅 RSS') }}</a>
        </div>
      </aside>
    </div>

    <!-- 相关文章 -->
    <section v-if="related.length" class="ef-related">
      <div class="ef-sect">
        <h2>{{ $t('相关主题') }}</h2>
        <span class="ef-rule"></span>
        <span class="ef-note">Related</span>
      </div>
      <div class="ef-rel">
        <a v-for="p in related" :key="p.token || p.path" class="ef-rc" :href="postHref(p)">
          <span v-if="catOf(p)" class="ef-tag">{{ catOf(p) }}</span>
          <h4>{{ titleOf(p) }}</h4>
          <div class="r-date">{{ dateOf(p) }}</div>
        </a>
      </div>
    </section>
    <EmforumAsk />
  </div>
</template>

<script setup>
import { t as i18t } from '@/i18n'
/**
 * EmforumPostView —— EmForum 主题「文章内页」（entries.post）。
 * 宿主 BlogPostView 在主题声明 entries.post 时整页交给本组件渲染，
 * 数据来自 inject('themeContext').page.post（标题/正文 HTML/分类/作者/日期）。
 * 布局与视觉严格对照 EmForum 论坛主题身份（海军蓝×水鸭青×鎏金，论坛帖子版式）。
 */
import { computed, ref } from 'vue'
import BlogPluginSlot from '@/components/BlogPluginSlot.vue'
import EmforumAsk from './EmforumAsk.vue'
import {
  useBlog,
  postTitle,
  postCat,
  postDate,
  boardCounts,
  boardIcon,
  boardColor,
  RSS_HREF,
  LIST_HREF,
} from './helpers.js'

const ctx = useBlog()
const post = computed(() => ctx.value?.page?.post || {})
const posts = computed(() => ctx.value?.posts || [])
const siteName = computed(() => ctx.value?.siteName || '爱库录')
const postUrlFn = computed(() => ctx.value?.postUrl)

const listHref = LIST_HREF
const rssHref = RSS_HREF

const authorName = computed(() => post.value.author || siteName.value)
const avatarText = computed(() => (authorName.value || '?').slice(0, 1))
const boardColorCls = computed(() => boardColor(post.value.cat || '文'))
const dateText = computed(() => {
  const d = postDate({ file: { updated_at: post.value.updatedAt } }, 'ymd')
  return d ? d + i18t('发布') : ''
})
function boardIconOf(name) {
  // 用稳定的版块序号取图标
  const cs = boardCounts(posts.value)
  const hit = cs.find((x) => x.name === (name || ''))
  return hit ? hit.icon : boardIcon(0)
}

/* 当前文章唯一键：博客是单分享，token 均为 "blog"，用 path/slug 区分 */
const curKey = computed(() => post.value.path || post.value.slug || '')
const idx = computed(() => posts.value.findIndex((p) => (p.path || p.file?.slug || '') === curKey.value))
const prevPost = computed(() => (idx.value > 0 ? posts.value[idx.value - 1] : null))
const nextPost = computed(() =>
  idx.value >= 0 && idx.value < posts.value.length - 1 ? posts.value[idx.value + 1] : null,
)

const related = computed(() => {
  const cur = curKey.value
  const sameCat = posts.value.filter((p) => (p.path || p.file?.slug || '') !== cur && postCat(p) === post.value.cat)
  const others = posts.value.filter((p) => (p.path || p.file?.slug || '') !== cur && postCat(p) !== post.value.cat)
  return [...sameCat, ...others].slice(0, 3)
})
const hotPosts = computed(() =>
  posts.value.filter((p) => (p.path || p.file?.slug || '') !== curKey.value).slice(0, 5),
)
const cats = computed(() => boardCounts(posts.value))

function postHref(p) {
  return postUrlFn.value ? postUrlFn.value(p) : '#'
}
function titleOf(p) {
  return postTitle(p)
}
function catOf(p) {
  return postCat(p)
}
function dateOf(p) {
  return postDate(p, 'ymd')
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

/* 互动（本地态，不臆造后端计数） */
const liked = ref(false)
const favorited = ref(false)
function toggleLike() {
  liked.value = !liked.value
}
function copyLink() {
  try {
    if (navigator.clipboard) navigator.clipboard.writeText(location.href)
  } catch (e) {
    /* 忽略 */
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
