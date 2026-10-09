<template>
  <!-- 文章卡片（列表页 / 分类页 / 作者页 / 搜索页共用） -->
  <a class="bt-card" :href="postHref(ctx, post)">
    <PostCover :post="post" round>
      <span class="bt-cover" :class="patClass(index)">
        <span v-if="showCat" class="bt-cover-tag">{{ cat }}</span>
        <b class="bt-cover-no">{{ seqNo(index) }}</b>
      </span>
    </PostCover>
    <span class="bt-card-body">
      <span class="bt-card-cat">
        <span class="bt-chip" :class="catColor(cat)">{{ cat }}</span>
        <span class="bt-card-date">{{ postDate(post) }}</span>
      </span>
      <span class="bt-card-title">{{ postTitle(post) }}</span>
      <span v-if="postSummary(post)" class="bt-card-ex">{{ postSummary(post) }}</span>
      <span class="bt-card-foot">
        <span class="bt-meta">{{ postSize(post) || '—' }}</span>
        <span v-if="daysAgo(post)" class="bt-meta">{{ daysAgo(post) }}</span>
        <span class="bt-read">{{ $t('阅读 →') }}</span>
      </span>
    </span>
  </a>
</template>

<script setup>
import { computed } from 'vue'
import PostCover from '@/components/PostCover.vue'
import {
  catColor,
  catName,
  daysAgo,
  patClass,
  postDate,
  postHref,
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
const cat = computed(() => catName(props.post))
</script>
