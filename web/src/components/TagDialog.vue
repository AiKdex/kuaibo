<template>
  <div class="modal-mask" @click.self="emit('close')">
    <div class="tag-dialog">
      <div class="td-head">
        <h3>{{  $t('标签 / 分类')  }}</h3>
        <button class="icon-btn" :title="$t('关闭')" @click="emit('close')"><AikIcon name="close" :size="16" /></button>
      </div>
      <div class="td-file" :title="fileLabel">{{  fileLabel  }}</div>

      <!-- 已选标签 -->
      <div v-if="selected.length" class="td-selected">
        <span v-for="t in selectedTags" :key="t.id" class="td-chip">
          {{  t.path  }}
          <button class="td-chip-x" :title="$t('移除')" @click="toggle(t)"><AikIcon name="close" :size="11" /></button>
        </span>
      </div>
      <div v-else class="td-hint">{{  $t('未打标签——点下方标签选择，或新建一个')  }}</div>

      <!-- 标签树（缩进显示层级） -->
      <div class="td-list">
        <div v-if="!allTags.length" class="td-empty">{{  $t('还没有标签，先新建一个（如：音乐、工作、项目A）')  }}</div>
        <button
          v-for="t in allTags"
          :key="t.id"
          class="td-tag"
          :class="{ on: selected.includes(t.id) }"
          :style="{ paddingLeft: (8 + depth(t) * 16) + 'px' }"
          @click="toggle(t)"
        >
          <AikIcon name="check" :size="12" class="td-check" />
          <span class="td-path">{{  t.path  }}</span>
          <span class="td-cnt">{{  t.count  }}</span>
        </button>
      </div>

      <!-- 新建标签 -->
      <div class="td-new">
        <input
          v-model="newName"
          :placeholder="$t('新标签名（如：粤语）')"
          @keydown.enter="createTag"
        />
        <button class="btn btn-sm" :disabled="!newName.trim()" @click="createTag">{{  $t('新建')  }}</button>
      </div>

      <div class="td-foot">
        <button class="btn" @click="emit('close')">{{  $t('取消')  }}</button>
        <button class="btn btn-primary" :disabled="saving" @click="save">{{  $t('保存')  }}</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import AikIcon from './AikIcon.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const props = defineProps({
  file: { type: Object, required: false, default: null },
  // 批量模式：传 files 数组时对多个文件统一设置所选标签（孤儿文件无已有标签，覆盖无副作用）
  files: { type: Array, required: false, default: () => [] }
})
const emit = defineEmits(['close', 'saved'])
const toast = useToastStore()

const allTags = ref([])
const selected = ref([])
const newName = ref('')
const saving = ref(false)

const selectedTags = computed(() =>
  selected.value
    .map((id) => allTags.value.find((t) => t.id === id))
    .filter(Boolean)
)

const isBatch = computed(() => props.files && props.files.length > 0)
const fileLabel = computed(() => {
  if (isBatch.value) return `已选 ${props.files.length} 个文件（统一设置所选标签）`
  return props.file ? props.file.name : ''
})

async function load() {
  try {
    const tags = await api.listTags()
    allTags.value = tags
    if (isBatch.value) {
      selected.value = [] // 批量模式不载入单个文件已有标签
    } else if (props.file) {
      const ftags = await api.getFileTags(props.file.id)
      selected.value = (ftags || []).map((t) => t.id)
    }
  } catch (e) {
    toast.push(t('加载标签失败：') + (e.message || e))
  }
}
onMounted(load)

function depth(t) {
  return t.path ? t.path.split('/').length - 1 : 0
}

function toggle(t) {
  const i = selected.value.indexOf(t.id)
  if (i >= 0) selected.value.splice(i, 1)
  else selected.value.push(t.id)
}

async function createTag() {
  const name = newName.value.trim()
  if (!name) return
  try {
    const t = await api.createTag(name)
    allTags.value.push(t)
    selected.value.push(t.id)
    newName.value = ''
  } catch (e) {
    toast.push(t('新建失败：') + (e.message || e))
  }
}

async function save() {
  saving.value = true
  try {
    if (isBatch.value) {
      for (const f of props.files) {
        await api.setFileTags(f.id, selected.value)
      }
      toast.push(`已给 ${props.files.length} 个文件打上所选标签`)
    } else if (props.file) {
      await api.setFileTags(props.file.id, selected.value)
      toast.push(t('标签已保存'))
    }
    emit('saved')
    emit('close')
  } catch (e) {
    toast.push(t('保存失败：') + (e.message || e))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.42);
  z-index: var(--z-modal);
  display: flex;
  align-items: center;
  justify-content: center;
}
.tag-dialog {
  width: 460px;
  max-width: calc(100vw - 40px);
  max-height: 82vh;
  display: flex;
  flex-direction: column;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-lg);
  padding: 16px;
}
.td-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}
.td-head h3 {
  font-size: 15px;
  font-weight: 600;
  color: var(--text);
}
.td-file {
  font-size: 12px;
  color: var(--text-2);
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 7px 10px;
  margin-bottom: 10px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.td-selected {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 10px;
}
.td-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: var(--primary-soft);
  color: var(--primary);
  font-size: 12px;
  border-radius: 999px;
  padding: 3px 9px;
}
.td-chip-x {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  opacity: 0.7;
}
.td-chip-x:hover {
  opacity: 1;
}
.td-hint {
  font-size: 12px;
  color: var(--text-3);
  margin-bottom: 10px;
}
.td-list {
  flex: 1;
  overflow-y: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 6px;
  margin-bottom: 10px;
  min-height: 120px;
  max-height: 300px;
}
.td-empty {
  font-size: 12.5px;
  color: var(--text-3);
  padding: 16px 10px;
  text-align: center;
}
.td-tag {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  padding: 7px 10px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--text-2);
  cursor: pointer;
  text-align: left;
  transition: background 0.12s ease;
}
.td-tag:hover {
  background: var(--surface-2);
}
.td-tag.on {
  background: var(--primary-soft);
  color: var(--primary);
}
.td-check {
  flex-shrink: 0;
  visibility: hidden;
}
.td-tag.on .td-check {
  visibility: visible;
}
.td-path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.td-cnt {
  font-size: 11px;
  color: var(--text-3);
  flex-shrink: 0;
}
.td-new {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
.td-new input {
  flex: 1;
  height: 32px;
  padding: 0 10px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: var(--surface-2);
  color: var(--text);
  font-size: 13px;
  outline: none;
}
.td-new input:focus {
  border-color: var(--primary);
}
.td-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
