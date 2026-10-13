<template>
  <!-- 跑马灯通栏（纯装饰：风格关键词轮播） -->
  <div class="bt-marquee" aria-hidden="true">
    <div class="bt-marquee-track">
      <span v-for="n in 2" :key="n" class="bt-marquee-set">
        <i>新粗野主义</i><i>硬边框</i><i>硬阴影</i><i>高饱和撞色</i><i>零圆角</i><i>不装温柔</i>
      </span>
    </div>
  </div>

  <!-- 品牌区 + 导航（自带主题切换器：manifest 已声明 ui.themeSwitcher） -->
  <header class="bt-masthead">
    <div class="bt-wrap bt-mast-row">
      <a class="bt-brand" :href="blogUrl">
        <span class="bt-brand-mark">粗</span>
        <span class="bt-brand-txt">
          <!-- 站名用 div，不用 h1（规范 §5.3） -->
          <span class="bt-brand-name">{{ siteName || '爱库录' }}</span>
          <span class="bt-brand-sub">{{ slogan }}</span>
        </span>
      </a>

      <div class="bt-head-right">
        <div v-if="$slots.stats" class="bt-stats"><slot name="stats" /></div>
        <nav class="bt-nav">
          <ThemeSwitch />
          <a :href="blogUrl">全部文章</a>
          <a :href="rssHref">RSS</a>
        </nav>
      </div>
    </div>
    <slot name="subbar" />
  </header>
</template>

<script setup>
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import { LIST_HREF, RSS_HREF } from './helpers.js'

defineProps({
  siteName: { type: String, default: '' },
  slogan: { type: String, default: 'BRUTALOG · RAW & BOLD' },
  blogUrl: { type: String, default: LIST_HREF },
})

const rssHref = RSS_HREF
</script>
