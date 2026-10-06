<template>
  <div class="zc-root">
    <ZirconHead />

    <main class="zc-main">
      <section class="zc-hero zc-hero-sm">
        <p class="zc-kicker">{{ $t('分类') }}</p>
        <div class="zc-hero-name">{{ cat || $t("未识别到分类参数") }}</div>
        <p class="zc-hero-desc">{{ $t('按目录首段聚合，共') }} {{ list.length }} {{ $t('篇公开文章。') }}</p>
      </section>

      <ZirconList v-if="list.length" :posts="list" />

      <div v-else class="zc-state">
        <div class="zc-state-t">
          {{ cat ? $t("该分类下暂无公开文章") : $t("未识别到分类参数") }}
        </div>
        <div class="zc-state-d">{{ $t('分类 = 文章所在目录的第一段。') }}</div>
      </div>
    </main>

    <ZirconFoot />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
/**
 * ZirconCatView —— 分类页（entries.cat / entries.category）。
 * 数据：ctx.page.cat（系统侧路由补齐后注入）+ ctx.posts 按 path 首段过滤；
 * 系统未注入时回退解析 hash（只读），保证当前版本可预览。
 */
import { computed } from 'vue'
import ZirconHead from './ZirconHead.vue'
import ZirconFoot from './ZirconFoot.vue'
import ZirconList from './ZirconList.vue'
import './style.css'
import { useBlog, catName, readPageParam } from './helpers.js'

const ctx = useBlog()
const cat = computed(() =>
  readPageParam(ctx.value, ['cat', 'category'], [/cat\/([^/?#]+)/, /[?&]cat=([^&#]+)/])
)
const list = computed(() =>
  (ctx.value.posts || []).filter((p) => cat.value && catName(p) === cat.value)
)
</script>
