<template>
  <div class="pv-view">
    <div class="pv-head">
      <div>
        <h2>{{  $t('提示词模板')  }}</h2>
        <p class="pv-sub">
          {{  $t('把 AI 写作提示词做成后台可配置项：在正文里写')  }} <code v-pre>{{  变量名  }}</code> {{  $t('声明占位符， 写作时填值即可复用。公开模板全站可用，私有模板仅自己可见——不必再为每种文体改代码。')  }}
        </p>
      </div>
      <div class="pv-tools">
        <button class="btn" :disabled="loading" @click="load"><AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}</button>
        <button class="btn btn-primary" @click="openNew"><AikIcon name="plus" :size="14" />{{  $t('新建模板')  }}</button>
      </div>
    </div>

    <div class="pv-kpis">
      <div class="pv-kpi"><span class="pv-kpi-n">{{  list.length  }}</span><span class="pv-kpi-l">{{  $t('可见模板')  }}</span></div>
      <div class="pv-kpi"><span class="pv-kpi-n">{{  list.filter((t) => t.is_public).length  }}</span><span class="pv-kpi-l">{{  $t('公开')  }}</span></div>
      <div class="pv-kpi"><span class="pv-kpi-n">{{  list.filter((t) => !t.is_public).length  }}</span><span class="pv-kpi-l">{{  $t('私有')  }}</span></div>
      <div class="pv-kpi"><span class="pv-kpi-n">{{  list.filter((t) => t.editable).length  }}</span><span class="pv-kpi-l">{{  $t('我可编辑')  }}</span></div>
    </div>

    <div class="pv-bar">
      <input v-model="q" class="pv-search" :placeholder="$t('搜索名称 / 分类 / 提示词正文')" />
      <select v-model="cat" class="pv-pick">
        <option value="">{{  $t('全部分类')  }}</option>
        <option v-for="c in categories" :key="c" :value="c">{{  c  }}</option>
      </select>
      <label class="pv-chk"><input v-model="onlyMine" type="checkbox" />{{  $t('仅我的')  }}</label>
    </div>

    <div v-if="loading" class="pv-empty">{{  $t('加载中…')  }}</div>
    <div v-else-if="!filtered.length" class="pv-empty">
      {{  $t('没有匹配的提示词模板')  }}<template v-if="!list.length">{{ $t('，点右上「新建模板」开始') }}</template>
    </div>
    <div v-else class="pv-grid">
      <div v-for="t in filtered" :key="t.id" class="pv-card">
        <div class="pv-card-head">
          <b class="pv-card-name">{{  t.name  }}</b>
          <span class="pv-badge" :class="{ pub: t.is_public }">{{  t.is_public ? $t('公开') : $t('私有')  }}</span>
        </div>
        <div class="pv-card-meta">
          <span v-if="t.category" class="pv-cat">{{  t.category  }}</span>
          <span>{{  t.variables.length  }} {{ $t('个变量') }}</span>
          <span class="pv-dot">·</span>
          <span>{{  t.author || '—'  }}</span>
          <span class="pv-dot">·</span>
          <span>{{  fmtTime(t.updated_at)  }}</span>
        </div>
        <p class="pv-card-body">{{  t.content  }}</p>
        <div class="pv-card-ops">
          <button class="btn btn-sm btn-primary" @click="openUse(t)">{{ $t('使用') }}</button>
          <button v-if="t.editable" class="btn btn-sm" @click="openEdit(t)">{{ $t('编辑') }}</button>
          <button v-if="t.editable" class="btn btn-sm pv-danger" @click="askDelete(t)">{{ $t('删除') }}</button>
        </div>
      </div>
    </div>

    <!-- 使用（渲染）弹窗 -->
    <div v-if="useT" class="pv-mask" @click.self="closeUse">
      <div class="pv-modal">
        <div class="pv-modal-head">
          <b>{{ $t('使用「') }}{{  useT.name  }}」</b>
          <button class="pv-x" @click="closeUse"><AikIcon name="close" :size="16" /></button>
        </div>
        <div class="pv-modal-body">
          <div v-if="useT.variables.length" class="pv-fields">
            <label v-for="v in useT.variables" :key="v.name" class="pv-field">
              <span class="pv-fl">{{  v.label || v.name  }}</span>
              <input v-model="useVals[v.name]" class="pv-input" :placeholder="v.default || ('{{ ' + v.name + ' }}')" />
            </label>
          </div>
          <p v-else class="pv-hint">{{ $t('该模板没有声明变量，可直接渲染。') }}</p>
          <div class="pv-prev-label">
            {{ $t('渲染结果') }}
            <span v-if="renderMissing.length" class="pv-warn-inline">{{ $t('未解析：') }}{{  renderMissing.join('、')  }}</span>
          </div>
          <textarea class="pv-prev" readonly rows="8" :value="renderOut" :placeholder="$t('点下方「渲染」生成结果')"></textarea>
        </div>
        <div class="pv-modal-foot">
          <button class="btn" :disabled="rendering" @click="doRender">{{  rendering ? $t('渲染中…') : $t('渲染')  }}</button>
          <button class="btn btn-primary" :disabled="!renderOut" @click="copyOut">{{ $t('复制结果') }}</button>
        </div>
      </div>
    </div>

    <!-- 删除确认（站内弹窗，不用浏览器原生 confirm） -->
    <div v-if="delT" class="pv-mask" @click.self="delT = null">
      <div class="pv-modal">
        <div class="pv-modal-head"><b>{{ $t('删除模板') }}</b><button class="pv-x" @click="delT = null"><AikIcon name="close" :size="16" /></button></div>
        <div class="pv-modal-body">
          <p class="pv-hint">{{ $t('确定删除「') }}{{  delT.name  }}{{ $t('」？删除后不可恢复；已复制走的提示词文本不受影响。') }}</p>
        </div>
        <div class="pv-modal-foot">
          <button class="btn" @click="delT = null">{{ $t('取消') }}</button>
          <button class="btn pv-danger" :disabled="deleting" @click="doDelete">{{  deleting ? $t('删除中…') : $t('确认删除')  }}</button>
        </div>
      </div>
    </div>

    <!-- 新建 / 编辑弹窗 -->
    <div v-if="form" class="pv-mask" @click.self="form = null">
      <div class="pv-modal wide">
        <div class="pv-modal-head">
          <b>{{  form.id ? $t('编辑模板') : $t('新建模板')  }}</b>
          <button class="pv-x" @click="form = null"><AikIcon name="close" :size="16" /></button>
        </div>
        <div class="pv-modal-body">
          <div class="pv-row2">
            <label class="pv-field">
              <span class="pv-fl">{{ $t('名称 *') }}</span>
              <input v-model="form.name" class="pv-input" :placeholder="$t('如：公众号开头润色')" maxlength="200" />
            </label>
            <label class="pv-field">
              <span class="pv-fl">{{ $t('分类') }}</span>
              <input v-model="form.category" class="pv-input" :placeholder="$t('写作 / 分析 / 客服…')" list="pv-cats" />
              <datalist id="pv-cats">
                <option v-for="c in categories" :key="c" :value="c"></option>
              </datalist>
            </label>
          </div>
          <label class="pv-field">
            <span class="pv-fl">{{ $t('提示词正文 *（用') }} <code v-pre>{{  变量名  }}</code> {{ $t('声明占位符）') }}</span>
            <textarea v-model="form.content" class="pv-input pv-ta" rows="7" :placeholder="$t('请以{{ 语气 }}的风格，为《{{ 标题 }}》写一段简介，面向{{ 读者 }}。')"></textarea>
          </label>

          <div class="pv-vars-head">
            <span class="pv-fl">{{ $t('变量声明（驱动「使用」时的表单）') }}</span>
            <button class="btn btn-sm" @click="addVar"><AikIcon name="plus" :size="12" />{{ $t('加变量') }}</button>
          </div>
          <div v-if="!form.vars.length" class="pv-hint">{{ $t('还没有变量。变量名需为字母/数字/下划线/中文，且不可重复。') }}</div>
          <div v-for="(v, i) in form.vars" :key="'v' + i" class="pv-var-row">
            <input v-model="v.name" class="pv-input" :placeholder="$t('变量名（如 标题）')" />
            <input v-model="v.label" class="pv-input" :placeholder="$t('显示名（可空）')" />
            <input v-model="v.default" class="pv-input" :placeholder="$t('默认值（可空）')" />
            <button class="pv-x" :title="t('删除变量 ') + (v.name || i + 1)" @click="form.vars.splice(i, 1)">
              <AikIcon name="trash" :size="14" />
            </button>
          </div>

          <label class="pv-chk pv-chk-block">
            <input v-model="form.is_public" type="checkbox" />
            {{ $t('公开（全站可用；不勾选则仅自己可见）') }}
          </label>
          <p v-if="formErr" class="pv-err">{{  formErr  }}</p>
        </div>
        <div class="pv-modal-foot">
          <button class="btn" @click="form = null">{{ $t('取消') }}</button>
          <button class="btn btn-primary" :disabled="saving" @click="saveForm">{{  saving ? $t('保存中…') : $t('保存')  }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const toast = useToastStore()

const list = ref([])
const loading = ref(false)
const q = ref('')
const cat = ref('')
const onlyMine = ref(false)

// 使用（渲染）态
const useT = ref(null)
const useVals = ref({})
const renderOut = ref('')
const renderMissing = ref([])
const rendering = ref(false)

// 新建 / 编辑态
const form = ref(null)
const formErr = ref('')
const saving = ref(false)
const delT = ref(null)
const deleting = ref(false)

const categories = computed(() => {
  const s = new Set()
  for (const t of list.value) if (t.category) s.add(t.category)
  return [...s].sort()
})

const filtered = computed(() => {
  const kw = q.value.trim().toLowerCase()
  return list.value.filter((t) => {
    if (onlyMine.value && !t.editable) return false
    if (cat.value && t.category !== cat.value) return false
    if (!kw) return true
    return (
      (t.name || '').toLowerCase().includes(kw) ||
      (t.category || '').toLowerCase().includes(kw) ||
      (t.content || '').toLowerCase().includes(kw)
    )
  })
})

async function load() {
  loading.value = true
  try {
    const d = await api.listPrompts()
    list.value = d.templates || []
  } catch (e) {
    toast.error(t('加载提示词模板失败：') + (e.message || e))
  } finally {
    loading.value = false
  }
}

function fmtTime(ts) {
  if (!ts) return '—'
  const d = new Date(ts < 1e12 ? ts * 1000 : ts)
  const p = (x) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

// ---- 使用（渲染） ----

function openUse(t) {
  useT.value = t
  renderOut.value = ''
  renderMissing.value = []
  const vals = {}
  for (const v of t.variables || []) vals[v.name] = ''
  useVals.value = vals
}

function closeUse() {
  useT.value = null
  renderOut.value = ''
  renderMissing.value = []
}

async function doRender() {
  if (!useT.value) return
  rendering.value = true
  try {
    const d = await api.renderPrompt(useT.value.id, useVals.value)
    renderOut.value = d.text || ''
    renderMissing.value = d.missing || []
    if (renderMissing.value.length) toast.push(t('有 ') + renderMissing.value.length + t(' 个变量没解析，已原样保留'))
  } catch (e) {
    toast.error(t('渲染失败：') + (e.message || e))
  } finally {
    rendering.value = false
  }
}

async function copyOut() {
  try {
    await navigator.clipboard.writeText(renderOut.value)
    toast.success(t('已复制到剪贴板'))
  } catch (_) {
    toast.error(t('复制失败，请手动选择文本'))
  }
}

// ---- 新建 / 编辑 ----

function openNew() {
  formErr.value = ''
  form.value = { id: '', name: '', category: cat.value || '', content: '', is_public: true, vars: [] }
}

function openEdit(t) {
  formErr.value = ''
  form.value = {
    id: t.id,
    name: t.name,
    category: t.category || '',
    content: t.content,
    is_public: !!t.is_public,
    vars: (t.variables || []).map((v) => ({ name: v.name, label: v.label || '', default: v.default || '' }))
  }
}

function addVar() {
  form.value?.vars.push({ name: '', label: '', default: '' })
}

async function saveForm() {
  const f = form.value
  if (!f) return
  formErr.value = ''
  if (!f.name.trim()) {
    formErr.value = t('名称必填')
    return
  }
  if (!f.content.trim()) {
    formErr.value = t('提示词正文必填')
    return
  }
  // 变量：丢弃整行都为空的行；只填了显示名/默认值但没填变量名 → 视为误填
  const vars = []
  const seen = new Set()
  for (const v of f.vars) {
    const name = (v.name || '').trim()
    if (!name) {
      if ((v.label || '').trim() || (v.default || '').trim()) {
        formErr.value = t('有变量只填了显示名/默认值，但没填变量名')
        return
      }
      continue
    }
    if (seen.has(name)) {
      formErr.value = `变量名「${name}」重复`
      return
    }
    seen.add(name)
    vars.push({ name, label: (v.label || '').trim(), default: v.default || '' })
  }
  const payload = {
    name: f.name.trim(),
    category: (f.category || '').trim(),
    content: f.content,
    variables: vars,
    is_public: !!f.is_public
  }
  saving.value = true
  try {
    if (f.id) {
      await api.updatePrompt(f.id, payload)
      toast.success(t('模板已更新'))
    } else {
      await api.createPrompt(payload)
      toast.success(t('模板已创建'))
    }
    form.value = null
    await load()
  } catch (e) {
    formErr.value = e.message || String(e)
  } finally {
    saving.value = false
  }
}

function askDelete(t) {
  delT.value = t
}

async function doDelete() {
  const t = delT.value
  if (!t) return
  deleting.value = true
  try {
    await api.deletePrompt(t.id)
    toast.success(t('已删除'))
    delT.value = null
    await load()
  } catch (e) {
    toast.error(t('删除失败：') + (e.message || e))
  } finally {
    deleting.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.pv-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.pv-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; margin-bottom: 14px; }
.pv-head h2 { font-size: 20px; margin: 0 0 4px; }
.pv-sub { color: var(--text-3, #8a919f); font-size: 13px; margin: 0; max-width: 780px; line-height: 1.7; }
.pv-sub code, .pv-fl code { background: var(--surface-2, #f2f4f8); padding: 1px 5px; border-radius: 5px; font-size: 12px; }
.pv-tools { display: inline-flex; gap: 8px; flex-shrink: 0; }

.pv-kpis { display: flex; gap: 10px; flex-wrap: wrap; margin-bottom: 12px; }
.pv-kpi { flex: 1 1 130px; display: flex; flex-direction: column; gap: 2px; padding: 11px 14px; border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--surface-2, #f7f8fa); }
.pv-kpi-n { font-size: 19px; font-weight: 700; color: var(--text, #20242c); }
.pv-kpi-l { font-size: 11.5px; color: var(--text-3, #8a919f); }

.pv-bar { display: flex; gap: 8px; align-items: center; margin-bottom: 12px; flex-wrap: wrap; }
.pv-search { flex: 1; max-width: 340px; height: 34px; padding: 0 12px; border-radius: 8px; border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 13px; outline: none; }
.pv-search:focus { border-color: var(--primary, #2f6bff); background: #fff; box-shadow: 0 0 0 3px rgba(47, 107, 255, 0.12); }
.pv-pick { height: 34px; padding: 0 10px; border-radius: 8px; border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 13px; outline: none; }
.pv-chk { display: inline-flex; align-items: center; gap: 6px; font-size: 12.5px; color: var(--text-2, #555c6b); cursor: pointer; }
.pv-chk-block { margin-top: 12px; }

.pv-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 12px; }
.pv-card { display: flex; flex-direction: column; gap: 8px; padding: 13px 15px; border: 1px solid var(--border, #e3e6ee); border-radius: 11px; background: var(--bg-2, #fff); }
.pv-card-head { display: flex; align-items: center; gap: 8px; }
.pv-card-name { flex: 1; min-width: 0; font-size: 14px; color: var(--text, #20242c); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pv-badge { flex-shrink: 0; font-size: 10.5px; padding: 1px 7px; border-radius: 8px; background: var(--surface-2, #f0f1f5); color: var(--text-3, #8a919f); }
.pv-badge.pub { background: var(--primary-soft, #eaf1ff); color: var(--primary, #2f6bff); }
.pv-card-meta { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 11.5px; color: var(--text-3, #8a919f); }
.pv-cat { padding: 1px 7px; border-radius: 8px; background: var(--surface-2, #f2f4f8); color: var(--text-2, #555c6b); }
.pv-dot { opacity: 0.5; }
.pv-card-body { margin: 0; font-size: 12.5px; line-height: 1.65; color: var(--text-2, #555c6b); display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; white-space: pre-wrap; word-break: break-word; }
.pv-card-ops { display: flex; gap: 6px; margin-top: auto; padding-top: 4px; }
.pv-danger { color: #e03e3e; }

.pv-empty { padding: 44px 14px; text-align: center; color: var(--text-3, #8a919f); font-size: 13px; }
.pv-hint { font-size: 12px; color: var(--text-3, #8a919f); margin: 6px 0; line-height: 1.6; }

.pv-mask { position: fixed; inset: 0; background: rgba(20, 24, 32, 0.42); display: flex; align-items: center; justify-content: center; z-index: 60; padding: 20px; }
.pv-modal { width: 100%; max-width: 560px; max-height: 88vh; display: flex; flex-direction: column; background: var(--bg-2, #fff); border-radius: 13px; overflow: hidden; box-shadow: 0 18px 48px rgba(16, 20, 30, 0.24); }
.pv-modal.wide { max-width: 720px; }
.pv-modal-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 13px 16px; border-bottom: 1px solid var(--border, #eceef4); font-size: 14px; }
.pv-modal-body { padding: 14px 16px; overflow-y: auto; display: flex; flex-direction: column; gap: 10px; }
.pv-modal-foot { display: flex; justify-content: flex-end; gap: 8px; padding: 12px 16px; border-top: 1px solid var(--border, #eceef4); background: var(--surface-2, #fafbfd); }
.pv-x { display: inline-flex; align-items: center; justify-content: center; width: 28px; height: 28px; border: 0; border-radius: 7px; background: transparent; color: var(--text-3, #8a919f); cursor: pointer; }
.pv-x:hover { background: var(--surface-2, #f0f1f5); color: var(--text, #20242c); }

.pv-fields { display: flex; flex-direction: column; gap: 9px; }
.pv-field { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0; }
.pv-fl { font-size: 12px; color: var(--text-3, #8a919f); }
.pv-input { width: 100%; box-sizing: border-box; min-height: 34px; padding: 7px 11px; border-radius: 8px; border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 13px; font-family: inherit; outline: none; }
.pv-input:focus { border-color: var(--primary, #2f6bff); background: #fff; box-shadow: 0 0 0 3px rgba(47, 107, 255, 0.12); }
.pv-ta { resize: vertical; line-height: 1.6; }
.pv-row2 { display: flex; gap: 10px; }
.pv-row2 > .pv-field { flex: 1; }

.pv-vars-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: 4px; }
.pv-var-row { display: grid; grid-template-columns: 1.2fr 1fr 1fr 32px; gap: 7px; align-items: center; }

.pv-prev-label { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--text-3, #8a919f); }
.pv-warn-inline { color: #d98200; font-size: 11.5px; }
.pv-prev { width: 100%; box-sizing: border-box; padding: 10px 12px; border-radius: 9px; border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 12.5px; line-height: 1.65; font-family: inherit; resize: vertical; }
.pv-err { color: #e03e3e; font-size: 12.5px; margin: 2px 0 0; }
</style>
