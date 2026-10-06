<template>
  <!-- 共享文章页侧栏：最新文章 + 标签云 + 关于。
       内容统一、视觉走主题令牌（--th-*）+ 安全兜底，因此每个主题自动套用自身配色，无需逐主题重写。 -->
  <aside class="sw-side">
    <section class="sw-block">
      <h3 class="sw-h">{{ $t('最新文章') }}</h3>
      <ul class="sw-list">
        <li v-for="p in recent" :key="p.token || p.path">
          <a class="sw-link" :href="postHref(p)">{{ postTitle(p) }}</a>
          <span class="sw-date" v-if="postDate(p)">{{ postDate(p) }}</span>
        </li>
      </ul>
    </section>

    <section class="sw-block" v-if="tags && tags.length">
      <h3 class="sw-h">{{ $t('标签') }}</h3>
      <div class="sw-tags">
        <a
          v-for="t in tags"
          :key="t.name"
          class="sw-tag"
          :href="'#/blog/tag/' + encodeURIComponent(t.name)"
        >{{ t.name }}</a>
      </div>
    </section>

    <section class="sw-block sw-about" v-if="siteName">
      <h3 class="sw-h">{{ $t('关于') }}</h3>
      <p class="sw-about-name">{{ siteName }}</p>
      <p class="sw-about-desc" v-if="siteTag">{{ siteTag }}</p>
    </section>
  </aside>
</template>

<script setup>
import { t } from '@/i18n'
import { computed, inject } from 'vue'

function useCtx() {
  const raw = inject('themeContext', null)
  return computed(() => raw?.value || raw || {})
}
const ctx = useCtx()

const recent = computed(() => {
  const posts = ctx.value?.posts || []
  return posts.slice(0, 6)
})
const tags = computed(() => ctx.value?.tags || [])
const siteName = computed(() => ctx.value?.site?.name || ctx.value?.siteName || '')
const siteTag = computed(() => ctx.value?.site?.tag || ctx.value?.siteTag || '')

function postTitle(p) {
  const n = p.file?.name || p.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('common.unnamed')
}
function postDate(p) {
  const v = p.file?.updated_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  const z = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}`
}
function postHref(p) {
  return ctx.value?.postUrl ? ctx.value.postUrl(p) : '#'
}
</script>

<style scoped>
.sw-side {
  display: flex; flex-direction: column; gap: 18px;
  font-family: var(--th-font-b, var(--th-font, system-ui, sans-serif));
}
.sw-block {
  background: var(--th-card, #fff);
  border: 1px solid var(--th-line, #e3e8ef);
  border-radius: 10px;
  padding: 14px 16px;
}
.sw-h {
  margin: 0 0 12px;
  font-size: 14px; font-weight: 700; letter-spacing: 1px;
  color: var(--th-ink, #222);
  border-left: 3px solid var(--th-accent, #2b6e6e);
  padding-left: 10px;
}
.sw-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; }
.sw-link {
  color: var(--th-ink, #222); text-decoration: none; font-size: 13px; line-height: 1.5;
  display: block; transition: color 0.15s;
}
.sw-link:hover { color: var(--th-accent, #2b6e6e); }
.sw-date { display: block; font-size: 11px; color: var(--th-ink-3, #8a94a6); margin-top: 2px; }
.sw-tags { display: flex; flex-wrap: wrap; gap: 8px; }
.sw-tag {
  display: inline-block; padding: 4px 10px; border-radius: 999px; font-size: 12px;
  background: var(--th-paper, #f6f8fb); color: var(--th-ink, #222);
  border: 1px solid var(--th-line, #e3e8ef); text-decoration: none; transition: 0.15s;
}
.sw-tag:hover { background: var(--th-accent, #2b6e6e); color: #fff; border-color: var(--th-accent, #2b6e6e); }
.sw-about-name { margin: 0 0 4px; font-size: 14px; font-weight: 700; color: var(--th-ink, #222); }
.sw-about-desc { margin: 0; font-size: 12px; color: var(--th-ink-3, #8a94a6); line-height: 1.6; }
</style>
