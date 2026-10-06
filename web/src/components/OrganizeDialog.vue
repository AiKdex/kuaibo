<template>
  <div class="organize-mask" @click.self="close">
    <div class="organize-dialog">
      <!-- 头部 -->
      <div class="og-head">
        <div class="og-title">
          <AikIcon name="sparkles" :size="18" />
          <span>{{  $t('AI 整理')  }}</span>
        </div>
        <button class="og-close icon-btn" @click="close" :title="$t('关闭')">
          <AikIcon name="close" :size="16" />
        </button>
      </div>

      <!-- 加载态 -->
      <div v-if="loading" class="og-body og-center">
        <div class="spinner"></div>
        <p>{{  $t('AI 正在阅读文件、检测冲突…')  }}</p>
        <p class="og-sub">{{  $t('摘要与归类建议由 AI 生成，请核对后应用')  }}</p>
      </div>

      <!-- 结果列表 -->
      <div v-else class="og-body">
        <div v-for="it in items" :key="it.file_id" class="og-item" :class="{ failed: it.error }">
          <!-- 失败 -->
          <div v-if="it.error" class="og-fail">
            <AikIcon name="info" :size="14" />
            <span>{{  it.name || it.file_id  }}：{{  it.error  }}</span>
          </div>

          <template v-else>
            <!-- 文件头 -->
            <div class="og-file">
              <AikIcon :name="it.kind === 'dir' ? 'folder' : 'fileText'" :size="15" />
              <span class="og-name" :title="it.name">{{  it.name  }}</span>
              <span v-if="!it.textable" class="og-nontext">{{  $t('非文本（仅冲突检测）')  }}</span>
              <button class="btn-link og-apply" :disabled="applying" @click="applyOne(it)">{{  $t('应用建议')  }}</button>
            </div>

            <!-- AI 摘要 -->
            <div v-if="it.summary" class="og-row">
              <span class="og-label">{{  $t('摘要')  }}</span>
              <p class="og-summary">{{  it.summary  }}</p>
            </div>

            <!-- 建议目录 / 标签 -->
            <div class="og-row og-grid">
              <div class="og-cell">
                <span class="og-label">{{  $t('建议目录')  }}</span>
                <div class="og-dirline">
                  <AikIcon name="folder" :size="13" />
                  <input
                    v-model="it.dir_path"
                    class="og-input"
                    :placeholder="$t('如 项目A/文档（不填不移动）')"
                    spellcheck="false"
                  />
                </div>
              </div>
              <div class="og-cell">
                <span class="og-label">{{  $t('建议标签')  }}</span>
                <div class="og-tags-edit">
                  <span
                    v-for="(t, ti) in it.tags"
                    :key="ti"
                    class="og-tag"
                  >
                    {{  t  }}
                    <button class="og-tag-x" @click="removeTag(it, ti)">×</button>
                  </span>
                  <input
                    v-model="it.tagInput"
                    class="og-input og-tag-input"
                    :placeholder="$t('+ 添加标签')"
                    @keydown.enter.prevent="addTag(it)"
                    @blur="addTag(it)"
                  />
                </div>
              </div>
            </div>

            <!-- 冲突列表 -->
            <div v-if="it.conflicts && it.conflicts.length" class="og-row">
              <span class="og-label">{{  $t('相似 / 冲突')  }}</span>
              <div class="og-conflicts">
                <button
                  v-for="c in it.conflicts"
                  :key="c.file_id"
                  class="og-conflict"
                  :class="c.reason"
                  :title="$t('{path}（相似度 {score}）', { path: c.path, score: c.score })"
                  @click="$emit('open', c.file_id)"
                >
                  <span v-if="c.reason === 'duplicate'" class="og-badge">{{  $t('疑似重复')  }}</span>
                  <span v-else class="og-badge og-badge-sim">{{  $t('同主题')  }}</span>
                  <span class="og-cname">{{  c.name  }}</span>
                  <span class="og-cscore">{{  Math.round(c.score * 100)  }}%</span>
                </button>
              </div>
              <p class="og-tip">{{  $t('点击冲突项可打开对照，避免重复归档')  }}</p>
            </div>
          </template>
        </div>
      </div>

      <!-- 底部 -->
      <div class="og-foot">
        <span class="og-foot-note">{{ $t('建议由 AI 生成，应用前可修改目录与标签') }}</span>
        <div class="og-foot-actions">
          <button class="btn" @click="close">{{ $t('取消') }}</button>
          <button class="btn btn-primary" :disabled="!canApply" @click="applyAll">
            <AikIcon name="sparkles" :size="14" />
            <span>{{ $t('应用全部（') }}{{  applyableCount  }}）</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import * as api from '@/api'
import { t } from '@/i18n'

const props = defineProps({
  fileIds: { type: Array, required: true }
})
const emit = defineEmits(['close', 'applied', 'open'])

const loading = ref(true)
const applying = ref(false)
const items = ref([])

const canApply = computed(() => items.value.some((it) => !it.error && it.textable))
const applyableCount = computed(() => items.value.filter((it) => !it.error && it.textable).length)

