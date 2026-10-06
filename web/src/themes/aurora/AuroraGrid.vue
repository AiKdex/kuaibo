<template>
  <!-- lead：列表页的 bento 节奏（首篇为深色头条卡）；flat：内页统一 4 列 -->
  <div v-if="lead" class="au-grid au-grid-lead">
    <AuroraCard v-for="(p, i) in posts" :key="postKey(p, i)" :post="p" :hero="i === 0" />
  </div>
  <div v-else class="au-grid au-grid-flat">
    <AuroraCard v-for="(p, i) in posts" :key="postKey(p, i)" :post="p" />
  </div>
</template>

<script setup>
/**
 * AuroraGrid —— bento 卡片栅格。
 *
 * lead=true（列表页）：首篇放大成深色头条卡（6 列 × 2 行），其余按 3 / 3 / 4 列节奏铺开；
 * lead=false（分类/作者/归档/搜索页）：统一 4 列等宽卡片，不再强调「头条」。
 * 两种节奏都只由 style.css 的 :nth-child 描述，模板不产生动态类名。
 */
import AuroraCard from './AuroraCard.vue'
import { postKey } from './helpers.js'

defineProps({
  posts: { type: Array, default: () => [] },
  lead: { type: Boolean, default: false },
})
</script>
