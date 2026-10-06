<template>
  <div class="kg-root">
    <header class="kg-head">
      <h2 class="kg-title">{{  t('kg.title')  }}</h2>
      <div class="kg-actions">
        <span class="kg-stat">{{  t('kg.nodes')  }}: {{  nodes.length  }} · {{  t('kg.links')  }}: {{  links.length  }}</span>
        <button class="kg-btn" :disabled="loading" @click="load">{{  loading ? t('kg.loading') : t('kg.refresh')  }}</button>
      </div>
    </header>
    <p v-if="err" class="kg-err">{{  err  }}</p>
    <p v-if="!loading && !nodes.length" class="kg-empty">{{  t('kg.empty')  }}</p>
    <div v-else ref="el" class="kg-chart"></div>
  </div>
</template>

<script setup>
import { onMounted, onUnmounted, ref, shallowRef, nextTick } from 'vue'
import { t } from '@/i18n'
import { requestJSON } from '@/api'
import * as echarts from 'echarts/core'
import { GraphChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

// echarts 按需引入（只注册图谱所需，避免整包）
echarts.use([GraphChart, TooltipComponent, LegendComponent, CanvasRenderer])

const el = ref(null)
const chart = shallowRef(null)
const nodes = ref([])
const links = ref([])
const loading = ref(false)
const err = ref('')

function render() {
  if (!el.value) return
  if (chart.value) chart.value.dispose()
  chart.value = echarts.init(el.value)
  const catIndex = { file: 0, tag: 1 }
  chart.value.setOption({
    tooltip: { formatter: (p) => (p.dataType === 'node' ? p.data.name : '') },
    legend: [{ data: [t('kg.file'), t('kg.tag')], top: 0 }],
    series: [{
      type: 'graph',
      layout: 'force',
      roam: true,
      draggable: true,
      force: { repulsion: 220, edgeLength: 90, gravity: 0.08 },
      categories: [
        { name: t('kg.file'), itemStyle: { color: '#2f6feb' } },
        { name: t('kg.tag'), itemStyle: { color: '#1a7f5a' } },
      ],
      label: { show: true, position: 'right', fontSize: 11, formatter: '{b}' },
      lineStyle: { color: 'source', curveness: 0.12, opacity: 0.5 },
      emphasis: { focus: 'adjacency', lineStyle: { width: 3, opacity: 0.9 } },
      data: nodes.value.map((n) => ({
        id: n.id,
        name: n.label,
        category: catIndex[n.type] ?? 0,
        symbolSize: Math.min(46, 12 + (n.size || 1) * 2),
        value: n.size,
      })),
      links: links.value.map((l) => ({ source: l.source, target: l.target })),
    }],
  })
}

async function load() {
  loading.value = true
  err.value = ''
  try {
    const r = await requestJSON('/kb/graph')
    nodes.value = (r && r.nodes) || []
    links.value = (r && r.links) || []
    await nextTick()
    render()
  } catch (e) {
    err.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}

function onResize() { if (chart.value) chart.value.resize() }

onMounted(() => {
  load()
  window.addEventListener('resize', onResize)
})
onUnmounted(() => {
  window.removeEventListener('resize', onResize)
  if (chart.value) { chart.value.dispose(); chart.value = null }
})
</script>

<style scoped>
.kg-root { display: flex; flex-direction: column; gap: 10px; height: 100%; }
.kg-head { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 10px; }
.kg-title { font-size: 18px; font-weight: 600; margin: 0; }
.kg-actions { display: flex; align-items: center; gap: 10px; }
.kg-stat { font-size: 12px; color: var(--ink-3, #8b949e); }
.kg-btn { border: 1px solid var(--border, #e3e6eb); background: var(--surface, #fff); color: var(--ink, #1f2328); border-radius: 8px; padding: 6px 12px; font-size: 13px; cursor: pointer; }
.kg-btn:disabled { opacity: .6; cursor: not-allowed; }
.kg-err { color: #c0392b; font-size: 13px; margin: 0; }
.kg-empty { color: var(--ink-3, #8b949e); font-size: 13px; padding: 24px; text-align: center; }
.kg-chart { flex: 1; min-height: 420px; width: 100%; }
</style>
