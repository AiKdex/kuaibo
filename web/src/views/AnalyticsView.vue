<template>
  <div class="an-view">
    <div class="an-head">
      <div>
        <h2>{{  $t('阅读数据')  }}</h2>
        <p class="an-sub">
          {{  $t('内容数据中心（A5）：阅读事件按天聚合，写入口唯一 = 公开页 PV 上报， 因此与文章「累计阅读量」始终同口径。草稿与定时未发布内容不会进入热门榜。')  }}
        </p>
      </div>
      <div class="an-tools">
        <select v-model.number="days" class="an-days" @change="loadAll">
          <option :value="7">{{  $t('近 7 天')  }}</option>
          <option :value="30">{{  $t('近 30 天')  }}</option>
          <option :value="90">{{  $t('近 90 天')  }}</option>
        </select>
        <button class="btn" :disabled="loading" @click="loadAll"><AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}</button>
      </div>
    </div>

    <div class="an-kpis">
      <div class="an-kpi">
        <span class="an-kpi-n">{{  overview.total || 0  }}</span>
        <span class="an-kpi-l">{{  $t('近')  }} {{  days  }} {{  $t('天阅读')  }}</span>
      </div>
      <div class="an-kpi">
        <span class="an-kpi-n">{{  avgPerDay  }}</span>
        <span class="an-kpi-l">{{  $t('日均阅读')  }}</span>
      </div>
      <div class="an-kpi">
        <span class="an-kpi-n">{{  (overview.top || []).length  }}</span>
        <span class="an-kpi-l">{{  $t('期间有阅读的文章')  }}</span>
      </div>
      <div class="an-kpi an-kpi-last">
        <span class="an-kpi-n">{{  peak ? peak.count : 0  }}</span>
        <span class="an-kpi-l">{{  $t('单日峰值')  }}{{  peak ? '（' + peak.day.slice(5) + '）' : ''  }}</span>
      </div>
    </div>

    <div class="an-card">
      <div class="an-card-head"><b>{{  $t('全站阅读趋势')  }}</b><span class="an-hint">{{  $t('按天，缺天补 0')  }}</span></div>
      <div class="an-chart-wrap">
        <canvas ref="chartEl"></canvas>
        <div v-if="!hasData" class="an-chart-empty">
          <AikIcon name="grid" :size="24" />
          <p>{{  $t('还没有阅读数据')  }}</p>
          <p class="an-hint">{{  $t('读者访问公开文章页（SSR 或交互版）并计为首次访问后才会产生记录。')  }}</p>
        </div>
      </div>
    </div>

    <div class="an-main">
      <div class="an-card an-top">
        <div class="an-card-head"><b>{{  $t('阅读排行')  }}</b><span class="an-hint">{{  $t('窗口内 top 20')  }}</span></div>
        <div v-if="loading" class="an-empty">{{  $t('加载中…')  }}</div>
        <div v-else-if="!(overview.top || []).length" class="an-empty">{{  $t('暂无数据')  }}</div>
        <div
          v-for="(p, i) in overview.top"
          :key="p.id"
          class="an-row"
          :class="{ sel: cur && cur.id === p.id }"
          @click="selectPost(p)"
        >
          <span class="an-rank" :class="{ top3: i < 3 }">{{  i + 1  }}</span>
          <div class="an-row-main">
            <div class="an-row-name" :title="p.name">{{  p.name || p.id  }}</div>
            <div class="an-bar"><i :style="{ width: barWidth(p) }"></i></div>
          </div>
          <div class="an-row-nums">
            <b>{{  p.window_views  }}</b>
            <span>{{  $t('累计')  }} {{  p.view_count  }}</span>
          </div>
        </div>
      </div>

      <div class="an-card an-detail">
        <div class="an-card-head">
          <b>{{  cur ? cur.name : $t('单篇热力')  }}</b>
          <span v-if="heat" class="an-hint">{{  $t('累计阅读')  }} {{  heat.lifetime  }} {{  $t('· 窗口内')  }} {{  heat.total  }}</span>
        </div>
        <div v-if="!cur" class="an-empty">{{  $t('点击左侧文章查看该篇阅读趋势')  }}</div>
        <div v-else class="an-chart-wrap an-chart-sm">
          <canvas ref="heatEl"></canvas>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, nextTick, onMounted, onUnmounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const toast = useToastStore()

const days = ref(30)
const loading = ref(false)
const overview = ref({ days: 30, total: 0, series: [], top: [] })

const chartEl = ref(null)
const heatEl = ref(null)
let chart = null
let heatChart = null

const cur = ref(null)
const heat = ref(null)

const hasData = computed(() => (overview.value.series || []).some((s) => s.count > 0))
const avgPerDay = computed(() => {
  const d = days.value || 1
  return ((overview.value.total || 0) / d).toFixed(1)
})
const peak = computed(() => {
  let best = null
  for (const s of overview.value.series || []) {
    if (!best || s.count > best.count) best = s
  }
  return best && best.count > 0 ? best : null
})

function barWidth(p) {
  const list = overview.value.top || []
  const max = list.length ? list[0].window_views || 1 : 1
  const w = Math.max(3, Math.round(((p.window_views || 0) / max) * 100))
  return w + '%'
}

async function loadAll() {
  loading.value = true
  try {
    overview.value = await api.adminAnalyticsOverview(days.value)
    await nextTick()
    renderMain()
    // 已选文章若仍在榜上则刷新其热力，否则清空选择
    if (cur.value) {
      const still = (overview.value.top || []).find((p) => p.id === cur.value.id)
      if (still) await selectPost(still)
      else {
        cur.value = null
        heat.value = null
        heatChart?.destroy()
        heatChart = null
      }
    }
  } catch (e) {
    toast.push(t('加载阅读数据失败：') + (e.message || e))
  } finally {
    loading.value = false
  }
}

