<template>
  <!-- list_item 挂载点示例：文章列表项内显示分类徽章（协议演示，第三方可替换/删除） -->
  <span v-if="ctx.post" class="li-badges">
    <span v-if="cat" class="li-badge" :title="$t('分类：') + cat">{{ cat }}</span>
    <span class="li-badge src" :title="$t('来源：爱库录知识库')">{{ $t('知识库') }}</span>
  </span>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({ ctx: { type: Object, default: () => ({}) } })
const cat = computed(() => {
  const p = props.ctx.post?.path
  return p && p.includes('/') ? p.split('/')[0] : ''
})
</script>

<style scoped>
.li-badges {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-left: 8px;
  vertical-align: 2px;
}

.li-badge {
  font-size: 11px;
  line-height: 1;
  padding: 3px 8px;
  border-radius: 10px;
  color: var(--primary, #4c7df0);
  background: color-mix(in srgb, var(--primary, #4c7df0) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--primary, #4c7df0) 30%, transparent);
}

.li-badge.src {
  color: var(--text-3, #8a919f);
  background: transparent;
  border-color: var(--border, #e3e6eb);
}
</style>
