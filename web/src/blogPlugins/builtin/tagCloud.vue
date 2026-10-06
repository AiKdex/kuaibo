<template>
  <aside v-if="tags.length" class="tc-card">
    <h4 class="tc-title">{{ $t('分类') }}</h4>
    <div class="tc-cloud">
      <a
        v-for="t in tags"
        :key="t.name"
        class="tc-tag"
        :style="{ fontSize: size(t.n) }"
        :href="t.href"
      >{{ t.name }}<span class="tc-n">{{ t.n }}</span></a>
    </div>
  </aside>
</template>

<script setup>
import { computed, inject } from 'vue'

const raw = inject('themeContext')
const tctx = computed(() => raw?.value || raw || { posts: [] })

const tags = computed(() => {
  const m = new Map()
  for (const p of tctx.value.posts || []) {
    const path = p.path || ''
    if (!path.includes('/')) continue
    const name = path.split('/')[0]
    m.set(name, (m.get(name) || 0) + 1)
  }
  return [...m.entries()]
    .map(([name, n]) => ({ name, n, href: `#/blog/cat/${encodeURIComponent(name)}` }))
    .sort((a, b) => b.n - a.n)
    .slice(0, 20)
})

function size(n) {
  if (n >= 8) return '15px'
  if (n >= 4) return '14px'
  return '13px'
}
</script>

<style scoped>
.tc-card {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  background: var(--surface, #fff);
  padding: 14px 16px;
  font-size: 13px;
}
.tc-title { margin: 0 0 10px; font-size: 14px; font-weight: 600; }
.tc-cloud { display: flex; flex-wrap: wrap; gap: 8px; }
.tc-tag {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  padding: 3px 10px;
  border-radius: 999px;
  background: #e6f3f0;
  color: #085548;
  text-decoration: none;
}
.tc-tag:hover { background: #0d7a6a; color: #fff; }
.tc-n { font-size: 11px; opacity: 0.75; }
</style>
