<template>
  <!-- sidebar 挂载点示例：文章页侧栏信息卡（协议演示，第三方可替换/删除） -->
  <aside v-if="ctx.title" class="mc-card">
    <h4 class="mc-title">{{ $t('文章信息') }}</h4>
    <dl class="mc-dl">
      <div class="mc-row"><dt>{{ $t('分类') }}</dt><dd>{{ ctx.category || $t("未分类") }}</dd></div>
      <div class="mc-row"><dt>{{ $t('更新时间') }}</dt><dd>{{ fmtTime(ctx.updatedAt) }}</dd></div>
      <div class="mc-row"><dt>{{ $t('大小') }}</dt><dd>{{ fmtSize(ctx.size) }}</dd></div>
      <div class="mc-row"><dt>{{ $t('来源') }}</dt><dd>{{ $t('爱库录知识库') }}</dd></div>
    </dl>
  </aside>
</template>

<script setup>
const props = defineProps({ ctx: { type: Object, default: () => ({}) } })

function fmtTime(ms) {
  if (!ms) return '—'
  const d = new Date(ms)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

function fmtSize(n) {
  if (!n) return '—'
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
  return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB'
}
</script>

<style scoped>
.mc-card {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: var(--radius, 10px);
  background: var(--surface, #fff);
  padding: 14px 16px;
  font-size: 13px;
}

.mc-title {
  margin: 0 0 10px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text, #1f2329);
}

.mc-dl {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.mc-row {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.mc-row dt {
  color: var(--text-3, #8a919f);
  flex-shrink: 0;
}

.mc-row dd {
  margin: 0;
  color: var(--text-2, #4b5563);
  text-align: right;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
