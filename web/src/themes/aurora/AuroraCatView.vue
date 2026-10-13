<template>
  <div class="au-root">
    <AuroraHead />

    <main class="au-main">
      <section class="au-hero au-hero-sm">
        <p class="au-kicker">分类</p>
        <div class="au-hero-name">{{ cat || '未识别到分类参数' }}</div>
        <p class="au-hero-desc">按目录首段聚合，共 {{ list.length }} 篇公开文章。</p>
      </section>

      <AuroraGrid v-if="list.length" :posts="list" />

      <div v-else class="au-state">
        <div class="au-state-t">
          {{ cat ? '该分类下暂无公开文章' : '未识别到分类参数' }}
        </div>
        <div class="au-state-d">分类 = 文章所在目录的第一段。</div>
      </div>
    </main>

    <AuroraFoot />
  </div>
</template>

<script setup>
/**
 * AuroraCatView —— 分类页（entries.cat / entries.category）。
 * 数据：ctx.page.cat（系统侧路由补齐后注入）+ ctx.posts 按 path 首段过滤；
 * 系统未注入时回退解析 hash（只读），保证当前版本可预览。
 */
import { computed } from 'vue'
import AuroraHead from './AuroraHead.vue'
import AuroraFoot from './AuroraFoot.vue'
import AuroraGrid from './AuroraGrid.vue'
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
