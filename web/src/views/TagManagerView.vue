<template>
  <div class="tm-view">
    <div class="tm-head">
      <div>
        <h2>{{  $t('标签管理')  }}</h2>
        <p class="tm-sub">{{  $t('统一树状标签 = 文件分类 / 知识库概念 / 博客分类 的同一套体系')  }}</p>
      </div>
      <div class="tm-tools">
        <button class="btn" @click="load" :disabled="loading">
          <AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}
        </button>
      </div>
    </div>

    <div class="tm-main">
      <!-- 左栏：标签树 -->
      <div class="tm-tree-pane">
        <div class="tm-new">
          <input v-model="newRoot" class="input" :placeholder="$t('新建顶层标签…（如：工作 / 学习 / 音乐）')" @keyup.enter="createRoot" />
          <button class="btn btn-primary" :disabled="!newRoot.trim()" @click="createRoot">
            <AikIcon name="plus" :size="14" />{{  $t('新建')  }}
          </button>
        </div>

        <div v-if="loading" class="tm-empty">{{  $t('加载中…')  }}</div>
        <div v-else-if="!tags.length" class="tm-empty">
          <AikIcon name="tag" :size="30" />
          <p>{{  $t('还没有标签')  }}</p>
          <p class="muted">{{  $t('在上方新建一个顶层标签；右键文件 →「标签/分类」→ 可新建子标签')  }}</p>
        </div>

        <div v-else class="tm-cascade">
          <div v-for="(col, ci) in columns" :key="ci" class="tm-col">
            <div v-for="t in col" :key="t.id" class="tm-col-item"
              :class="{ sel: selPath[ci] && selPath[ci].id === t.id }"
              :title="$t('{path}（{count} 个文件）', { path: t.path, count: t.count })" @click="selectLevel(ci, t)">
              <AikIcon name="tag" :size="13" class="tm-ico" />
              <span class="tm-col-name">{{  t.name  }}</span>
              <span class="tm-cnt">{{  t.count  }}</span>
              <AikIcon v-if="hasChildren(t.id)" name="chevron-right" :size="12" class="tm-col-arrow" />
            </div>
            <div v-if="!col.length" class="tm-col-empty">{{  $t('（无）')  }}</div>
          </div>
        </div>
      </div>

      <!-- 右栏：选中标签详情 -->
      <div class="tm-detail-pane">
        <template v-if="filterTag">
          <div class="tm-detail-head">
            <div class="tm-detail-path">
              <AikIcon name="tag" :size="15" class="tm-ico" />
              <b>{{  filterTag.path  }}</b>
              <span class="tm-cnt">{{  files.length  }} {{  $t('个文件')  }}</span>
            </div>
            <div class="tm-detail-ops">
              <button class="btn btn-sm" :title="$t('在该标签下新建子标签')" @click="startChild(filterTag)"><AikIcon name="plus" :size="13" />{{  $t('子标签')  }}</button>
              <button class="btn btn-sm" :title="$t('重命名')" @click="startRename(filterTag)"><AikIcon name="edit" :size="13" />{{  $t('重命名')  }}</button>
              <button class="btn btn-sm btn-danger-ghost" :title="$t('删除（连同子标签）')" @click="confirmRemoveTag = filterTag"><AikIcon name="trash" :size="13" />{{  $t('删除')  }}</button>
            </div>
          </div>
          <!-- 行内新建子标签 -->
          <div v-if="childOf === filterTag.id" class="tm-inline" @click.stop>
            <input v-model="childName" class="input tm-inline-input" :placeholder="$t('子标签名…')" @keyup.enter="createChild(filterTag)" @keyup.esc="childOf = ''" />
            <button class="btn btn-sm" :disabled="!childName.trim()" @click="createChild(filterTag)">{{  $t('确定')  }}</button>
            <button class="btn btn-sm btn-ghost" @click="childOf = ''">{{  $t('取消')  }}</button>
          </div>
          <!-- 行内重命名 -->
          <div v-if="renameOf === filterTag.id" class="tm-inline" @click.stop>
            <input v-model="renameName" class="input tm-inline-input" :placeholder="$t('新名称…')" @keyup.enter="doRename(filterTag)" @keyup.esc="renameOf = ''" />
            <button class="btn btn-sm" :disabled="!renameName.trim()" @click="doRename(filterTag)">{{  $t('确定')  }}</button>
            <button class="btn btn-sm btn-ghost" @click="renameOf = ''">{{  $t('取消')  }}</button>
          </div>
          <div v-if="filesLoading" class="tm-empty-sm">{{  $t('加载中…')  }}</div>
          <div v-else-if="!files.length" class="tm-detail-empty-sm">
            {{  $t('该标签下暂无文件 —— 在文件库选中文件 →「标签/分类」打上这个标签')  }}
          </div>
          <div v-else class="tm-files">
            <div v-for="f in files" :key="f.id" class="tm-file" @click="openFile(f)">
              <AikIcon :name="f.kind === 'dir' ? 'folder' : 'file'" :size="14" class="tm-file-ico" />
              <span class="tm-file-name">{{  f.name  }}</span>
              <span class="tm-file-meta">{{  f.kind === 'dir' ? $t('目录') : fmtSize(f.size)  }}</span>
            </div>
          </div>
        </template>
        <div v-else class="tm-detail-empty">
          <AikIcon name="tag" :size="30" />
          <p>{{ $t('点选左侧标签') }}</p>
          <p class="muted">{{ $t('查看该标签下的文件，或进行 子标签 / 重命名 / 删除 操作') }}</p>
        </div>
      </div>
    </div>
    <ConfirmDialog
      v-if="confirmRemoveTag"
      :title="$t('删除标签')"
      :message="$t('删除标签「{v0}」？其下所有子标签一并删除，文件本身不受影响。', { v0: confirmRemoveTag.path })"
      confirm-text="删除"
      danger
      @confirm="doRemoveTag(confirmRemoveTag)"
      @cancel="confirmRemoveTag = null"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const router = useRouter()
