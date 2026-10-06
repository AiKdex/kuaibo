<template>
  <a class="au-card" :href="href">
    <div class="au-card-top">
      <span v-if="hasCat" class="au-card-cat">{{ cat }}</span>
      <span v-if="hero" class="au-card-badge">{{ $t('头条') }}</span>
    </div>
    <h2 class="au-card-title">{{ title }}</h2>
    <p class="au-card-ex">{{ excerpt }}</p>
    <div class="au-card-meta">
      <span class="au-card-date">{{ date }}</span>
      <span class="au-card-arrow" aria-hidden="true">→</span>
    </div>
  </a>
</template>

<script setup>
/**
 * AuroraCard —— bento 栅格里的单张文章卡。
 *
 * 卡片在栅格中的跨度（6/3/4 列）与配色由 style.css 的 :nth-child 规则决定，
 * 因此模板里只有 .au-card 一个静态类 —— 无动态拼类、无死 CSS、无悬空类。
 * 文案只用可证事实：位置用「头条」（就是列表第一条），日期用 file.updated_at。
 */
import { computed } from 'vue'
import { useBlog, postTitle, catName, postCat, postDate, postExcerpt, postHref } from './helpers.js'

const props = defineProps({
  post: { type: Object, required: true },
  hero: { type: Boolean, default: false },
})

const ctx = useBlog()
const title = computed(() => postTitle(props.post))
const cat = computed(() => catName(props.post))
const hasCat = computed(() => !!postCat(props.post))
const date = computed(() => postDate(props.post, 'ymd'))
const excerpt = computed(() => postExcerpt(props.post, props.hero ? 160 : 78))
const href = computed(() => postHref(ctx.value, props.post))
</script>
