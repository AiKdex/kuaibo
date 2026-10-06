<template>
  <!-- 顶部通栏 -->
  <div class="ef-topbar">
    <div class="ef-wrap ef-topbar-in">
      <span>{{ siteDesc || $t("目录即站点，文件即文章") }}</span>
      <span>
        <a :href="blogUrl">{{ $t('文章') }}</a>
        <a :href="rssHref">RSS</a>
      </span>
    </div>
  </div>

  <!-- 品牌区 + 导航（自带主题切换器：manifest 已声明 ui.themeSwitcher） -->
  <header class="ef-masthead">
    <div class="ef-wrap ef-mast-row">
      <div class="ef-brand">
        <!-- 站名用 div，不用 h1（规范 §5.3） -->
        <div class="ef-name">{{ siteName || '爱库录' }}</div>
        <span class="ef-slogan">{{ slogan }}</span>
      </div>
      <div class="ef-head-right">
        <div v-if="$slots.stats" class="ef-stats-mini"><slot name="stats" /></div>
        <nav class="ef-nav">
          <ThemeSwitch />
          <a :href="blogUrl">{{ $t('文章') }}</a>
          <a :href="rssHref">RSS</a>
        </nav>
      </div>
    </div>
    <slot name="boardbar" />
  </header>
</template>

<script setup>
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import { LIST_HREF, RSS_HREF } from './helpers.js'

defineProps({
  siteName: { type: String, default: '' },
  siteDesc: { type: String, default: '' },
  blogUrl: { type: String, default: LIST_HREF },
  slogan: { type: String, default: 'EMFORUM · BBS STYLE' },
})

const rssHref = RSS_HREF
</script>
