<template>
  <div class="row-menu-pop" :style="pos" @click.self="emit('close')">
    <button class="menu-item" @click="emit('open')">
      <AikIcon name="eye" :size="14" /><span>{{  f.kind === 'dir' ? $t('打开') : $t('打开阅读')  }}</span>
    </button>
    <button class="menu-item" @click="emit('download')" :disabled="f.kind === 'dir'">
      <AikIcon name="download" :size="14" /><span>{{  $t('下载')  }}</span>
    </button>
    <div class="menu-sep"></div>
    <button class="menu-item" @click="emit('copy')">
      <AikIcon name="copy" :size="14" /><span>{{  $t('复制')  }}</span>
    </button>
    <button class="menu-item" @click="emit('cut')">
      <AikIcon name="cut" :size="14" /><span>{{  $t('剪切')  }}</span>
    </button>
    <button class="menu-item" @click="emit('rename')">
      <AikIcon name="edit" :size="14" /><span>{{  $t('重命名')  }}</span>
    </button>
    <button v-if="f.kind !== 'dir'" class="menu-item" @click="emit('todraft')">
      <AikIcon name="edit" :size="14" /><span>{{  $t('转为草稿')  }}</span>
    </button>
    <button v-if="f.kind !== 'dir'" class="menu-item" @click="emit('tags')">
      <AikIcon name="tag" :size="14" /><span>{{  $t('标签 / 分类')  }}</span>
    </button>
    <div class="menu-sep"></div>
    <button class="menu-item" @click="emit('share')">
      <AikIcon name="share" :size="14" /><span>{{  $t('分享 / 发布')  }}</span>
    </button>
    <button v-if="isImage" class="menu-item" @click="emit('ocr')">
      <AikIcon name="file" :size="14" /><span>{{  $t('OCR 识别文字')  }}</span>
    </button>
    <button class="menu-item danger" @click="emit('delete')">
      <AikIcon name="trash" :size="14" /><span>{{  $t('删除')  }}</span>
    </button>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import AikIcon from './AikIcon.vue'

const props = defineProps({
  f: { type: Object, required: true },
  pos: { type: Object, required: true }
})
const emit = defineEmits(['open', 'download', 'copy', 'cut', 'rename', 'todraft', 'tags', 'share', 'ocr', 'delete', 'close'])

const IMG = ['png', 'jpg', 'jpeg', 'webp', 'bmp', 'gif', 'avif']
const isImage = computed(() => {
  const ext = (props.f.name || '').split('.').pop().toLowerCase()
  return IMG.includes(ext)
})
</script>

<style scoped>
.row-menu-pop {
  position: fixed;
  min-width: 170px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-lg);
  padding: 5px;
  z-index: var(--z-modal);
  display: flex;
  flex-direction: column;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--text-2);
  cursor: pointer;
  transition: background 0.12s ease, color 0.12s ease;
  text-align: left;
  white-space: nowrap;
}

.menu-item:hover {
  background: var(--surface-2);
  color: var(--text);
}

.menu-item:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.menu-item.danger {
  color: var(--danger);
}

.menu-item.danger:hover {
  background: var(--danger-soft);
}

.menu-sep {
  height: 1px;
  background: var(--border);
  margin: 4px 6px;
}
</style>
