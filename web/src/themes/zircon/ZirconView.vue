<template>
  <div class="zc-root">
    <ZirconHead />

    <main class="zc-main">
      <!-- 渐变头图带：站点名 + 描述 + 可证事实（篇数 / 分类数） -->
      <section class="zc-hero">
        <p class="zc-kicker">{{ $t('Zircon · 青绿胶囊') }}</p>
        <div class="zc-hero-name">{{ siteName }}</div>
        <p class="zc-hero-desc">{{ siteDesc }}</p>
        <div class="zc-hero-stat">
          <span>{{ posts.length }} {{ $t('篇公开文章') }}</span>
          <span class="zc-hero-line" aria-hidden="true"></span>
          <span>{{ cats.length }} {{ $t('个分类') }}</span>
          <span class="zc-hero-line" aria-hidden="true"></span>
          <span>{{ $t('RSS 实时同步') }}</span>
        </div>
      </section>

      <!-- 加载中 -->
      <div v-if="loading" class="zc-state">
        <div class="zc-state-t">{{ $t('加载中…') }}</div>
        <div class="zc-list">
          <div v-for="n in 4" :key="n" class="zc-skel">
            <div class="zc-skel-line"></div>
            <div class="zc-skel-line"></div>
            <div class="zc-skel-line"></div>
          </div>
        </div>
      </div>

      <!-- 错误态 -->
      <div v-else-if="error" class="zc-state">
        <div class="zc-state-t">{{ error }}</div>
        <div class="zc-state-d">{{ $t("稍后重试，或直接访问静态页列表。") }}</div>
      </div>

      <!-- 空态 -->
      <div v-else-if="!posts.length" class="zc-state">
        <div class="zc-state-t">{{ $t('还没有公开文章') }}</div>
        <div class="zc-state-d">{{ $t("在知识库里把文件分享出来，这里就会出现列表。") }}</div>
      </div>

      <template v-else>
        <div class="zc-layout">
          <!-- 左：分类筛选 + 文章卡片流 -->
          <div class="zc-flow">
            <!-- 分类筛选：纯前端筛选已加载文章，用 button 而非 hash 链接（规范 §3.2） -->
            <div v-if="cats.length > 1" class="zc-chips">
              <button
                class="zc-chip"
                :class="{ on: activeCat === '' }"
                type="button"
                @click="pickCat('')"
              >
                {{ $t('全部') }}<em class="zc-chip-n">{{ posts.length }}</em>
              </button>
              <button
                v-for="c in cats"
                :key="c.name"
                class="zc-chip"
                :class="{ on: activeCat === c.name }"
                type="button"
                @click="pickCat(c.name)"
              >
                {{ c.name }}<em class="zc-chip-n">{{ c.count }}</em>
              </button>
            </div>

            <ZirconList :posts="shown" lead />

            <div v-if="!shown.length" class="zc-state">
              <div class="zc-state-t">{{ $t('该分类下暂无公开文章') }}</div>
            </div>
          </div>

          <!-- 右：信息侧栏（只用可证事实，不用任何质量评价词） -->
          <aside class="zc-side">
            <div v-if="latest.length" class="zc-side-box">
              <div class="zc-side-t">{{ $t('最新更新') }}</div>
              <ul class="zc-side-list">
                <li v-for="(p, i) in latest" :key="i" class="zc-side-li">
                  <a class="zc-side-a" :href="postHref(ctx, p)">{{ postTitle(p) }}</a>
                  <span class="zc-side-d">{{ postDate(p, 'ymd') }}</span>
                </li>
              </ul>
            </div>

            <div v-if="cats.length" class="zc-side-box">
              <div class="zc-side-t">{{ $t('分类目录') }}</div>
              <ul class="zc-side-list">
                <li v-for="c in cats" :key="c.name" class="zc-side-li">
                  <button class="zc-side-a zc-side-btn" type="button" @click="pickCat(c.name)">
                    {{ c.name }}
                  </button>
                  <span class="zc-side-d">{{ c.count }} {{ $t('篇') }}</span>
                </li>
              </ul>
            </div>

            <div v-if="tags.length" class="zc-side-box">
              <div class="zc-side-t">{{ $t('标签聚合') }}</div>
              <div class="zc-side-tags">
                <span v-for="t in tags" :key="t.name" class="zc-side-tag">{{ t.name }}<em v-if="t.count" class="zc-tag-n">{{ t.count }}</em></span>
              </div>
            </div>
          </aside>
        </div>
      </template>
    </main>

    <ZirconFoot />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
/**
 * ZirconView —— 青璃主题列表页（主题入口）。
 *
 * 取数完全走 inject('themeContext')（无 props）；顺序沿用系统返回的
 * 「置顶优先 + 更新时间倒序」，主题不做任何重排（规范 §2.4）。
 * 分类筛选是读者显式操作，只在本页过滤已加载文章，不写 location.hash。
 * 侧栏「最新更新」取已加载列表前 5 条（系统顺序），不是任何排行榜语义。
 */
import { computed, ref } from 'vue'
import ZirconHead from './ZirconHead.vue'
import ZirconFoot from './ZirconFoot.vue'
import ZirconList from './ZirconList.vue'
import './style.css'
import { useBlog, catCounts, catName, postTitle, postDate, postHref } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const siteName = computed(() => ctx.value.siteName || '爱库录')
const siteDesc = computed(() => ctx.value.siteDesc || '目录即站点，文件即文章')
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const tags = computed(() => ctx.value.tags || [])

const activeCat = ref('')
const cats = computed(() => catCounts(posts.value))
const shown = computed(() =>
  activeCat.value ? posts.value.filter((p) => catName(p) === activeCat.value) : posts.value
)
const latest = computed(() => posts.value.slice(0, 5))

function pickCat(name) {
  activeCat.value = activeCat.value === name ? '' : name
}
</script>