onMounted(async () => {
  try {
    items.value = (await api.organizeSuggest(props.fileIds)).map((it) => ({ ...it, tagInput: '' }))
  } catch (e) {
    items.value = [{ file_id: 'x', name: t('请求失败'), error: e.message || String(e) }]
  } finally {
    loading.value = false
  }
})

function addTag(it) {
  const t = (it.tagInput || '').trim()
  if (t && !it.tags.includes(t)) it.tags.push(t)
  it.tagInput = ''
}
function removeTag(it, i) {
  it.tags.splice(i, 1)
}

async function applyOne(it) {
  if (applying.value) return
  applying.value = true
  try {
    await api.organizeApply({
      file_id: it.file_id,
      dir_path: it.dir_path || '',
      tags: it.tags || []
    })
    it.applied = true
    emit('applied')
  } catch (e) {
    alertBox(e.message || String(e))
  } finally {
    applying.value = false
  }
}

async function applyAll() {
  if (applying.value) return
  applying.value = true
  let fail = 0
  for (const it of items.value) {
    if (it.error || !it.textable) continue
    try {
      await api.organizeApply({
        file_id: it.file_id,
        dir_path: it.dir_path || '',
        tags: it.tags || []
      })
      it.applied = true
    } catch (e) {
      fail++
    }
  }
  applying.value = false
  emit('applied')
  if (fail) {
    alertBox(`${fail} 项应用失败，请逐项重试`)
    return
  }
  close()
}

function alertBox(msg) {
  // 应用内轻提示：避免系统级弹窗
  emit('applied', { toast: msg })
}

function close() {
  emit('close')
}
</script>

<style scoped>
.organize-mask {
  position: fixed;
  inset: 0;
  background: rgba(10, 12, 18, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 500;
  padding: 24px;
}
.organize-dialog {
  width: min(720px, 100%);
  max-height: 86vh;
  display: flex;
  flex-direction: column;
  background: #fff;
  border-radius: 14px;
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.22);
  overflow: hidden;
}
.og-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid #eef0f4;
}
.og-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
  color: #1a1b1c;
}
.og-close { border: none; background: none; cursor: pointer; color: #6b7280; }
.og-body {
  flex: 1;
  overflow-y: auto;
  padding: 14px 18px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.og-center { align-items: center; justify-content: center; padding: 48px 18px; color: #6b7280; font-size: 13px; }
.og-sub { margin-top: 4px; font-size: 12px; color: #9ca3af; }
.og-item {
  border: 1px solid #eef0f4;
  border-radius: 10px;
  padding: 12px 14px;
  background: #fafbfc;
}
.og-fail { display: flex; align-items: center; gap: 6px; color: #d4380d; font-size: 13px; }
.og-file {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.og-name { font-weight: 600; font-size: 13.5px; color: #1a1b1c; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 60%; }
.og-nontext { font-size: 11px; color: #9ca3af; background: #f1f2f4; padding: 2px 8px; border-radius: 10px; }
.og-apply { margin-left: auto; font-size: 12.5px; }
.og-row { margin-top: 6px; }
.og-label { display: block; font-size: 11px; color: #9ca3af; margin-bottom: 4px; }
.og-summary { margin: 0; font-size: 12.5px; color: #374151; line-height: 1.55; }
.og-grid { display: flex; gap: 14px; flex-wrap: wrap; }
.og-cell { flex: 1 1 280px; min-width: 0; }
.og-dirline { display: flex; align-items: center; gap: 6px; }
.og-input {
  flex: 1;
  border: 1px solid #e2e6ec;
  border-radius: 6px;
  padding: 5px 8px;
  font-size: 12.5px;
  color: #1a1b1c;
  background: #fff;
  outline: none;
  min-width: 0;
}
.og-input:focus { border-color: #6b8afd; }
.og-tags-edit { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
.og-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: #eef2ff;
  color: #3b5bdb;
  font-size: 12px;
  padding: 3px 10px;
  border-radius: 12px;
}
.og-tag-x { border: none; background: none; color: #8a9cf5; cursor: pointer; font-size: 13px; padding: 0 2px; line-height: 1; }
.og-tag-input { flex: 0 1 120px; padding: 3px 8px; }
.og-conflicts { display: flex; flex-direction: column; gap: 6px; }
.og-conflict {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid #fde3cf;
  background: #fff8f4;
  border-radius: 8px;
  padding: 6px 10px;
  cursor: pointer;
  font-size: 12.5px;
  color: #374151;
  text-align: left;
}
.og-conflict.similar { border-color: #e0e7ff; background: #f6f8ff; }
.og-badge { font-size: 10.5px; background: #ff4d4f; color: #fff; padding: 1px 7px; border-radius: 8px; flex: none; }
.og-badge-sim { background: #5b7cfa; }
.og-cname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.og-cscore { margin-left: auto; color: #9ca3af; font-size: 11.5px; flex: none; }
.og-tip { margin: 4px 0 0; font-size: 11px; color: #b0b6c0; }
.og-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 12px 18px;
  border-top: 1px solid #eef0f4;
}
.og-foot-note { font-size: 11.5px; color: #9ca3af; }
.og-foot-actions { display: flex; gap: 8px; }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
