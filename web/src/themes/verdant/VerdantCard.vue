<template>
  <a class="vd-card" :href="href">
    <!-- 封面图优先；无封面则保留青野有机图形占位 -->
    <span v-if="coverImg" class="vd-card-cover" :class="tone" aria-hidden="true">
      <img :src="coverImg" :alt="postTitle(post)" loading="lazy" @error="coverFailed = true" />
      <span class="vd-card-no">{{ seqNo(index) }}</span>
    </span>
    <span v-else class="vd-card-art" :class="tone" aria-hidden="true">
      <span class="vd-card-leaf"></span>
      <span class="vd-card-no">{{ seqNo(index) }}</span>
    </span>

    <span class="vd-card-body">
      <span v-if="showCat" class="vd-card-cat">{{ catName(post) }}</span>
      <span class="vd-card-title">{{ postTitle(post) }}</span>
      <span class="vd-card-ex">{{ postSummary(post, 76) || $t("点击阅读全文") }}</span>
      <span class="vd-card-meta">
        <span class="vd-card-date">{{ postDate(post) }}</span>
        <span v-if="postSize(post)" class="vd-card-read">{{ postSize(post) }}</span>
        <span v-if="daysAgo(post)" class="vd-card-read">{{ daysAgo(post) }}</span>
      </span>
    </span>

    <span class="vd-card-more">{{ $t('阅读全文 →') }}</span>
  </a>
</template>

<script setup>
import { computed, ref } from 'vue'
import {
  catName,
  catTone,
  daysAgo,
  postDate,
  postSize,
  postSummary,
  postTitle,
  seqNo,
  useBlog,
} from './helpers.js'

const props = defineProps({
  post: { type: Object, required: true },
  index: { type: Number, default: 0 },
  showCat: { type: Boolean, default: true },
})

const ctx = useBlog()
const href = computed(() => (ctx.value.postUrl ? ctx.value.postUrl(props.post) : '#'))
const tone = computed(() => catTone(catName(props.post)))

// 封面图：优先文章的封面字段（编辑器设置/上传），回退从摘要首图提取；
// 都没有则计算属性返回空串 → 模板回退到青野有机图形占位。
// 加载失败（如已删除的媒体）标记后回退占位，避免裂图。
const coverFailed = ref(false)
const coverImg = computed(() => {
  if (coverFailed.value) return ''
  const p = props.post || {}
  const c = p.file?.cover
  if (typeof c === 'string' && c.trim()) return c.trim()
  const s = p.preview || ''
  const m = /!\[[^\]]*\]\((https?:\/\/[^)\s]+|\/[^)\s]+)\)/.exec(s)
  return m ? m[1] : ''
})
</script>

<style scoped>
.vd-card-cover {
  position: relative;
  display: block;
  height: 132px;
  overflow: hidden;
  background: var(--th-leaf, #eef5e9);
}
.vd-card-cover img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}
/* 封面图上叠加序号，保持版面一致性，并加阴影保证可读 */
.vd-card-cover .vd-card-no {
  color: #fff;
  text-shadow: 0 1px 8px rgba(0, 0, 0, 0.45);
}
</style>
