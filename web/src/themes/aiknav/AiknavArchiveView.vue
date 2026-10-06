<template>
  <div class="an-root">
    <header class="an-top"><div class="an-top-in">
      <div class="an-logo"><span class="an-logo-icon">◆</span><span>{{ ctx.siteName || '好站导航' }}</span></div>
      <nav class="an-nav"><a class="an-nav-btn" :href="homeHref">{{ $t('全部') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="an-hero"><h1 class="an-hero-h1">{{ $t('归档') }}</h1><p class="an-hero-p">{{ $t('共') }} {{ ctx.posts?.length || 0 }} {{ $t('篇文章') }}</p></section>
    <div class="an-wrap"><main class="an-main">
      <div v-for="group in archives" :key="group.ym" class="an-group">
        <div class="an-section"><span class="an-dot" style="background:var(--th-blue)"></span><h2 class="an-section-h2">{{ group.label }}</h2><span class="an-line"></span><span class="an-cnt">{{ group.posts.length }}</span></div>
        <div class="an-grid">
          <a v-for="p in group.posts" :key="p.token" class="an-card" :href="postHref(p)">
            <div class="an-card-icon"><span>📄</span></div>
            <div class="an-card-info">
              <h3 class="an-card-h3">{{ postTitle(p) }}</h3>
              <p class="an-card-desc">{{ p.preview || $t("暂无摘要") }}</p>
              <div class="an-card-meta"><span class="an-tag an-tag-green">{{ postDate(p) }}</span></div>
            </div><span class="an-arrow">→</span>
          </a>
        </div>
      </div>
      <p v-if="!archives.length" class="an-empty">{{ $t('暂无文章') }}</p>
    </main></div>
    <footer class="an-footer"><p>© {{ year }} {{ ctx.siteName || '好站导航' }} · Powered by AiKlog</p></footer>
  </div>
    <AskWidget />
</template>
<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'
const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [] })
const year = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
function postTitle(p) { const n = p.file?.name || p.path || '' ; return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop() || t('common.unnamed') }
function postDate(p) { const v = p.file?.updated_at ; if(!v) return '' ; const ms = typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v) ; if(!Number.isFinite(ms)) return '' ; const d=new Date(ms) ; const z=n=>String(n).padStart(2,'0') ; return `${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}` }
function postHref(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '#' }
const archives = computed(() => {
  const map = {}
  for (const p of (ctx.value.posts || [])) {
    const v = p.file?.updated_at ; if(!v) continue
    const ms = typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v) ; if(!Number.isFinite(ms)) continue
    const d = new Date(ms) ; const ym = `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}`
    if(!map[ym]) map[ym] = { ym, label: `${d.getFullYear()} 年 ${d.getMonth()+1} 月`, posts: [] }
    map[ym].posts.push(p)
  }
  return Object.values(map).sort((a,b) => b.ym.localeCompare(a.ym))
})
</script>