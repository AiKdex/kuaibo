<template>
  <aside class="vrt-card">
    <div class="vrt-row">
      <button class="vrt-btn" :disabled="busy || !hasSlug" @click="toggle">
        <span v-if="busy">{{ $t('合成中…') }}</span>
        <span v-else-if="playing">{{ $t('停止朗读') }}</span>
        <span v-else>{{ $t('🎧 朗读全文') }}</span>
      </button>
      <span v-if="truncated" class="vrt-note">{{ $t('正文较长，已朗读前 4000 字') }}</span>
    </div>
    <audio
      v-if="src"
      ref="audioEl"
      :src="src"
      controls
      class="vrt-audio"
      @ended="playing = false"
      @pause="playing = false"
    ></audio>
    <div v-if="error" class="vrt-error">{{ error }}</div>
  </aside>
</template>

<script setup>
import { t } from '@/i18n'
import { ref, computed, onBeforeUnmount, watch, nextTick } from 'vue'
import { publicBlogTTSBlob } from '@/api'

const props = defineProps({
  ctx: { type: Object, default: () => ({}) },
})

const slug = computed(() => props.ctx?.slug || '')
const hasSlug = computed(() => !!slug.value)
const busy = ref(false)
const playing = ref(false)
const error = ref('')
const truncated = ref(false)
const src = ref('')
const audioEl = ref(null)
let url = null

function revoke() {
  if (url) {
    URL.revokeObjectURL(url)
    url = null
  }
}

async function start() {
  busy.value = true
  error.value = ''
  truncated.value = false
  try {
    const d = await publicBlogTTSBlob(slug.value)
    revoke()
    url = URL.createObjectURL(d.blob)
    src.value = url
    truncated.value = d.truncated
    await nextTick()
    if (audioEl.value) {
      try {
        await audioEl.value.play()
      } catch (_) {
        /* 自动播放被浏览器拦截时，等待用户点击播放控件 */
      }
    }
    playing.value = true
  } catch (e) {
    error.value = e.message || t('朗读失败')
  } finally {
    busy.value = false
  }
}

function stop() {
  if (audioEl.value) audioEl.value.pause()
  playing.value = false
}

function toggle() {
  if (playing.value) stop()
  else start()
}

onBeforeUnmount(revoke)
// 同一挂载点组件复用于不同文章时，清理旧音频与状态
watch(slug, () => {
  revoke()
  src.value = ''
  playing.value = false
  error.value = ''
  truncated.value = false
})
</script>

<style scoped>
.vrt-card {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  background: var(--surface, #fff);
  padding: 14px 16px;
  font-size: 13px;
}
.vrt-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.vrt-btn {
  padding: 8px 16px;
  border-radius: 8px;
  border: none;
  background: #0d7a6a;
  color: #fff;
  cursor: pointer;
  font: inherit;
  white-space: nowrap;
}
.vrt-btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.vrt-note {
  color: var(--text-3, #8a919f);
  font-size: 12px;
}
.vrt-audio {
  display: block;
  width: 100%;
  margin-top: 12px;
}
.vrt-error {
  margin-top: 8px;
  color: #b91c1c;
  font-size: 12px;
}
</style>
