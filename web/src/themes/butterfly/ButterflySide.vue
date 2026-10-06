<template>
  <aside class="bf-side">
    <!-- 作者卡：无头像/无作者字段 → 用站名首字做圆形徽标（纯装饰），统计只用真实计数 -->
    <section class="bf-widget bf-wcard">
      <div class="bf-wcard-top">
        <span class="bf-wcard-ava" aria-hidden="true">{{ avatarCh }}</span>
        <span class="bf-wcard-name">{{ siteName || '爱库录' }}</span>
        <span class="bf-wcard-desc">{{ siteDesc || $t("目录即站点，文件即文章") }}</span>
      </div>
      <div class="bf-wcard-stats">
        <span class="bf-wcard-stat">
          <b>{{ posts.length }}</b>
          <i>{{ $t('文章') }}</i>
        </span>
        <span class="bf-wcard-stat">
          <b>{{ cats.length }}</b>
          <i>{{ $t('分类') }}</i>
        </span>
      </div>
    </section>

    <!-- 站点公告：内容取站点描述（真实字段），无则整卡隐藏 -->
    <section v-if="siteDesc" class="bf-widget">
      <h3 class="bf-widget-h">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 9.5v5h3l6 4V5.5l-6 4z" /><path d="M17 8.6a5 5 0 0 1 0 6.8" /></svg>
        <span>{{ $t('站点公告') }}</span>
      </h3>
      <p class="bf-notice">{{ siteDesc }}</p>
    </section>

    <!-- 最新更新：按 existing 顺序取前 5（不重排，标题如实体现在「最新」） -->
    <section v-if="latest.length" class="bf-widget">
      <h3 class="bf-widget-h">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="8.5" /><path d="M12 7.4V12l3.2 2" /></svg>
        <span>{{ $t('最新更新') }}</span>
      </h3>
      <ul class="bf-latest">
        <li v-for="(p, i) in latest" :key="p.token || p.path" class="bf-latest-i">
          <span class="bf-latest-no">{{ String(i + 1).padStart(2, '0') }}</span>
          <a class="bf-latest-t" :href="postHref(ctx, p)">{{ postTitle(p) }}</a>
          <span class="bf-latest-d">{{ postDate(p, 'short') }}</span>
        </li>
      </ul>
    </section>

    <!-- 分类：客户端按 path 首段聚合，点选即筛选列表 -->
    <section v-if="cats.length" class="bf-widget">
      <h3 class="bf-widget-h">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 5.5h7v7h-7zM13.5 5.5h7v7h-7zM3.5 13.5h7v5h-7zM13.5 13.5h7v5h-7z" /></svg>
        <span>{{ $t('分类') }}</span>
      </h3>
      <ul class="bf-cats">
        <li v-for="c in shownCats" :key="c.name">
          <button
            class="bf-cat"
            :class="{ 'is-on': c.name === activeCat }"
            type="button"
            @click="$emit('pick-cat', c.name === activeCat ? '' : c.name)"
          >
            <span class="bf-cat-dot" :class="catTone(c.name)"></span>
            <span class="bf-cat-name">{{ c.name }}</span>
            <span class="bf-cat-num">{{ c.count }}</span>
          </button>
        </li>
      </ul>
      <button v-if="cats.length > 6" class="bf-widget-more" type="button" @click="expandAll = !expandAll">
        {{ expandAll ? $t("收起") : $t("展开全部 {cats} 个分类", { cats: cats.length }) }}
      </button>
    </section>

    <!-- 标签云：系统层标签数据未实现（规范 §4.6），恒空时整卡隐藏 -->
    <section v-if="tags.length" class="bf-widget">
      <h3 class="bf-widget-h">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 11.4V4.5h6.9l9.1 9.1-6.9 6.9z" /><circle cx="7.6" cy="8.4" r="1.3" /></svg>
        <span>{{ $t('标签云') }}</span>
      </h3>
      <div class="bf-tagcloud">
        <span v-for="t in tags" :key="t.name" class="bf-tag">{{ t.name }}<i v-if="t.count">{{ t.count }}</i></span>
      </div>
    </section>

    <!-- 站点信息：全部为真实统计（文章数 / 分类数 / 最近更新时间） -->
    <section v-if="posts.length" class="bf-widget">
      <h3 class="bf-widget-h">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="8.5" /><path d="M12 3.5v8.5h8.5" /></svg>
        <span>{{ $t('站点信息') }}</span>
      </h3>
      <ul class="bf-info">
        <li><span>{{ $t('文章总数') }}</span><b>{{ posts.length }}</b></li>
        <li><span>{{ $t('分类数量') }}</span><b>{{ cats.length }}</b></li>
        <li><span>{{ $t('最近更新') }}</span><b>{{ lastUpdated || '—' }}</b></li>
      </ul>
      <div class="bf-widget-links">
        <button class="bf-widget-link" type="button" @click="$emit('go-archive')">{{ $t('全部归档') }}</button>
        <button class="bf-widget-link" type="button" @click="$emit('go-search')">{{ $t('全文检索') }}</button>
        <a class="bf-widget-link" :href="rssHref" target="_blank" rel="noopener">{{ $t('RSS 订阅') }}</a>
      </div>
    </section>
  </aside>
</template>

<script setup>
import { computed, ref } from 'vue'
import {
  RSS_HREF, catCounts, catTone, initials, postDate, postHref, postTitle, useBlog,
} from './helpers.js'

const props = defineProps({
  posts: { type: Array, default: () => [] },
  tags: { type: Array, default: () => [] },
  siteName: { type: String, default: '' },
  siteDesc: { type: String, default: '' },
  activeCat: { type: String, default: '' },
})

defineEmits(['pick-cat', 'go-archive', 'go-search'])

const ctx = useBlog()
const rssHref = RSS_HREF

const avatarCh = computed(() => initials(props.siteName, 1))
const cats = computed(() => catCounts(props.posts))
const latest = computed(() => (props.posts || []).slice(0, 5))
const lastUpdated = computed(() => postDate(props.posts?.[0] || {}))

/** 分类折叠：默认展示前 6 个，其余「展开全部」——对应 Butterfly 的分类展开动画 */
const expandAll = ref(false)
const shownCats = computed(() => (expandAll.value ? cats.value : cats.value.slice(0, 6)))
</script>
