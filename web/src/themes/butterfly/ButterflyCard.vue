<template>
  <a class="bf-item" :class="{ 'is-rev': reversed }" :href="href">
    <PostCover :post="post" round>
      <!-- 契约不含封面图（无 cover 字段）→ 用纯 CSS 渐变 + 纹样做版面块，色相按分类稳定派生，只作装饰 -->
      <span class="bf-item-art" :class="tone" aria-hidden="true">
        <span class="bf-item-art-pat"></span>
        <span class="bf-item-art-ch">{{ ch }}</span>
        <span class="bf-item-art-read"><span>{{ $t('阅读') }}</span></span>
      </span>
    </PostCover>

    <span class="bf-item-info">
      <span class="bf-item-cats">
        <span class="bf-item-cat">{{ catName(post) }}</span>
        <span v-if="daysAgo(post) === $t('今天')" class="bf-item-new">NEW</span>
      </span>

      <span class="bf-item-title">{{ postTitle(post) }}</span>

      <span class="bf-item-ex">{{ postSummary(post, 78) || $t("点击阅读全文") }}</span>

      <span class="bf-item-foot">
        <span class="bf-item-meta">
          <span class="bf-item-meta-i">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><rect x="3.5" y="5" width="17" height="15.5" rx="2.4" /><path d="M8 3v4M16 3v4M3.5 10h17" /></svg>
            {{ postDate(post) || $t("未标注日期") }}
          </span>
          <span v-if="postSize(post)" class="bf-item-meta-i">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M5 3.5h9l5 5V20.5H5z" /><path d="M14 3.5v5h5" /></svg>
            {{ postSize(post) }}
          </span>
          <span v-if="daysAgo(post)" class="bf-item-meta-i">{{ daysAgo(post) }}</span>
        </span>
        <span class="bf-item-more">{{ $t('阅读全文') }}<i>›</i></span>
      </span>
    </span>
  </a>
</template>

<script setup>
import { computed } from 'vue'
import PostCover from '@/components/PostCover.vue'
import {
  catName, catTone, daysAgo, initials, postDate, postHref,
  postSize, postSummary, postTitle, useBlog,
} from './helpers.js'

const props = defineProps({
  post: { type: Object, required: true },
  index: { type: Number, default: 0 },
  /** Butterfly 的卡片左右交替：偶数行封面在右 */
  reversed: { type: Boolean, default: false },
})

const ctx = useBlog()
const href = computed(() => postHref(ctx.value, props.post))
const tone = computed(() => catTone(catName(props.post)))
const ch = computed(() => initials(catName(props.post), 1))
</script>
