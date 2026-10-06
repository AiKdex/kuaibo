<template>
  <div class="cxp-root">
    <header class="cxp-top"><div class="cxp-top-in">
      <div class="cxp-logo"><div class="cxp-logo-dot">☀️</div><a :href="homeHref" class="cxp-logo-text">{{ ctx.siteName||'晨曦笔记' }}</a></div>
      <nav class="cxp-nav"><a :href="homeHref" class="cxp-nav-a">{{ $t('← 返回列表') }}</a><ThemeSwitch /></nav>
    </div></header>
    <div v-if="loading" class="cxp-status"><p>{{ $t('⏳ 加载中...') }}</p></div>
    <div v-else-if="error" class="cxp-status"><p>❌ {{ error }}</p></div>
    <div v-else class="cx-layout"><article class="cx-article cxp-wrapper">
      <header class="cxp-head">
        <div class="cxp-head-tags"><span v-if="cat" class="cxp-chip">{{ cat }}</span></div>
        <h1 class="cxp-h1">{{ title }}</h1>
        <div class="cxp-meta"><span v-if="date" class="cxp-meta-item">📅 {{ date }}</span><span v-if="cat" class="cxp-meta-item">📂 {{ cat }}</span></div>
      </header>
      <div v-if="post.preview" class="cxp-summary"><div class="cxp-summary-label">{{ $t('📋 摘要') }}</div><p>{{ post.preview }}</p></div>
      <div class="cxp-body" v-html="postHtml"></div>
      <div v-if="shareHref" class="cxp-share"><a :href="shareHref" target="_blank" rel="noopener">{{ $t('🔗 查看静态页面 / 分享链接') }}</a></div>
      <div class="cxp-tags"><span v-if="cat" class="cxp-chip">{{ cat }}</span></div>
      <div v-if="related.length" class="cxp-related">
        <h3 class="cxp-related-title">{{ $t('📎 相关文章') }}</h3>
        <div class="cxp-related-grid"><a v-for="p in related" :key="p.token" class="cxp-related-card" :href="relatedHref(p)"><h4>{{ relatedTitle(p) }}</h4><p>{{ relatedDate(p) }}</p></a></div>
      </div>
    </article>
    <aside class="cx-aside"><SideWidget /></aside>
    </div>
    <footer class="cxp-footer"><p>© {{ year }} {{ ctx.siteName||'晨曦笔记' }} · Powered by AiKlog</p></footer>
  </div>
    <AskWidget />
</template>
<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import SideWidget from '../SideWidget.vue'
import { computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'
import './post.css'
const raw=inject('themeContext');const ctx=computed(()=>raw?.value||raw||{posts:[],loading:true})
const loading=computed(()=>ctx.value.loading);const error=computed(()=>ctx.value.error||'')
const page=computed(()=>ctx.value.page||{});const post=computed(()=>page.value.post||{})
const year=new Date().getFullYear();const homeHref='/app#/blog?view=public'
const title=computed(()=>post.value.title|| t('common.unnamed'))
const cat=computed(()=>post.value.cat||post.value.category||'')
const date=computed(()=>{const v=post.value.updatedAt;if(!v)return'';const ms=typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v);if(!Number.isFinite(ms))return'';const d=new Date(ms);const z=n=>String(n).padStart(2,'0');return`${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}`})
const postHtml=computed(()=>post.value.html||'')
const shareHref=computed(()=>ctx.value.postSsrUrl&&post.value?ctx.value.postSsrUrl(post.value):'')
const related=computed(()=>{const posts=ctx.value.posts||[];const cur=post.value.token;const c=cat.value;return posts.filter(p=>p.token!==cur&&(c?(p.path||'').startsWith(c+'/'):true)).slice(0,3)})
function relatedTitle(p){const n=p.file?.name||p.path||'';return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop()|| t('common.unnamed')}
function relatedDate(p){const v=p.file?.updated_at;if(!v)return'';const ms=typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v);if(!Number.isFinite(ms))return'';const d=new Date(ms);const z=n=>String(n).padStart(2,'0');return`${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}`}
function relatedHref(p){return ctx.value.postUrl?ctx.value.postUrl(p):'#'}
</script>
