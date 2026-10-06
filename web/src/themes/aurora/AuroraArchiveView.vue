<template>
  <div class="au-root">
    <AuroraHead />

    <main class="au-main">
      <section class="au-hero au-hero-sm">
        <p class="au-kicker">{{ $t('归档') }}</p>
        <div class="au-hero-name">{{ $t('按年份浏览') }}</div>
        <p class="au-hero-desc">
          {{ $t('共') }} {{ groups.length }} {{ $t('个年份 ·') }} {{ posts.length }} {{ $t('篇公开文章') }}
        </p>
      </section>

      <template v-if="groups.length">
        <!-- 年份跳转用 button + scrollIntoView，绝不把锚点写进 href / location.hash -->
        <div class="au-chips">
          <button
            v-for="g in groups"
            :key="g.year"
            class="au-chip"
            type="button"
            @click="jumpTo(g.year)"
          >
            {{ g.year }} {{ $t('年') }}<em class="au-chip-n">{{ g.list.length }}</em>
          </button>
        </div>

        <section v-for="g in groups" :id="'au-y-' + g.year" :key="g.year" class="au-year">
          <div class="au-sec-head">
            <div class="au-sec-t">{{ g.year }} {{ $t('年') }}</div>
            <div class="au-sec-n">{{ g.list.length }} {{ $t('篇') }}</div>
          </div>
          <AuroraGrid :posts="g.list" />
        </section>
      </template>

      <div v-else class="au-state">
        <div class="au-state-t">{{ $t('还没有公开文章') }}</div>
      </div>
    </main>

    <AuroraFoot />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
/**
 * AuroraArchiveView —— 归档页（entries.archive）。
 * 系统不提供归档接口，分组全部由 posts 的 file.updated_at 前端派生（规范 §4.7）。
 */
import { computed } from 'vue'
import AuroraHead from './AuroraHead.vue'
import AuroraFoot from './AuroraFoot.vue'
import AuroraGrid from './AuroraGrid.vue'
import './style.css'
import { useBlog, yearGroups } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const groups = computed(() => yearGroups(posts.value))

function jumpTo(year) {
  const el = document.getElementById('au-y-' + year)
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
</script>
