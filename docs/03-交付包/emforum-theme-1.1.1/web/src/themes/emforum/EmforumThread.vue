<template>
  <!-- 帖子行（列表页 / 分类页 / 作者页 / 搜索页共用） -->
  <a class="ef-thread" :href="postHref(ctx, post)">
    <span class="ef-avatar" :class="boardColor(postCat(post) || '未分类')">{{ avaText }}</span>
    <span class="ef-thread-main">
      <span class="ef-row1">
        <span v-if="showCat && postCat(post)" class="ef-tagline">{{ postCat(post) }}</span>
        <span class="ef-title">{{ postTitle(post) }}</span>
      </span>
      <span class="ef-row2">
        <span>{{ postDate(post) }}</span>
        <span v-if="daysAgo(post)" class="ef-dot">·</span>
        <span>{{ daysAgo(post) }}</span>
        <span v-if="postSize(post)" class="ef-dot">·</span>
        <span>{{ postSize(post) }}</span>
      </span>
      <span v-if="postSummary(post)" class="ef-excerpt">{{ postSummary(post) }}</span>
    </span>
    <span class="ef-thread-side">{{ postDate(post, 'short') }}</span>
  </a>
</template>

<script setup>
import { computed } from 'vue'
import {
  boardColor,
  daysAgo,
  postCat,
  postDate,
  postHref,
  postSize,
  postSummary,
  postTitle,
  useBlog,
} from './helpers.js'

const props = defineProps({
  post: { type: Object, required: true },
  showCat: { type: Boolean, default: true },
})

const ctx = useBlog()
const avaText = computed(() => postTitle(props.post).trim()[0] || '文')
</script>
