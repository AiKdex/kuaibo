<template>
  <div v-if="cover" class="cv-wrap">
    <img class="cv-img" :src="cover" :alt="alt" loading="lazy" />
  </div>
</template>

<script setup>
import { t } from '@/i18n'
import { computed } from 'vue'

const props = defineProps({
  ctx: { type: Object, default: () => ({}) },
})

// 从列表 preview 中提取第一张图（绝对 URL 或 /media/）
const cover = computed(() => {
  const s = props.ctx?.post?.preview || ''
  const m = /!\[[^\]]*\]\((https?:\/\/[^)\s]+|\/[^)\s]+)\)/.exec(s)
  return m ? m[1] : ''
})

const alt = computed(() => props.ctx?.post?.file?.name || t('封面'))
</script>

<style scoped>
.cv-wrap {
  margin: 8px 0 0;
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid var(--border, #e3e6eb);
  background: var(--surface, #fafbfc);
  max-height: 160px;
}
.cv-img {
  display: block;
  width: 100%;
  height: 160px;
  object-fit: cover;
}
</style>