const toast = useToastStore()

const tags = ref([])
const loading = ref(false)
const newRoot = ref('')
const childOf = ref('')
const childName = ref('')
const renameOf = ref('')
const renameName = ref('')
const filterTag = ref(null)
const files = ref([])
const filesLoading = ref(false)

// 级联列：selPath 保存每层选中的标签
const selPath = ref([])
const columns = computed(() => {
  const cols = []
  let parentId = ''
  for (let i = 0; i < selPath.value.length; i++) {
    cols.push(tags.value.filter((t) => (i === 0 ? !t.parent_id : t.parent_id === parentId)))
    parentId = selPath.value[i].id
  }
  // 末列：未选中时显示根标签；已选中时显示当前层的子级
  cols.push(tags.value.filter((t) => (selPath.value.length === 0 ? !t.parent_id : t.parent_id === parentId)))
  // 隐藏空尾列（无子级时不留"（无）"占位列），首列保留
  while (cols.length > 1 && cols[cols.length - 1].length === 0) cols.pop()
  return cols
})
function hasChildren(id) {
  return tags.value.some((t) => t.parent_id === id)
}
function selectLevel(ci, t) {
  const cur = selPath.value[ci]
  if (cur && cur.id === t.id) {
    // 点击已选中项 → 收缩该层
    selPath.value = selPath.value.slice(0, ci)
    const parent = selPath.value[ci - 1] || null
    if (parent) filterByTag(parent)
    else clearFilter()
    return
  }
  selPath.value = selPath.value.slice(0, ci).concat([t])
  filterByTag(t)
}

