<template>
  <aside v-if="items.length" class="rp-card">
    <h4 class="rp-title">相关推荐</h4>
    <ul class="rp-list">
      <li v-for="(p, i) in items" :key="i">
        <a :href="href(p)">{{ title(p) }}</a>
        <span v-if="cat(p)" class="rp-cat">{{ cat(p) }}</span>
      </li>
    </ul>
  </aside>
</template>

<script setup>
import { computed, inject } from 'vue'

const props = defineProps({
  ctx: { type: Object, default: () => ({}) },
})

const raw = inject('themeContext')
const tctx = computed(() => raw?.value || raw || { posts: [] })

const items = computed(() => {
  const all = tctx.value.posts || []
  if (!all.length) return []
  const curPath = props.ctx?.path || ''
  const curTitle = props.ctx?.title || ''
  const curCat = curPath.includes('/') ? curPath.split('/')[0] : ''
  const rest = all.filter((p) => {
    const path = p.path || ''
    if (path && path === curPath) return false
    if (title(p) === curTitle && curTitle) return false
    return true
  })
  // 同分类优先，再按更新时间
  const same = rest.filter((p) => cat(p) && cat(p) === curCat)
  const other = rest.filter((p) => !same.includes(p))
  const byTime = (a, b) => (b.file?.updated_at || 0) - (a.file?.updated_at || 0)
  return [...same.sort(byTime), ...other.sort(byTime)].slice(0, 5)
})

function title(p) {
  const n = p.file?.name || p.name || p.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || '未命名'
}
function cat(p) {
  const path = p.path || ''
  return path.includes('/') ? path.split('/')[0] : ''
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
.rp-card {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  background: var(--surface, #fff);
  padding: 14px 16px;
  font-size: 13px;
}
.rp-title {
  margin: 0 0 10px;
  font-size: 14px;
  font-weight: 600;
}
.rp-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.rp-list a {
  color: var(--text, #1f2329);
  text-decoration: none;
}
.rp-list a:hover { color: #0d7a6a; }
.rp-cat {
  display: block;
  font-size: 11px;
  color: var(--text-3, #8a919f);
  margin-top: 2px;
}
</style>
