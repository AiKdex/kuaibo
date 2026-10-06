<template>
  <div class="wd-view">
    <div class="wd-head">
      <div>
        <h2>{{  $t('外部挂载（WebDAV）')  }}</h2>
        <p class="wd-sub">
          {{  $t('把远端 WebDAV 服务挂进来浏览，并把文件导入知识库（目录导入取递归第一层文件）。 本机也可被外部挂载：服务端提供')  }} <code>/api/v1/dav/</code> {{  $t('端点（需开启')  }} <code>dav.enabled</code>）。
        </p>
      </div>
      <div class="wd-tools">
        <button class="btn" :disabled="loading" @click="loadMounts"><AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}</button>
        <button class="btn btn-primary" @click="openCreate"><AikIcon name="plus" :size="14" />{{  $t('新建挂载')  }}</button>
      </div>
    </div>

    <div class="wd-main">
      <!-- 左：挂载列表 -->
      <div class="wd-list">
        <div v-if="loading" class="wd-empty">{{  $t('加载中…')  }}</div>
        <div v-else-if="!mounts.length" class="wd-empty">
          <AikIcon name="database" :size="26" />
          <p>{{  $t('还没有挂载')  }}</p>
        </div>
        <div
          v-for="m in mounts"
          :key="m.id"
          class="wd-card"
          :class="{ sel: cur && cur.id === m.id }"
          @click="selectMount(m)"
        >
          <div class="wd-card-top">
            <b class="wd-card-name">{{  m.name  }}</b>
            <span v-if="m.has_pass" class="wd-badge"><AikIcon name="lock" :size="11" />{{  $t('已设密码')  }}</span>
          </div>
          <div class="wd-card-url" :title="m.url">{{  m.url  }}</div>
          <div class="wd-card-ops">
            <button class="btn btn-sm" :disabled="testing === m.id" @click.stop="doTest(m)">
              <AikIcon name="link" :size="13" />{{  testing === m.id ? $t('测试中…') : $t('测试')  }}
            </button>
            <button class="btn btn-sm" @click.stop="openEdit(m)"><AikIcon name="edit" :size="13" />{{  $t('编辑')  }}</button>
            <button class="btn btn-sm btn-danger-ghost" @click.stop="confirmDel = m"><AikIcon name="trash" :size="13" />{{  $t('删除')  }}</button>
          </div>
        </div>
      </div>

      <!-- 右：远端浏览 -->
      <div class="wd-browse">
        <template v-if="cur">
          <div class="wd-browse-head">
            <div class="wd-crumbs">
              <a @click="openDir('')">{{  $t('根目录')  }}</a>
              <template v-for="(seg, i) in crumbs" :key="i">
                <AikIcon name="chevronRight" :size="12" />
                <a @click="openDir(pathUpTo(i))">{{  seg  }}</a>
              </template>
            </div>
            <div class="wd-browse-tools">
              <select v-model="importParent" class="wd-pick" :title="$t('导入到知识库的目录')">
                <option value="">{{ $t('导入到：根目录') }}</option>
                <option v-for="d in dirOptions" :key="d.id" :value="d.id">{{ $t('导入到：') }}{{  d.label  }}</option>
              </select>
              <button class="btn btn-sm" :disabled="browsing" @click="loadDir(curPath)">
                <AikIcon name="refresh" :size="13" />{{ $t('刷新') }}
              </button>
            </div>
          </div>

          <div v-if="browsing" class="wd-empty">{{ $t('读取远端目录…') }}</div>
          <div v-else-if="!entries.length" class="wd-empty-sm">{{ $t('该目录为空（或不可列出）') }}</div>
          <div v-else class="wd-rows">
            <div v-for="e in entries" :key="e.path" class="wd-row">
              <span class="wd-row-ico">
                <AikIcon :name="e.type === 'directory' ? 'folder' : 'file'" :size="15" />
              </span>
              <div class="wd-row-main">
                <a v-if="e.type === 'directory'" class="wd-row-name" @click="openDir(e.path)">{{  e.name  }}</a>
                <span v-else class="wd-row-name plain">{{  e.name  }}</span>
                <span class="wd-row-meta">
                  <span v-if="e.type === 'file'">{{  e.size_h || fmtSize(e.size)  }}</span>
                  <span v-if="e.mtime">{{  e.mtime  }}</span>
                </span>
              </div>
              <button
                v-if="e.type === 'file' && e.text !== false"
                class="btn btn-sm"
                :disabled="importing === e.path"
                @click="doImport(e.path)"
              ><AikIcon name="download" :size="13" />{{  importing === e.path ? $t('导入中…') : $t('导入')  }}</button>
              <button
                v-else-if="e.type === 'file'"
                class="btn btn-sm"
                disabled
                :title="$t('非文本文件不支持导入')"
              ><AikIcon name="download" :size="13" />{{ $t('导入') }}</button>
            </div>
          </div>

          <div class="wd-browse-foot">
            <button class="btn btn-sm" :disabled="importing === '__dir__'" @click="doImport(curPath)">
              <AikIcon name="download" :size="13" />
              {{  importing === '__dir__' ? $t('导入中…') : $t('导入当前目录（递归第一层）')  }}
            </button>
          </div>
        </template>
        <div v-else class="wd-empty">
          <AikIcon name="database" :size="26" />
          <p>{{ $t('选择左侧挂载开始浏览') }}</p>
        </div>
      </div>
    </div>

    <!-- 新建 / 编辑 -->
    <div v-if="formOpen" class="modal-mask" @click.self="formOpen = false">
      <div class="modal-box wd-modal">
        <h3 class="wd-modal-title">{{  form.id ? $t('编辑挂载') : $t('新建挂载')  }}</h3>
        <div class="wd-row-form">
          <label>{{ $t('名称') }} <span class="req">*</span></label>
          <input v-model="form.name" class="input" :placeholder="$t('如 团队网盘')" />
        </div>
        <div class="wd-row-form">
          <label>{{ $t('地址') }} <span class="req">*</span></label>
          <input v-model="form.url" class="input" placeholder="https://example.com/remote.php/dav/files/user/" />
        </div>
        <div class="wd-row-form wd-row-2">
          <div>
            <label>{{ $t('用户名') }}</label>
            <input v-model="form.username" class="input" autocomplete="off" />
          </div>
          <div>
            <label>{{ $t('密码') }}{{  form.id ? $t('（留空不改）') : ''  }}</label>
            <input v-model="form.password" class="input" type="password" autocomplete="new-password" />
          </div>
        </div>
        <label class="wd-switch-label">
          <input type="checkbox" v-model="form.test" />
          <span>{{ $t('保存前先测试连接') }}</span>
        </label>
        <p class="wd-hint">{{ $t('地址需为 WebDAV 根（以 / 结尾更稳）；凭据服务端加密存储，前端不再回显明文密码。') }}</p>
        <div class="wd-modal-actions">
          <button class="btn" @click="formOpen = false">{{ $t('取消') }}</button>
          <button class="btn btn-primary" :disabled="saving" @click="doSave">{{  saving ? $t('保存中…') : $t('保存')  }}</button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      v-if="confirmDel"
      :title="$t('删除挂载')"
      :message="$t('删除挂载「{v0}」？不影响远端数据，仅移除本地配置。', { v0: confirmDel.name })"
      confirm-text="删除"
      danger
      @confirm="doDelete(confirmDel)"
      @cancel="confirmDel = null"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const toast = useToastStore()

