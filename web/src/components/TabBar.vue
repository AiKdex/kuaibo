<template>
  <div class="tab-bar" :class="{ empty: tabs.length === 0 }">
    <div v-if="tabs.length === 0" class="tab-empty">{{  $t('打开文档后在此显示标签页 · 可同时浏览多个文件')  }}</div>
    <div
      v-for="t in tabs"
      :key="t.fileId"
      class="tab"
      :class="{ active: t.fileId === activeId }"
      @click="$emit('activate', t.fileId)"
      @mousedown.middle.prevent="$emit('close', t.fileId)"
      :title="t.name"
    >
      <AikIcon :name="tabIcon(t)" :size="13" class="tab-ic" />
      <span class="tab-name">{{  t.name  }}</span>
      <span v-if="isDirty(t.fileId)" class="tab-dirty" :title="$t('有未保存的修改')"></span>
      <button
        class="tab-x"
        :title="$t('关闭标签')"
        @click.stop="$emit('close', t.fileId)"
      ><AikIcon name="close" :size="12" /></button>
    </div>
    <div class="tab-spacer"></div>
    <div v-if="tabs.length > 1" class="tab-actions">
      <button class="tab-act" :title="$t('关闭其他标签')" @click="$emit('close-others', activeId)">
        <AikIcon name="close" :size="12" /> {{  $t('其他')  }}
      </button>
      <button class="tab-act" :title="$t('关闭全部标签')" @click="$emit('close-all')">
        <AikIcon name="close" :size="12" /> {{  $t('全部')  }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import AikIcon from '@/components/AikIcon.vue'

const props = defineProps({
  tabs: { type: Array, default: () => [] },
  activeId: { type: String, default: '' },
  dirtyMap: { type: Object, default: () => ({}) }
})
defineEmits(['activate', 'close', 'close-others', 'close-all'])

const dirtySet = computed(() => new Set(Object.keys(props.dirtyMap || {}).filter(k => props.dirtyMap[k])))
function isDirty(fileId) {
  return dirtySet.value.has(fileId)
}

const KIND_ICON = {
  image: 'fileImage', video: 'fileVideo', audio: 'fileAudio', text: 'fileText', pdf: 'filePdf', dir: 'folder', other: 'file'
}
function tabIcon(t) {
  return KIND_ICON[t.kind] || 'file'
}
</script>

<style scoped>
.tab-bar {
  display: flex;
  align-items: stretch;
  height: var(--tabbar-h, 38px);
  background: var(--surface);
  border-bottom: 1px solid var(--border);
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
  user-select: none;
  flex-shrink: 0;
}
.tab-bar.empty {
  display: none;
}
.tab-empty {
  align-self: center;
  padding: 0 14px;
  font-size: var(--fs-micro);
  color: var(--text-3);
}
.tab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px 0 12px;
  min-width: 0;
  max-width: 220px;
  border-right: 1px solid var(--border);
  color: var(--text-2);
  font-size: var(--fs-small);
  cursor: pointer;
  position: relative;
  background: var(--surface);
  transition: background 0.12s ease, color 0.12s ease;
  flex-shrink: 0;
}
.tab:hover {
  background: var(--bg-hover, rgba(0, 0, 0, 0.03));
}
.tab.active {
  background: var(--bg);
  color: var(--text);
  font-weight: 600;
  box-shadow: inset 0 2px 0 var(--primary);
}
.tab.active::after {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(to bottom, rgba(127, 127, 213, 0.05), transparent 60%);
}
.tab-ic {
  color: var(--text-3);
  flex-shrink: 0;
}
.tab.active .tab-ic {
  color: var(--primary);
}
.tab-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.tab-dirty {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--primary);
  flex-shrink: 0;
  margin-left: 2px;
}
.tab-x {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: none;
  border-radius: 5px;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  flex-shrink: 0;
  padding: 0;
}
.tab-x:hover {
  background: var(--danger-soft, rgba(220, 38, 38, 0.12));
  color: var(--danger);
}
.tab-spacer {
  flex: 1;
}
.tab-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 0 8px;
  flex-shrink: 0;
}
.tab-act {
  display: flex;
  align-items: center;
  gap: 3px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: transparent;
  color: var(--text-3);
  font-size: var(--fs-micro);
  padding: 2px 8px;
  cursor: pointer;
  white-space: nowrap;
}
.tab-act:hover {
  color: var(--text-2);
  border-color: var(--primary);
}
</style>
