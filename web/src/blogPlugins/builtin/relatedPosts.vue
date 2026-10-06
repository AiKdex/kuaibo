<template>
  <aside v-if="items.length" class="rp-card">
    <h4 class="rp-title">{{ $t('相关推荐') }}</h4>
    <ul class="rp-list">
      <li v-for="(p, i) in items" :key="i">
        <a :href="href(p)">{{ titleOf(p) }}</a>
        <span v-if="cat(p)" class="rp-cat">{{ cat(p) }}</span>
      </li>
    </ul>
  </aside>
</template>

<script setup>
import { computed, inject, ref, watch } from 'vue'
import { publicBlogRelated } from '@/api'

const props = defineProps({
  ctx: { type: Object, default: () => ({}) },
})

const raw = inject('themeContext')
const tctx = computed(() => raw?.value || raw || { posts: [] })

// 语义推荐（后端向量）：非空则优先，否则降级到同分类+时间排序。
const vecItems = ref([])
async function loadRelated(slug) {
  if (!slug) return
  try {
    const r = await publicBlogRelated(slug, 5)
    const arr = (r && r.items) || []
    if (Array.isArray(arr) && arr.length) vecItems.value = arr
  } catch {
    vecItems.value = [] // 失败降级，不影响文章页
  }
}
watch(() => props.ctx?.slug, (s) => loadRelated(s), { immediate: true })

function titleOf(it) {
  // 向量结果已带 title；回退项是主题 post 对象
  return it.title || title(it)
}

const items = computed(() => {
  // 1) 语义推荐命中：直接用（已按相似度降序）
  if (vecItems.value.length) return vecItems.value
  // 2) 降级：同分类优先 + 时间排序（冷启动/无向量）
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
    // 语义推荐项：后端已给 slug/path，博客分享 token 固定为 'blog'
    if (p.slug && !p.token) {
      const qs = new URLSearchParams()
      if (p.path) qs.set('path', p.path)
      qs.set('slug', p.slug)
      return `#/post/blog?${qs.toString()}`
    }
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