const loading = ref(false)
const mounts = ref([])
const cur = ref(null)
const curPath = ref('')
const entries = ref([])
const browsing = ref(false)
const testing = ref('')
const importing = ref('')
const confirmDel = ref(null)
const importParent = ref('')
const dirs = ref([])

const formOpen = ref(false)
const saving = ref(false)
const form = ref(blankForm())

function blankForm() {
  return { id: '', name: '', url: '', username: '', password: '', test: true }
}

const crumbs = computed(() => (curPath.value || '').split('/').filter(Boolean))
const dirOptions = computed(() => flattenDirs(dirs.value))

function pathUpTo(i) {
  return crumbs.value.slice(0, i + 1).join('/')
}

function flattenDirs(list) {
  const out = []
  const walk = (parentId, depth) => {
    for (const d of list) {
      if ((d.parent_id || '') !== parentId) continue
      out.push({ id: d.id, label: '　'.repeat(depth) + d.name })
      walk(d.id, depth + 1)
    }
  }
  walk('', 0)
  return out
}

async function loadMounts() {
  loading.value = true
  try {
    const d = await api.webdavMounts()
    mounts.value = d.items || []
    const prev = cur.value ? mounts.value.find((m) => m.id === cur.value.id) : null
    if (prev) cur.value = prev
    else if (mounts.value.length) await selectMount(mounts.value[0])
    else {
      cur.value = null
      entries.value = []
    }
  } catch (e) {
    toast.push(t('加载挂载失败：') + (e.message || e))
  } finally {
    loading.value = false
  }
}

