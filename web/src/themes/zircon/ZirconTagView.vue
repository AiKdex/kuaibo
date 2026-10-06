<template>
  <div class="zc-root">
    <ZirconHead />

    <main class="zc-main">
      <section class="zc-hero zc-hero-sm">
        <p class="zc-kicker">{{ $t('标签') }}</p>
        <div class="zc-hero-name">{{ tag || $t("全部文章") }}</div>
        <p class="zc-hero-desc">{{ $t('按文章标签聚合，共') }} {{ list.length }} {{ $t('篇公开文章。') }}</p>
      </section>

      <nav v-if="tagList.length" class="zc-tagcloud">
        <a
          v-for="t in tagList"
          :key="t.name"
          class="zc-tagchip"
          :class="{ on: t.name === (tag || '') }"
          :href="'#/blog/tag/' + encodeURIComponent(t.name)"
        >{{ t.name }}<i v-if="t.count">{{ t.count }}</i></a>
      </nav>

      <ZirconList v-if="list.length" :posts="list" />

      <div v-else class="zc-state">
        <div class="zc-state-t">
          {{ tag ? $t("该标签下暂无公开文章") : $t("尚未选择标签") }}
        </div>
        <div class="zc-state-d">{{ $t("标签 = 文章元数据里的标签字段，点击上方标签可筛选。") }}</div>
      </div>
    </main>

    <ZirconFoot />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
/**
 * ZirconTagView —— 标签页（entries.tag）。
 * 数据：ctx.page.tag（系统侧路由补齐后注入）+ ctx.posts 按 file.tags 过滤；
 * 系统未注入时回退解析 hash（只读），保证当前版本可预览。
 */
import { computed } from 'vue'
import ZirconHead from './ZirconHead.vue'
import ZirconFoot from './ZirconFoot.vue'
import ZirconList from './ZirconList.vue'
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
.zc-tagcloud{display:flex;flex-wrap:wrap;gap:10px;margin:0 0 28px}
.zc-tagchip{
  display:inline-flex;align-items:center;gap:6px;
  padding:6px 16px;border-radius:999px;font-size:13px;font-weight:600;
  text-decoration:none;color:var(--th-ink-2,#475569);
  background:var(--th-muted,#eef2f6);border:1px solid var(--th-line,#e2e8f0);
  transition:.2s;
}
.zc-tagchip:hover{color:var(--th-primary,#16b597);border-color:var(--th-primary,#16b597)}
.zc-tagchip.on{color:#fff;background:var(--th-primary,#16b597);border-color:var(--th-primary,#16b597)}
.zc-tagchip i{font-style:normal;font-size:11px;opacity:.75}
</style>
