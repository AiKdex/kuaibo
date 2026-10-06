<template>
  <aside v-if="groups.length" class="ar-card">
    <h4 class="ar-title">{{ $t('归档') }}</h4>
    <ul class="ar-list">
      <li v-for="g in groups" :key="g.key">
        <span class="ar-month">{{ g.label }}</span>
        <span class="ar-n">{{ g.items.length }}</span>
        <ul class="ar-posts">
          <li v-for="(p, i) in g.items" :key="i">
            <a :href="href(p)">{{ title(p) }}</a>
          </li>
        </ul>
      </li>
    </ul>
  </aside>
</template>

<script setup>
import { computed, inject } from 'vue'

const raw = inject('themeContext')
const tctx = computed(() => raw?.value || raw || { posts: [] })

const groups = computed(() => {
  const map = new Map()
  for (const p of tctx.value.posts || []) {
    const ms = p.file?.updated_at
    if (!ms) continue
    const d = new Date(ms > 1e12 ? ms : ms * 1000)
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
    if (!map.has(key)) map.set(key, [])
    map.get(key).push(p)
  }
  return [...map.entries()]
    .sort((a, b) => (a[0] < b[0] ? 1 : -1))
    .slice(0, 12)
    .map(([key, items]) => ({ key, label: key, items }))
})

function title(p) {
  const n = p.file?.name || p.name || p.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || '未命名'
}
function href(p) {
  try {
    return tctx.value.postUrl ? tctx.value.postUrl(p) : `#/p/${p.token}`
  } catch {
    return '#/blog?view=public'
  }
}
</script>

<style scoped>
.ar-card {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  background: var(--surface, #fff);
  padding: 14px 16px;
  font-size: 13px;
}
.ar-title { margin: 0 0 10px; font-size: 14px; font-weight: 600; }
.ar-list { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 12px; }
.ar-month { font-weight: 600; font-family: ui-monospace, Consolas, monospace; font-size: 12px; }
.ar-n { margin-left: 6px; color: var(--text-3, #8a919f); font-size: 11px; }
.ar-posts { margin: 6px 0 0; padding: 0 0 0 12px; list-style: none; display: flex; flex-direction: column; gap: 4px; }
.ar-posts a { color: var(--text-2, #4b5563); text-decoration: none; }
.ar-posts a:hover { color: #0d7a6a; }
</style>
