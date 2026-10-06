<template>
  <!-- 卡片流：lead=true 时首篇带渐变头图（就是列表第一条，用位置描述不用质量词） -->
  <div class="zc-list">
    <a v-for="(p, i) in posts" :key="postKey(p, i)" class="zc-item" :href="postHref(ctx, p)">
      <div v-if="lead && i === 0" class="zc-cover">
        <span class="zc-cover-cat">{{ catName(p) }}</span>
      </div>
      <div class="zc-item-body">
        <h2 class="zc-item-title">{{ postTitle(p) }}</h2>
        <p v-if="excerptOf(p)" class="zc-item-ex">{{ excerptOf(p) }}</p>
        <div class="zc-item-meta">
          <span v-if="postCat(p)" class="zc-item-cat">{{ postCat(p) }}</span>
          <span v-if="dateOf(p)" class="zc-item-date">{{ dateOf(p) }}</span>
          <span v-if="sizeOf(p)" class="zc-item-size">{{ sizeOf(p) }}</span>
          <span class="zc-item-arrow" aria-hidden="true">→</span>
        </div>
      </div>
    </a>
  </div>
</template>

<script setup>
/**
 * ZcList —— 社区资讯式文章卡片流。
 *
 * lead=true（列表页）：首条展开成带渐变头图的特写卡，其余为紧凑行卡；
 * lead=false（分类/作者/归档/搜索页）：统一行卡，不做特写。
 * 特写由「列表第一条」这一位置事实决定（hero prop），不使用任何质量评价词。
 */
import { useBlog, postTitle, catName, postCat, postDate, postExcerpt, postSize, postHref, postKey } from './helpers.js'

defineProps({
  posts: { type: Array, default: () => [] },
  lead: { type: Boolean, default: false },
})

const ctx = useBlog()

function excerptOf(p) {
  return postExcerpt(p, 88)
}
function dateOf(p) {
  return postDate(p, 'ymd')
}
function sizeOf(p) {
  return postSize(p)
}
</script>