async function loadDirs() {
  try {
    const d = await api.getTree()
    dirs.value = d.items || []
  } catch (_) {
    dirs.value = []
  }
}

async function selectMount(m) {
  cur.value = m
  curPath.value = ''
  entries.value = []
  await loadDir('')
}

async function loadDir(path) {
  if (!cur.value) return
  browsing.value = true
  curPath.value = path || ''
  try {
    const d = await api.webdavMountList(cur.value.id, path || '')
    entries.value = d.items || []
  } catch (e) {
    entries.value = []
    toast.push(t('读取远端目录失败：') + (e.message || e))
  } finally {
    browsing.value = false
  }
}

function openDir(path) {
  loadDir(path)
}

async function doTest(m) {
  testing.value = m.id
  try {
    const d = await api.webdavMountTest(m.id)
    toast.push(d.message || t('连接正常'))
  } catch (e) {
    toast.push(t('连接失败：') + (e.message || e))
  } finally {
    testing.value = ''
  }
}

async function doImport(path) {
  if (!cur.value) return
  const key = path === curPath.value ? '__dir__' : path
  importing.value = key
  try {
    const d = await api.webdavMountImport(cur.value.id, path, importParent.value)
    toast.push(`导入完成：新增 ${d.imported || 0}，跳过 ${d.skipped || 0}`)
  } catch (e) {
    toast.push(t('导入失败：') + (e.message || e))
  } finally {
    importing.value = ''
  }
}

function openCreate() {
  form.value = blankForm()
  formOpen.value = true
}

function openEdit(m) {
  form.value = { id: m.id, name: m.name, url: m.url, username: m.username || '', password: '', test: false }
  formOpen.value = true
}

async function doSave() {
  const f = form.value
  if (!f.name.trim() || !f.url.trim()) {
    toast.push(t('名称和地址必填'))
    return
  }
  saving.value = true
  try {
    const payload = {
      name: f.name.trim(),
      url: f.url.trim(),
      username: f.username.trim(),
      password: f.password,
      test: !!f.test
    }
    if (f.id) await api.webdavMountUpdate(f.id, payload)
    else await api.webdavMountCreate(payload)
    formOpen.value = false
    toast.push(t('已保存'))
    await loadMounts()
  } catch (e) {
    toast.push(t('保存失败：') + (e.message || e))
  } finally {
    saving.value = false
  }
}

async function doDelete(m) {
  confirmDel.value = null
  try {
    await api.webdavMountDelete(m.id)
    if (cur.value && cur.value.id === m.id) {
      cur.value = null
      entries.value = []
    }
    toast.push(t('挂载已删除'))
    await loadMounts()
  } catch (e) {
    toast.push(t('删除失败：') + (e.message || e))
  }
}

function fmtSize(n) {
  if (!n) return '0 B'
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1024 / 1024).toFixed(1) + ' MB'
}

onMounted(() => {
  loadDirs()
  loadMounts()
})
</script>

