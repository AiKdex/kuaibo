<template>
  <!-- 通用文章封面：有封面显示真图（卡片顶部 banner）；无封面则渲染兜底插槽（主题原有占位/装饰） -->
  <div v-if="src" class="post-cover" :class="{ 'post-cover--round': round }">
    <img :src="src" :alt="alt" loading="lazy" @error="failed = true" />
  </div>
  <slot v-else />
</template>

<script setup>
import { computed, ref } from 'vue'
import { postCover } from '@/utils/postCover.js'

const props = defineProps({
  post: { type: Object, required: true },
  alt: { type: String, default: '' },
  round: { type: Boolean, default: false },
})

const failed = ref(false)
const src = computed(() => (failed.value ? '' : postCover(props.post)))
</script>

<style scoped>
.post-cover {
  display: block;
  width: 100%;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  background: #eef0f3;
  flex-shrink: 0;
}
.post-cover--round {
  border-radius: 12px 12px 0 0;
}
.post-cover img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}
</style>