async function load() {
  loading.value = true
  try {
    tags.value = await api.listTags()
    // 默认选中第一个根标签：级联两列 + 右侧详情即刻饱满，避免"窄列+大片留白"
    if (tags.value.length && !selPath.value.length) {
      const root = tags.value.find((t) => !t.parent_id)
      if (root) selectLevel(0, root)
    }
  } catch (e) {
    toast.push(t('加载标签失败：') + (e.message || e))
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function createRoot() {
  const name = newRoot.value.trim()
  if (!name) return
  try {
    await api.createTag(name)
    newRoot.value = ''
    await load()
    toast.push(`已创建「${name}」`)
  } catch (e) {
    toast.push(t('创建失败：') + (e.message || e))
  }
}

function startChild(t) {
  childOf.value = childOf.value === t.id ? '' : t.id
  childName.value = ''
  renameOf.value = ''
}
async function createChild(t) {
  const name = childName.value.trim()
  if (!name) return
  try {
    await api.createTag(name, t.id)
    childOf.value = ''
    await load()
    toast.push(`已创建「${t.path}/${name}」`)
  } catch (e) {
    toast.push(t('创建失败：') + (e.message || e))
  }
}

function startRename(t) {
  renameOf.value = renameOf.value === t.id ? '' : t.id
  renameName.value = t.name
  childOf.value = ''
}
async function doRename(t) {
  const name = renameName.value.trim()
  if (!name || name === t.name) {
    renameOf.value = ''
    return
  }
  try {
    await api.renameTag(t.id, name)
    renameOf.value = ''
    await load()
    toast.push(`已重命名为「${name}」`)
  } catch (e) {
    toast.push(t('重命名失败：') + (e.message || e))
  }
}

const confirmRemoveTag = ref(null)

function doRemoveTag(t) {
  confirmRemoveTag.value = null
  api
    .deleteTag(t.id)
    .then(async () => {
      await load()
      if (filterTag.value && filterTag.value.id === t.id) {
        selPath.value = []
        clearFilter()
      }
      toast.push(`已删除「${t.path}」`)
    })
    .catch((e) => toast.push(t('删除失败：') + (e.message || e)))
}

async function filterByTag(t) {
  filterTag.value = t
  filesLoading.value = true
  try {
    files.value = await api.listFilesByTag(t.id)
  } catch (e) {
    toast.push(t('加载文件失败：') + (e.message || e))
  } finally {
    filesLoading.value = false
  }
}

function clearFilter() {
  filterTag.value = null
  files.value = []
}
function openFile(f) {
  if (f.kind === 'dir') router.push({ path: '/files', query: { parent: f.id } })
  else router.push('/read/' + f.id)
}

function fmtSize(n) {
  if (!n) return '—'
  if (n < 1024) return n + ' B'
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1048576).toFixed(1) + ' MB'
}
</script>

<style scoped>
.tm-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.tm-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 14px; }
.tm-head h2 { font-size: 20px; margin: 0 0 4px; }
.tm-sub { color: var(--text-3, #8a919f); font-size: 13px; margin: 0; }

.tm-main { display: flex; gap: 16px; align-items: flex-start; }
.tm-tree-pane { flex: 0 0 auto; max-width: 860px; min-width: 200px; }

.tm-new { display: flex; gap: 8px; margin-bottom: 12px; }
.tm-new input { flex: 1; }

.input {
  height: 34px;
  padding: 0 12px;
  border-radius: var(--radius-sm, 8px);
  border: 1px solid var(--border, #e3e6ee);
  background: var(--surface-2, #f7f8fa);
  color: var(--text, #20242c);
  font-size: 13px;
  outline: none;
  transition: border-color .15s, box-shadow .15s, background .15s;
}
.input::placeholder { color: var(--text-3, #a0a6b2); }
.input:focus {
  border-color: var(--primary, #2f6bff);
  background: var(--bg-1, #fff);
  box-shadow: 0 0 0 3px rgba(47, 107, 255, .12);
}

/* 级联多列：列等宽自适应（选中后自动撑满左栏，不留白） */
.tm-cascade { display: flex; align-items: stretch; background: var(--bg-2, #fff); border: 1px solid var(--border, #ececf1); border-radius: 10px; overflow: hidden; }
.tm-col { flex: 1 1 0; min-width: 170px; max-width: 270px; border-right: 1px solid var(--border, #ececf1); padding: 6px; max-height: calc(100vh - 240px); overflow-y: auto; }
.tm-col:last-child { border-right: none; }
.tm-col-item { display: flex; align-items: center; gap: 6px; padding: 7px 9px; border-radius: 7px; cursor: pointer; font-size: 13px; color: var(--text-1, #20242c); transition: background .12s; }
.tm-col-item:hover { background: var(--bg-3, #f5f6f8); }
.tm-col-item.sel { background: var(--primary-soft, #eaf1ff); color: var(--accent, #2f6bff); font-weight: 600; }
.tm-col-item .tm-ico { flex-shrink: 0; }
.tm-col-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tm-col-arrow { flex-shrink: 0; color: var(--text-3, #8a919f); }
.tm-col-empty { text-align: center; color: var(--text-3, #8a919f); font-size: 12px; padding: 18px 0; }

.tm-cnt { font-size: 11px; color: var(--text-3, #8a919f); background: var(--bg-3, #f5f6f8); padding: 1px 7px; border-radius: 8px; flex-shrink: 0; }
.tm-ico { color: var(--text-3, #8a919f); flex-shrink: 0; }
.tm-inline { display: flex; align-items: center; gap: 6px; padding: 10px 16px; border-bottom: 1px solid var(--border, #ececf1); }
.tm-inline-input { width: 200px; height: 30px; padding: 0 8px; border: 1px solid var(--border, #ececf1); border-radius: 6px; background: var(--surface-2, #f5f6f8); font-size: 12.5px; color: var(--text, #20242c); outline: none; }
.tm-inline-input:focus { border-color: var(--primary, #2f6bff); }

/* 右栏详情 */
.tm-detail-pane { flex: 1; min-width: 0; background: var(--bg-2, #fff); border: 1px solid var(--border, #ececf1); border-radius: 10px; min-height: 420px; display: flex; flex-direction: column; }
.tm-detail-head { display: flex; justify-content: space-between; align-items: center; gap: 10px; padding: 14px 16px; border-bottom: 1px solid var(--border, #ececf1); flex-wrap: wrap; }
.tm-detail-path { display: flex; align-items: center; gap: 8px; font-size: 15px; min-width: 0; }
.tm-detail-path b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tm-detail-ops { display: inline-flex; gap: 6px; }
.tm-detail-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; color: var(--text-3, #8a919f); padding: 40px 0; }
.tm-detail-empty p { margin: 0; font-size: 14px; }
.tm-detail-empty .muted { font-size: 12.5px; max-width: 320px; text-align: center; line-height: 1.7; }
.tm-detail-empty-sm { text-align: center; color: var(--text-3, #8a919f); padding: 30px 0; font-size: 12.5px; }
.tm-files { flex: 1; display: flex; flex-direction: column; gap: 4px; padding: 10px 14px; overflow-y: auto; max-height: calc(100vh - 320px); }
.tm-file { display: flex; align-items: center; gap: 8px; padding: 7px 8px; border-radius: 6px; cursor: pointer; }
.tm-file:hover { background: var(--bg-3, #f5f6f8); }
.tm-file-ico { color: var(--text-3, #8a919f); }
.tm-file-name { flex: 1; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tm-file-meta { font-size: 12px; color: var(--text-3, #8a919f); }
.tm-empty { text-align: center; color: var(--text-3, #8a919f); padding: 40px 0; font-size: 13px; }
.tm-empty-sm { text-align: center; color: var(--text-3, #8a919f); padding: 12px 0; font-size: 12px; }
.btn-ghost { background: none; }
.btn-danger-ghost { background: none; color: #e03e3e; border: 1px solid rgba(224, 62, 62, 0.35); }
.btn-danger-ghost:hover { background: rgba(224, 62, 62, 0.08); }
</style>