<style scoped>
.wd-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.wd-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; margin-bottom: 14px; }
.wd-head h2 { font-size: 20px; margin: 0 0 4px; }
.wd-sub { color: var(--text-3, #8a919f); font-size: 13px; margin: 0; max-width: 760px; line-height: 1.6; }
.wd-sub code { font-size: 12px; padding: 1px 5px; border-radius: 4px; background: var(--bg-3, #f0f1f4); }
.wd-tools { display: inline-flex; gap: 8px; flex-shrink: 0; }

.wd-main { display: flex; gap: 14px; align-items: flex-start; }
.wd-list { flex: 0 0 300px; display: flex; flex-direction: column; gap: 8px; max-height: calc(100vh - 250px); overflow-y: auto; }
.wd-card { border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--bg-2, #fff); padding: 11px 12px; cursor: pointer; transition: border-color .14s, box-shadow .14s; }
.wd-card:hover { border-color: var(--primary, #2f6bff); }
.wd-card.sel { border-color: var(--primary, #2f6bff); box-shadow: 0 0 0 3px var(--primary-soft, rgba(47,107,255,.12)); }
.wd-card-top { display: flex; align-items: center; gap: 6px; }
.wd-card-name { font-size: 14px; color: var(--text, #20242c); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.wd-badge { display: inline-flex; align-items: center; gap: 3px; font-size: 11px; padding: 1px 6px; border-radius: 8px; background: var(--bg-3, #f0f1f4); color: var(--text-3, #8a919f); flex-shrink: 0; }
.wd-card-url { font-size: 11.5px; color: var(--text-3, #8a919f); margin: 5px 0 8px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.wd-card-ops { display: inline-flex; gap: 6px; }

.wd-browse { flex: 1; min-width: 0; border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--bg-2, #fff); min-height: 460px; display: flex; flex-direction: column; }
.wd-browse-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 11px 14px; border-bottom: 1px solid var(--border, #ececf1); flex-wrap: wrap; }
.wd-crumbs { display: flex; align-items: center; gap: 5px; font-size: 12.5px; color: var(--text-3, #8a919f); min-width: 0; flex-wrap: wrap; }
.wd-crumbs a { color: var(--primary, #2f6bff); cursor: pointer; }
.wd-browse-tools { display: inline-flex; gap: 8px; align-items: center; }
.wd-pick { height: 30px; max-width: 240px; padding: 0 8px; border-radius: 7px; border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 12.5px; outline: none; }

.wd-rows { flex: 1; overflow-y: auto; }
.wd-row { display: flex; align-items: center; gap: 10px; padding: 9px 14px; border-bottom: 1px solid var(--border, #f2f3f7); }
.wd-row-ico { flex-shrink: 0; color: var(--text-3, #8a919f); display: inline-flex; }
.wd-row-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.wd-row-name { font-size: 13.5px; color: var(--primary, #2f6bff); cursor: pointer; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.wd-row-name.plain { color: var(--text, #20242c); cursor: default; }
.wd-row-meta { display: flex; gap: 10px; font-size: 11.5px; color: var(--text-3, #8a919f); }

.wd-browse-foot { padding: 10px 14px; border-top: 1px solid var(--border, #ececf1); }

.wd-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; color: var(--text-3, #8a919f); padding: 50px 0; font-size: 13px; }
.wd-empty p { margin: 0; }
.wd-empty-sm { padding: 18px 14px; color: var(--text-3, #8a919f); font-size: 12.5px; }

.modal-mask { position: fixed; inset: 0; z-index: 400; background: rgba(22,24,43,.45); display: flex; align-items: center; justify-content: center; }
.modal-box { background: var(--surface, #fff); border-radius: var(--radius, 12px); padding: 22px 24px; box-shadow: 0 20px 50px rgba(22,24,43,.2); }
.wd-modal { width: 540px; max-width: calc(100vw - 40px); display: flex; flex-direction: column; gap: 12px; }
.wd-modal-title { margin: 0; font-size: 16px; color: var(--text, #20242c); }
.wd-row-form { display: flex; flex-direction: column; gap: 5px; }
.wd-row-form label { font-size: 12.5px; color: var(--text-2, #4a5164); }
.wd-row-2 { flex-direction: row; gap: 12px; }
.wd-row-2 > div { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 5px; }
.req { color: #e03e3e; }
.wd-switch-label { display: inline-flex; align-items: center; gap: 7px; font-size: 13px; color: var(--text, #20242c); cursor: pointer; }
.wd-hint { font-size: 11.5px; color: var(--text-3, #8a919f); margin: 0; line-height: 1.6; }
.wd-modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 4px; }

.input { width: 100%; box-sizing: border-box; height: 34px; padding: 0 12px; border-radius: var(--radius-sm, 8px); border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 13px; outline: none; transition: border-color .15s, box-shadow .15s, background .15s; }
.input::placeholder { color: var(--text-3, #a0a6b2); }
.input:focus { border-color: var(--primary, #2f6bff); background: var(--bg-1, #fff); box-shadow: 0 0 0 3px rgba(47,107,255,.12); }
</style>