async function renderMain() {
  const el = chartEl.value
  if (!el) return
  const series = overview.value.series || []
  if (!series.length) return
  try {
    const { Chart } = await import('chart.js/auto')
    chart?.destroy()
    chart = new Chart(el, {
      type: 'line',
      data: {
        labels: series.map((s) => s.day.slice(5)),
        datasets: [
          {
            label: t('阅读量'),
            data: series.map((s) => s.count),
            borderColor: 'rgba(47,107,255,.9)',
            backgroundColor: 'rgba(47,107,255,.14)',
            fill: true,
            tension: 0.3,
            pointRadius: series.length > 45 ? 0 : 2,
            borderWidth: 2
          }
        ]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        scales: { y: { beginAtZero: true, ticks: { precision: 0 } } },
        plugins: { legend: { display: false } }
      }
    })
  } catch (_) {
    /* 图表库加载失败不阻断数字展示 */
  }
}

async function selectPost(p) {
  cur.value = p
  heat.value = null
  try {
    heat.value = await api.fileHeatmap(p.id)
    await nextTick()
    renderHeat()
  } catch (e) {
    toast.push(t('加载单篇热力失败：') + (e.message || e))
  }
}

async function renderHeat() {
  const el = heatEl.value
  const h = heat.value
  if (!el || !h) return
  try {
    const { Chart } = await import('chart.js/auto')
    heatChart?.destroy()
    heatChart = new Chart(el, {
      type: 'bar',
      data: {
        labels: (h.series || []).map((s) => s.day.slice(5)),
        datasets: [
          {
            label: t('阅读量'),
            data: (h.series || []).map((s) => s.count),
            backgroundColor: 'rgba(46,168,122,.72)',
            borderRadius: 3
          }
        ]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: { y: { beginAtZero: true, ticks: { precision: 0 } } },
        plugins: { legend: { display: false } }
      }
    })
  } catch (_) {
    /* 同上 */
  }
}

onMounted(loadAll)
onUnmounted(() => {
  chart?.destroy()
  heatChart?.destroy()
})
</script>

<style scoped>
.an-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.an-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; margin-bottom: 14px; }
.an-head h2 { font-size: 20px; margin: 0 0 4px; }
.an-sub { color: var(--text-3, #8a919f); font-size: 13px; margin: 0; max-width: 760px; line-height: 1.6; }
.an-tools { display: inline-flex; gap: 8px; align-items: center; flex-shrink: 0; }
.an-days { height: 34px; padding: 0 10px; border-radius: var(--radius-sm, 8px); border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 13px; outline: none; }

.an-kpis { display: flex; gap: 10px; flex-wrap: wrap; margin-bottom: 14px; }
.an-kpi { flex: 1 1 150px; display: flex; flex-direction: column; gap: 2px; padding: 12px 14px; border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--surface-2, #f7f8fa); }
.an-kpi-n { font-size: 20px; font-weight: 700; color: var(--text, #20242c); }
.an-kpi-l { font-size: 11.5px; color: var(--text-3, #8a919f); }
.an-kpi-last .an-kpi-n { color: var(--primary, #2f6bff); }

.an-card { border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--bg-2, #fff); margin-bottom: 14px; }
.an-card-head { display: flex; align-items: baseline; gap: 8px; padding: 12px 14px; border-bottom: 1px solid var(--border, #ececf1); }
.an-card-head b { font-size: 14px; }
.an-hint { font-size: 11.5px; color: var(--text-3, #8a919f); }
.an-chart-wrap { position: relative; height: 260px; padding: 12px 14px 16px; }
.an-chart-sm { height: 200px; }
.an-chart-empty { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; color: var(--text-3, #8a919f); font-size: 13px; text-align: center; padding: 0 24px; }
.an-chart-empty p { margin: 0; }

.an-main { display: flex; gap: 14px; align-items: flex-start; }
.an-top { flex: 0 0 400px; margin-bottom: 0; max-height: 520px; overflow-y: auto; }
.an-detail { flex: 1; min-width: 0; margin-bottom: 0; }

.an-row { display: flex; align-items: center; gap: 10px; padding: 9px 14px; border-bottom: 1px solid var(--border, #f2f3f7); cursor: pointer; transition: background .12s; }
.an-row:last-child { border-bottom: 0; }
.an-row:hover { background: var(--surface-2, #f7f8fa); }
.an-row.sel { background: var(--primary-soft, #eaf1ff); }
.an-rank { flex-shrink: 0; width: 20px; text-align: center; font-size: 12px; font-weight: 600; color: var(--text-3, #8a919f); }
.an-rank.top3 { color: var(--primary, #2f6bff); }
.an-row-main { flex: 1; min-width: 0; }
.an-row-name { font-size: 13px; color: var(--text, #20242c); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.an-bar { height: 4px; margin-top: 5px; border-radius: 3px; background: var(--bg-3, #f0f1f4); overflow: hidden; }
.an-bar i { display: block; height: 100%; border-radius: 3px; background: var(--primary, #2f6bff); }
.an-row-nums { flex-shrink: 0; display: flex; flex-direction: column; align-items: flex-end; }
.an-row-nums b { font-size: 14px; color: var(--text, #20242c); }
.an-row-nums span { font-size: 11px; color: var(--text-3, #8a919f); }

.an-empty { padding: 40px 14px; text-align: center; color: var(--text-3, #8a919f); font-size: 13px; }
</style>
