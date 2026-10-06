<template>
  <div class="zc-root">
    <ZirconHead />

    <main class="zc-main">
      <section class="zc-hero zc-hero-sm">
        <p class="zc-kicker">{{ $t('归档') }}</p>
        <div class="zc-hero-name">{{ $t('按年份浏览') }}</div>
        <p class="zc-hero-desc">
          {{ $t('共') }} {{ groups.length }} {{ $t('个年份 ·') }} {{ posts.length }} {{ $t('篇公开文章') }}
        </p>
      </section>

      <template v-if="groups.length">
        <!-- 年份跳转用 button + scrollIntoView，绝不把锚点写进 href / location.hash -->
        <div class="zc-chips">
          <button
            v-for="g in groups"
            :key="g.year"
            class="zc-chip"
            type="button"
            @click="jumpTo(g.year)"
          >
            {{ g.year }} {{ $t('年') }}<em class="zc-chip-n">{{ g.list.length }}</em>
          </button>
        </div>

        <section v-for="g in groups" :id="'zc-y-' + g.year" :key="g.year" class="zc-year">
          <div class="zc-sec-head">
            <div class="zc-sec-t">{{ g.year }} {{ $t('年') }}</div>
            <div class="zc-sec-n">{{ g.list.length }} {{ $t('篇') }}</div>
          </div>
          <ZirconList :posts="g.list" />
        </section>
      </template>

      <div v-else class="zc-state">
        <div class="zc-state-t">{{ $t('还没有公开文章') }}</div>
      </div>
    </main>

    <ZirconFoot />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
/**
 * ZirconArchiveView —— 归档页（entries.archive）。
 * 系统不提供归档接口，分组全部由 posts 的 file.updated_at 前端派生（规范 §4.7）。
 */
import { computed } from 'vue'
import ZirconHead from './ZirconHead.vue'
import ZirconFoot from './ZirconFoot.vue'
import ZirconList from './ZirconList.vue'
import './style.css'
import { useBlog, yearGroups } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const groups = computed(() => yearGroups(posts.value))

function jumpTo(year) {
  const el = document.getElementById('zc-y-' + year)
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
</script>
