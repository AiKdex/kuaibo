<template>
  <div class="zc-root">
    <header class="zc-top"><div class="zc-top-in">
      <div class="zc-logo"><div class="zc-logo-icon">📚</div><span>{{ ctx.siteName || '知藏' }}</span></div>
      <nav class="zc-nav"><a class="zc-nav-a" :href="homeHref">{{ $t('发现') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="zc-hero"><div class="zc-hero-text"><h1 class="zc-hero-h1"><em>{{ $t('归档') }}</em></h1><p class="zc-hero-p">{{ $t('共') }} {{ ctx.posts?.length || 0 }} {{ $t('篇资源') }}</p></div></section>
    <div class="zc-main-wrap"><main class="zc-main">
      <div v-for="group in archives" :key="group.ym" style="margin-bottom:40px">
        <div class="zc-section-head"><div class="zc-section-title"><span class="zc-icon">📅</span> {{ group.label }}</div><span class="zc-section-cnt">{{ group.posts.length }}</span></div>
        <div class="zc-res-grid">
          <a v-for="p in group.posts" :key="p.token" class="zc-res-card" :href="postHref(p)">
            <div class="zc-res-cover" style="background:linear-gradient(135deg,#e0f2fe,#bae6fd)"><div class="zc-res-pattern">📄</div></div>
            <div class="zc-res-body">
              <div class="zc-res-type">{{ postCat(p) || '资源' }}</div>
              <h3>{{ postTitle(p) }}</h3>
              <p>{{ p.preview || $t("暂无摘要") }}</p>
              <div class="zc-res-meta"><span>📅 {{ postDate(p) }}</span></div>
            </div>
          </a>
        </div>
      </div>
      <p v-if="!archives.length" class="zc-status">{{ $t('暂无资源') }}</p>
    </main></div>
    <footer class="zc-footer"><div class="zc-footer-brand">📚 {{ ctx.siteName || '知藏' }}</div><p class="zc-footer-copy">© {{ year }} · Powered by AiKlog</p></footer>
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
function postCat(p) { const path = p.path || '' ; return path.includes('/') ? path.split('/')[0] : '' }
function postDate(p) { const v = p.file?.updated_at ; if(!v) return '' ; const ms = typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v) ; if(!Number.isFinite(ms)) return '' ; const d=new Date(ms) ; const z=n=>String(n).padStart(2,'0') ; return `${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}` }
function postHref(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '#' }
const archives = computed(() => {
  const map = {} ; const posts = ctx.value.posts || []
  for (const p of posts) { const v = p.file?.updated_at ; if(!v) continue ; const ms = typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v) ; if(!Number.isFinite(ms)) continue ; const d = new Date(ms) ; const ym = `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}` ; if(!map[ym]) map[ym] = { ym, label: `${d.getFullYear()} 年 ${d.getMonth()+1} 月`, posts: [] } ; map[ym].posts.push(p) }
  return Object.values(map).sort((a,b) => b.ym.localeCompare(a.ym))
})
</script>