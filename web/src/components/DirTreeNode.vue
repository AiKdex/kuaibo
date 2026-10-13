<template>
  <div class="tree-node">
    <div
      class="tree-row"
      :class="{ active: current === node.id, 'drop-over': isOver }"
      :style="{ paddingLeft: 8 + depth * 13 + 'px' }"
      data-kind="dir"
      :data-fid="node.id"
      @dragover="onOver($event)"
      @dragleave="onLeave"
      @drop="onDrop($event)"
    >
      <button v-if="(children[node.id] || []).length" class="tree-tw" @click="emit('toggle', node.id)">
        <svg class="aik-icon" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <polyline :points="expanded.has(node.id) ? '6 9 12 15 18 9' : '9 6 15 12 9 18'" />
        </svg>
      </button>
      <span v-else class="tree-tw-placeholder"></span>
      <span class="tree-label" :title="node.name" @click="emit('open', node.id)">{{ node.name }}</span>
    </div>
    <template v-if="expanded.has(node.id)">
      <DirTreeNode
        v-for="c in children[node.id] || []"
        :key="c.id"
        :node="c"
        :depth="depth + 1"
        :children="children"
        :expanded="expanded"
        :current="current"
        @toggle="emit('toggle', $event)"
        @open="emit('open', $event)"
        @move="emit('move', $event)"
      />
    </template>
  </div>
</template>

<script setup>
import { ref } from 'vue'
const props = defineProps({
  node: { type: Object, required: true },
  depth: { type: Number, required: true },
  children: { type: Object, required: true },
  expanded: { type: Set, required: true },
  current: { type: String, default: '' }
})
const emit = defineEmits(['toggle', 'open', 'move'])

// 拖拽移动：接受来自文件列表行的拖拽（dataTransfer 文本为文件 id）
const isOver = ref(false)
function onOver(e) {
  if (e.dataTransfer && Array.from(e.dataTransfer.types).includes('text/plain')) {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    isOver.value = true
  }
}
function onLeave() {
  isOver.value = false
}
function onDrop(e) {
  e.preventDefault()
  isOver.value = false
  const id = e.dataTransfer ? e.dataTransfer.getData('text/plain') : ''
  if (!id || id === props.node.id) return
  emit('move', { id, target: props.node.id })
}
</script>

<style scoped>
.tree-row {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 5px 10px 5px 8px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--sidebar-text);
  font-size: 12.5px;
  transition: background 0.14s ease, color 0.14s ease;
  white-space: nowrap;
}

.tree-row:hover {
  background: rgba(255, 255, 255, 0.06);
  color: #fff;
}

.tree-row.active {
  background: var(--sidebar-active);
  color: #fff;
}

/* 拖拽悬停目标：高亮 */
.tree-row.drop-over {
  background: rgba(79, 131, 255, 0.35);
  outline: 2px dashed rgba(129, 166, 255, 0.9);
  outline-offset: -2px;
  color: #fff;
}

.tree-tw {
  width: 16px;
  height: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--sidebar-text-2);
  flex-shrink: 0;
  cursor: pointer;
  border-radius: 4px;
  background: transparent;
  border: none;
  padding: 0;
}

.tree-tw:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.08);
}

.tree-tw-placeholder {
  width: 16px;
  flex-shrink: 0;
}

.tree-label {
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
}
</style>
