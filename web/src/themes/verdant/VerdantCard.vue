<template>
  <a class="vd-card" :href="href">
    <!-- 无封面图字段（契约不含 cover）→ 用纯 CSS 有机图形做版面占位，色相按分类稳定派生 -->
    <span class="vd-card-art" :class="tone" aria-hidden="true">
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
import { computed } from 'vue'
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
</script>
