<template>
  <div class="cx-root">
    <header class="cx-top"><div class="cx-top-in">
      <div class="cx-logo"><div class="cx-logo-dot">☀️</div><span>{{ ctx.siteName||'晨曦笔记' }}</span></div>
      <nav class="cx-nav"><a :href="homeHref" class="cx-nav-a">{{ $t('首页') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="cx-hero"><div class="cx-hero-card"><div class="cx-hero-inner"><div class="cx-hero-text">
      <h1 class="cx-hero-h1">{{ q?$t("搜索「")+q+'」':$t("搜索") }}</h1><p class="cx-hero-p">{{ list.length }} {{ $t('条结果') }}</p>
    </div></div></div></section>
    <div class="cx-main-wrap"><main>
      <div v-if="list.length" class="cx-post-list">
        <a v-for="p in list" :key="p.token" class="cx-post-card" :href="postHref(p)">
          <div class="cx-post-thumb" style="background:linear-gradient(135deg,#ede9fe,#ddd6fe)"><div class="cx-thumb-inner">🔍</div></div>
          <div class="cx-post-body">
            <div class="cx-post-cats"><span class="cx-post-cat cx-c-pink">{{ postCat(p)||$t("未分类") }}</span></div>
            <h2>{{ postTitle(p) }}</h2><p>{{ p.preview||$t("暂无摘要") }}</p>
            <div class="cx-post-meta"><span>📅 {{ postDate(p) }}</span></div>
          </div>
        </a>
      </div>
      <p v-else class="cx-status">{{ q?$t("未找到匹配文章"):$t("请输入搜索关键词") }}</p>
    </main></div>
    <footer class="cx-footer"><div class="cx-footer-brand">☀️ {{ ctx.siteName||'晨曦笔记' }}</div><p class="cx-footer-copy">© {{ year }} · Powered by AiKlog</p></footer>
  </div>
    <AskWidget />
</template>
<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'
const raw=inject('themeContext');const ctx=computed(()=>raw?.value||raw||{posts:[]})
const page=computed(()=>ctx.value.page||{});const q=computed(()=>page.value.q||'')
const year=new Date().getFullYear();const homeHref='/app#/blog?view=public'
function postTitle(p){const n=p.file?.name||p.path||'';return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop()|| t('common.unnamed')}
function postCat(p){const path=p.path||'';return path.includes('/')?path.split('/')[0]:''}
function postDate(p){const v=p.file?.updated_at;if(!v)return'';const ms=typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v);if(!Number.isFinite(ms))return'';const d=new Date(ms);const z=n=>String(n).padStart(2,'0');return`${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}`}
function postHref(p){return ctx.value.postUrl?ctx.value.postUrl(p):'#'}
const list=computed(()=>{const keyword=q.value.trim().toLowerCase();if(!keyword)return[];return(ctx.value.posts||[]).filter(p=>postTitle(p).toLowerCase().includes(keyword)||(p.preview||'').toLowerCase().includes(keyword))})
</script>
