<template>
  <div v-if="pv !== null" class="st-line">
    <span class="st-label">{{ $t('阅读') }}</span>
    <span class="st-n">{{ pv }}</span>
  </div>
</template>

<script setup>
import { ref, watch, onMounted } from 'vue'
import { publicBlogPV, publicBlogPVGet } from '@/api'

const props = defineProps({
  ctx: { type: Object, default: () => ({}) },
})

const pv = ref(null)

async function load() {
  const key = props.ctx?.path || props.ctx?.slug || props.ctx?.title || ''
  if (!key) return
  try {
    await publicBlogPV(key)
    const d = await publicBlogPVGet(key)
    pv.value = d.pv ?? 0
  } catch (_) {
    pv.value = null
  }
}

onMounted(load)
watch(() => props.ctx?.path, load)
</script>

<style scoped>
.st-line {
  margin-top: 8px;
  font-size: 12px;
  color: var(--text-3, #8a919f);
  font-variant-numeric: tabular-nums;
}
.st-label { margin-right: 4px; }
.st-n { color: var(--text-2, #4b5563); font-weight: 600; }
</style>
