<template>
  <div class="zc-root">
    <header class="zc-top"><div class="zc-top-in">
      <div class="zc-logo"><div class="zc-logo-icon">📚</div><span>{{ ctx.siteName || '知藏' }}</span></div>
      <nav class="zc-nav"><a class="zc-nav-a zc-on" :href="homeHref">{{ $t('发现') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="zc-hero"><div class="zc-hero-text"><h1 class="zc-hero-h1">{{ $t('分类：') }}<em>{{ cat }}</em></h1><p class="zc-hero-p">{{ list.length }} {{ $t('篇资源') }}</p></div></section>
    <div class="zc-main-wrap"><main class="zc-main">
      <div v-if="list.length" class="zc-res-grid">
        <a v-for="p in list" :key="p.token" class="zc-res-card" :href="postHref(p)">
          <div class="zc-res-cover" :style="coverStyle(p)"><div class="zc-res-pattern">{{ catEmoji }}</div></div>
          <div class="zc-res-body">
            <div class="zc-res-type">{{ cat }}</div>
            <h3>{{ postTitle(p) }}</h3>
            <p>{{ p.preview || $t("暂无摘要") }}</p>
            <div class="zc-res-meta"><span>📅 {{ postDate(p) }}</span></div>
          </div>
        </a>
      </div>
      <p v-else class="zc-status">{{ $t('该分类下暂无资源') }}</p>
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
const page = computed(() => ctx.value.page || {})
const cat = computed(() => page.value.cat || '')
const year = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
const catEmoji = '📁'
const list = computed(() => (ctx.value.posts || []).filter(p => { const c = (p.path||'').includes('/') ? p.path.split('/')[0] : '' ; return c === cat.value }))
function postTitle(p) { const n = p.file?.name || p.path || '' ; return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop() || t('common.unnamed') }
function postDate(p) { const v = p.file?.updated_at ; if(!v) return '' ; const ms = typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v) ; if(!Number.isFinite(ms)) return '' ; const d=new Date(ms) ; const z=n=>String(n).padStart(2,'0') ; return `${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}` }
function postHref(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '#' }
const pats = ['linear-gradient(135deg,#e0e7ff,#c7d2fe)','linear-gradient(135deg,#fce7f3,#fbcfe8)','linear-gradient(135deg,#d1fae5,#a7f3d0)','linear-gradient(135deg,#fef3c7,#fde68a)','linear-gradient(135deg,#e0f2fe,#bae6fd)']
function coverStyle(p) { let h=0 ; const s=cat.value ; for(let i=0;i<s.length;i++)h=((h<<5)-h+s.charCodeAt(i))|0 ; return { background: pats[Math.abs(h)%pats.length] } }
</script>