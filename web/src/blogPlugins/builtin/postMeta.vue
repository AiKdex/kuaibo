<template>
  <!-- 内置示例插件：文章元信息（协议首个实例，第三方照此格式开发） -->
  <div class="bpf-meta">
    <span v-if="ctx.category" class="bpf-tag">{{ ctx.category }}</span>
    <span v-if="ctx.updatedAt" class="bpf-item">{{ $t('更新于') }} {{ fmt(ctx.updatedAt) }}</span>
    <span v-if="ctx.size" class="bpf-item">{{ size(ctx.size) }}</span>
  </div>
</template>

<script setup>
const props = defineProps({
  ctx: { type: Object, default: () => ({}) }
})

function fmt(ts) {
  const d = new Date(ts)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
function size(n) {
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1024 / 1024).toFixed(1) + ' MB'
}
</script>

<style scoped>
.bpf-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--border, #e5e7eb);
  font-size: 12px;
  color: var(--muted, #6b7280);
}
.bpf-tag {
  display: inline-block;
  padding: 2px 10px;
  border-radius: 999px;
  background: var(--accent-soft, rgba(59, 130, 246, 0.1));
  color: var(--accent, #2563eb);
  font-weight: 500;
}
</style>
