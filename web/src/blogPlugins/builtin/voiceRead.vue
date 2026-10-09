<template>
  <!-- 语音朗读插件（紧凑内联版）：挂载 post_meta（文章标题下元信息行） -->
  <span class="vrt-inline">
    <button class="vrt-btn-inline" :disabled="busy || !hasSlug" @click="toggle">
      <span v-if="busy">{{ $t('合成中…') }}</span>
      <span v-else-if="playing">{{ $t('⏹ 停止朗读') }}</span>
      <span v-else>{{ $t('🎧 朗读全文') }}</span>
    </button>
    <span v-if="truncated" class="vrt-note-inline">{{ $t('前4000字') }}</span>
    <div v-if="src" class="vrt-audio-wrap">
      <audio ref="audioEl" :src="src" controls class="vrt-audio-inline" @ended="playing = false" @pause="playing = false"></audio>
    </div>
    <div v-if="error" class="vrt-error-inline">{{ error }}</div>
  </span>
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
/* 元信息行内联样式：颜色继承主题元信息行，避免各主题配色冲突 */
.vrt-inline {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.vrt-btn-inline {
  display: inline-flex;
  align-items: center;
  padding: 1px 10px;
  border-radius: 999px;
  border: 1px solid currentColor;
  background: transparent;
  color: inherit;
  opacity: 0.85;
  cursor: pointer;
  font: inherit;
  font-size: inherit;
  line-height: 1.6;
  white-space: nowrap;
  transition: opacity 0.15s ease;
}
.vrt-btn-inline:hover {
  opacity: 1;
}
.vrt-btn-inline:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.vrt-note-inline {
  font-size: inherit;
  opacity: 0.65;
}
/* 播放时音频条出现在元信息行下方（整行换行展示，不挤元信息） */
.vrt-audio-wrap {
  flex-basis: 100%;
}
.vrt-audio-inline {
  display: block;
  width: 100%;
  max-width: 420px;
  height: 32px;
  margin-top: 4px;
}
.vrt-error-inline {
  flex-basis: 100%;
  color: #b91c1c;
  font-size: 12px;
}
</style>
