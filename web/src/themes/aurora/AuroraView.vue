<template>
  <div class="au-root">
    <AuroraHead />

    <main class="au-main">
      <!-- 首屏：站点名 + 描述 + 可证事实（篇数 / 分类数） -->
      <section class="au-hero">
        <p class="au-kicker">{{ $t('Bento · 极光 Aurora') }}</p>
        <div class="au-hero-name">{{ siteName }}</div>
        <p class="au-hero-desc">{{ siteDesc }}</p>
        <div class="au-hero-stat">
          <span>{{ posts.length }} {{ $t('篇公开文章') }}</span>
          <span class="au-hero-line" aria-hidden="true"></span>
          <span>{{ cats.length }} {{ $t('个分类') }}</span>
        </div>
      </section>

      <!-- 加载中 -->
      <div v-if="loading" class="au-state">
        <div class="au-state-t">{{ $t('加载中…') }}</div>
        <div class="au-grid au-grid-flat">
          <div v-for="n in 6" :key="n" class="au-skel">
            <div class="au-skel-line"></div>
            <div class="au-skel-line"></div>
            <div class="au-skel-line"></div>
          </div>
        </div>
      </div>

      <!-- 错误态 -->
      <div v-else-if="error" class="au-state">
        <div class="au-state-t">{{ error }}</div>
        <div class="au-state-d">{{ $t("稍后重试，或直接访问静态页列表。") }}</div>
      </div>

      <!-- 空态 -->
      <div v-else-if="!posts.length" class="au-state">
        <div class="au-state-t">{{ $t('还没有公开文章') }}</div>
        <div class="au-state-d">{{ $t("在知识库里把文件分享出来，这里就会出现列表。") }}</div>
      </div>

      <template v-else>
        <!-- 分类筛选：纯前端筛选已加载的文章，用 button 而非 hash 链接（规范 §3.2） -->
        <section v-if="cats.length > 1" class="au-sec">
          <div class="au-sec-head">
            <div class="au-sec-t">{{ $t('按分类浏览') }}</div>
            <div class="au-sec-n">{{ shown.length }} / {{ posts.length }} {{ $t('篇') }}</div>
          </div>
          <div class="au-chips">
            <button
              class="au-chip"
              :class="{ on: activeCat === '' }"
              type="button"
              @click="pickCat('')"
            >
              {{ $t('全部') }}<em class="au-chip-n">{{ posts.length }}</em>
            </button>
            <button
              v-for="c in cats"
              :key="c.name"
              class="au-chip"
              :class="{ on: activeCat === c.name }"
              type="button"
              @click="pickCat(c.name)"
            >
              {{ c.name }}<em class="au-chip-n">{{ c.count }}</em>
            </button>
          </div>
        </section>

        <AuroraGrid :posts="shown" lead />

        <div v-if="!shown.length" class="au-state">
          <div class="au-state-t">{{ $t('该分类下暂无公开文章') }}</div>
        </div>
      </template>
    </main>

    <AuroraFoot />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
/**
 * AuroraView —— 极光主题列表页（主题入口，一期核心页面）。
 *
 * 取数完全走 inject('themeContext')（无 props）；顺序沿用系统返回的
 * 「置顶优先 + 更新时间倒序」，主题不做任何重排（规范 §2.4）。
 * 分类筛选是读者显式操作，只在本页过滤已加载文章，不写 location.hash。
 */
import { computed, ref } from 'vue'
import AuroraHead from './AuroraHead.vue'
import AuroraFoot from './AuroraFoot.vue'
import AuroraGrid from './AuroraGrid.vue'
import './style.css'
import { useBlog, catCounts, catName } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const siteName = computed(() => ctx.value.siteName || '爱库录')
const siteDesc = computed(() => ctx.value.siteDesc || '目录即站点，文件即文章')
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')

const activeCat = ref('')
const cats = computed(() => catCounts(posts.value))
const shown = computed(() =>
  activeCat.value ? posts.value.filter((p) => catName(p) === activeCat.value) : posts.value
)

function pickCat(name) {
  activeCat.value = activeCat.value === name ? '' : name
}
</script>
