<template>
  <div class="au-root">
    <AuroraHead />

    <main class="au-main">
      <section class="au-hero au-hero-sm">
        <p class="au-kicker">{{ $t('标签') }}</p>
        <div class="au-hero-name">{{ tag || $t("全部文章") }}</div>
        <p class="au-hero-desc">{{ $t('按文章标签聚合，共') }} {{ list.length }} {{ $t('篇公开文章。') }}</p>
      </section>

      <nav v-if="tagList.length" class="au-tagcloud">
        <a
          v-for="t in tagList"
          :key="t.name"
          class="au-tagchip"
          :class="{ on: t.name === (tag || '') }"
          :href="'#/blog/tag/' + encodeURIComponent(t.name)"
        >{{ t.name }}<i v-if="t.count">{{ t.count }}</i></a>
      </nav>

      <AuroraGrid v-if="list.length" :posts="list" />

      <div v-else class="au-state">
        <div class="au-state-t">
          {{ tag ? $t("该标签下暂无公开文章") : $t("尚未选择标签") }}
        </div>
        <div class="au-state-d">{{ $t("标签 = 文章元数据里的标签字段，点击上方标签可筛选。") }}</div>
      </div>
    </main>

    <AuroraFoot />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
/**
 * AuroraTagView —— 标签页（entries.tag）。
 * 数据：ctx.page.tag（系统侧路由补齐后注入）+ ctx.posts 按 file.tags 过滤；
 * 系统未注入时回退解析 hash（只读），保证当前版本可预览。
 */
import { computed } from 'vue'
import AuroraHead from './AuroraHead.vue'
import AuroraFoot from './AuroraFoot.vue'
import AuroraGrid from './AuroraGrid.vue'
import './style.css'
import { useBlog, readPageParam } from './helpers.js'

const ctx = useBlog()
const tag = computed(() =>
  readPageParam(ctx.value, ['tag'], [/\/tag\/([^/?#]+)/, /[?&]tag=([^&#]+)/])
)
const tagList = computed(() => {
  const map = new Map()
  for (const p of (ctx.value.posts || [])) {
    for (const t of (p.file?.tags || [])) {
      const name = typeof t === 'string' ? t : (t && t.name)
      if (!name) continue
      map.set(name, (map.get(name) || 0) + 1)
    }
  }
  return [...map.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count || String(a.name).localeCompare(String(b.name), 'zh'))
})
const list = computed(() => {
  if (!tag.value) return ctx.value.posts || []
  return (ctx.value.posts || []).filter((p) =>
    (p.file?.tags || []).some((t) => (typeof t === 'string' ? t : (t && t.name)) === tag.value)
  )
})
</script>

<style scoped>
.au-tagcloud{display:flex;flex-wrap:wrap;gap:10px;margin:0 0 28px}
.au-tagchip{
  display:inline-flex;align-items:center;gap:6px;
  padding:6px 16px;border-radius:999px;font-size:13px;font-weight:600;
  text-decoration:none;color:var(--th-ink-2,#64748b);
  background:var(--th-paper,#f1f5f9);border:1px solid var(--th-line,#e2e8f0);
  transition:.2s;
}
.au-tagchip:hover{color:var(--th-accent,#0a84ff);border-color:var(--th-accent,#0a84ff)}
.au-tagchip.on{color:#fff;background:var(--th-accent,#0a84ff);border-color:var(--th-accent,#0a84ff)}
.au-tagchip i{font-style:normal;font-size:11px;opacity:.7}
</style>
