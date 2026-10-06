<template>
  <div class="us-view">
    <div class="us-head">
      <div>
        <h2>{{  $t('用户管理')  }}</h2>
        <p class="us-sub">
          {{  $t('站点成员的角色与状态。owner 账号受保护（不可降级/禁用，避免把系统锁死）； 角色决定后台可见范围，状态决定能否登录。')  }}
        </p>
      </div>
      <div class="us-tools">
        <button class="btn" :disabled="loading" @click="loadAll"><AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}</button>
      </div>
    </div>

    <div class="us-kpis">
      <div class="us-kpi"><span class="us-kpi-n">{{  users.length  }}</span><span class="us-kpi-l">{{  $t('用户总数')  }}</span></div>
      <div class="us-kpi"><span class="us-kpi-n">{{  countBy('role', ['owner', 'admin'])  }}</span><span class="us-kpi-l">{{  $t('管理员')  }}</span></div>
      <div class="us-kpi"><span class="us-kpi-n">{{  countBy('status', ['active'])  }}</span><span class="us-kpi-l">{{  $t('可登录')  }}</span></div>
      <div class="us-kpi" :class="{ warn: countBy('status', ['disabled']) > 0 }">
        <span class="us-kpi-n">{{  countBy('status', ['disabled'])  }}</span><span class="us-kpi-l">{{  $t('已禁用')  }}</span>
      </div>
    </div>

    <div class="us-bar">
      <input v-model="q" class="us-search" :placeholder="$t('搜索用户名 / 显示名')" />
      <select v-model="roleFilter" class="us-pick">
        <option value="">{{  $t('全部角色')  }}</option>
        <option value="owner">owner</option>
        <option value="admin">admin</option>
        <option value="member">member</option>
        <option value="viewer">viewer</option>
      </select>
    </div>

    <div class="us-table">
      <div class="us-tr us-th">
        <span class="us-c-user">{{  $t('用户')  }}</span>
        <span class="us-c-role">{{  $t('角色')  }}</span>
        <span class="us-c-status">{{  $t('状态')  }}</span>
        <span class="us-c-time">{{  $t('注册时间')  }}</span>
        <span class="us-c-ops">{{  $t('操作')  }}</span>
      </div>
      <div v-if="loading" class="us-empty">{{  $t('加载中…')  }}</div>
      <div v-else-if="!filtered.length" class="us-empty">{{  $t('没有匹配的用户')  }}</div>
      <div v-for="u in filtered" :key="u.id" class="us-tr" :class="{ off: u.status === 'disabled' }">
        <span class="us-c-user">
          <b>{{  u.display_name || u.username  }}</b>
          <em>{{  u.username  }}</em>
          <span v-if="u.role === 'owner'" class="us-tag">{{  $t('主账号')  }}</span>
        </span>
        <span class="us-c-role">
          <select v-model="draft[u.id].role" class="us-sel" :disabled="u.role === 'owner'">
            <option value="admin">admin</option>
            <option value="member">member</option>
            <option value="viewer">viewer</option>
          </select>
        </span>
        <span class="us-c-status">
          <select v-model="draft[u.id].status" class="us-sel" :disabled="u.role === 'owner'">
            <option value="active">active</option>
            <option value="disabled">disabled</option>
            <option value="invited">invited</option>
          </select>
        </span>
        <span class="us-c-time">{{  fmtTime(u.created_at)  }}</span>
        <span class="us-c-ops">
          <button
            class="btn btn-sm btn-primary"
            :disabled="u.role === 'owner' || saving === u.id || !dirty(u)"
            :title="u.role === 'owner' ? $t('owner 账号不可修改') : ''"
            @click="save(u)"
          >{{  saving === u.id ? $t('保存中…') : $t('保存')  }}</button>
        </span>
      </div>
    </div>

    <p class="us-hint">
      {{  $t('新用户通过公开注册入口自行注册（受「开放注册」开关与邮箱验证码约束），注册后默认角色为 member。')  }}
    </p>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const toast = useToastStore()

const users = ref([])
const draft = ref({})
const loading = ref(false)
const saving = ref('')
const q = ref('')
const roleFilter = ref('')

const filtered = computed(() => {
  const kw = q.value.trim().toLowerCase()
  return users.value.filter((u) => {
    if (roleFilter.value && u.role !== roleFilter.value) return false
    if (!kw) return true
    return (u.username || '').toLowerCase().includes(kw) || (u.display_name || '').toLowerCase().includes(kw)
  })
})

function countBy(field, vals) {
  return users.value.filter((u) => vals.includes(u[field])).length
}

function dirty(u) {
  const d = draft.value[u.id]
  if (!d) return false
  return d.role !== u.role || d.status !== u.status
}

async function loadAll() {
  loading.value = true
  try {
    const d = await api.adminUsersList()
    users.value = d.users || []
    const nd = {}
    for (const u of users.value) nd[u.id] = { role: u.role, status: u.status }
    draft.value = nd
  } catch (e) {
    toast.push(t('加载用户失败：') + (e.message || e))
  } finally {
    loading.value = false
  }
}

async function save(u) {
  const d = draft.value[u.id]
  if (!d) return
  saving.value = u.id
  try {
    const payload = {}
    if (d.role !== u.role) payload.role = d.role
    if (d.status !== u.status) payload.status = d.status
    await api.adminUserUpdate(u.id, payload)
    toast.push(`已更新 ${u.display_name || u.username}`)
    await loadAll()
  } catch (e) {
    toast.push(t('保存失败：') + (e.message || e))
  } finally {
    saving.value = ''
  }
}

function fmtTime(ts) {
  if (!ts) return '—'
  const d = new Date(ts < 1e12 ? ts * 1000 : ts)
  const p = (x) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

onMounted(loadAll)
</script>

<style scoped>
.us-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.us-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; margin-bottom: 14px; }
.us-head h2 { font-size: 20px; margin: 0 0 4px; }
.us-sub { color: var(--text-3, #8a919f); font-size: 13px; margin: 0; max-width: 760px; line-height: 1.6; }
.us-tools { display: inline-flex; gap: 8px; flex-shrink: 0; }

.us-kpis { display: flex; gap: 10px; flex-wrap: wrap; margin-bottom: 12px; }
.us-kpi { flex: 1 1 140px; display: flex; flex-direction: column; gap: 2px; padding: 11px 14px; border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--surface-2, #f7f8fa); }
.us-kpi-n { font-size: 19px; font-weight: 700; color: var(--text, #20242c); }
.us-kpi-l { font-size: 11.5px; color: var(--text-3, #8a919f); }
.us-kpi.warn .us-kpi-n { color: #e03e3e; }

.us-bar { display: flex; gap: 8px; margin-bottom: 10px; }
.us-search { flex: 1; max-width: 320px; height: 34px; padding: 0 12px; border-radius: 8px; border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 13px; outline: none; }
.us-search:focus { border-color: var(--primary, #2f6bff); background: #fff; box-shadow: 0 0 0 3px rgba(47,107,255,.12); }
.us-pick { height: 34px; padding: 0 10px; border-radius: 8px; border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 13px; outline: none; }

.us-table { border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--bg-2, #fff); overflow: hidden; }
.us-tr { display: flex; align-items: center; gap: 10px; padding: 10px 14px; border-bottom: 1px solid var(--border, #f2f3f7); font-size: 13px; }
.us-tr:last-child { border-bottom: 0; }
.us-th { background: var(--surface-2, #f7f8fa); font-size: 12px; color: var(--text-3, #8a919f); }
.us-tr.off { opacity: .55; }
.us-c-user { flex: 1; min-width: 0; display: flex; align-items: baseline; gap: 7px; flex-wrap: wrap; }
.us-c-user b { color: var(--text, #20242c); }
.us-c-user em { font-style: normal; font-size: 11.5px; color: var(--text-3, #8a919f); }
.us-tag { font-size: 10.5px; padding: 1px 6px; border-radius: 8px; background: var(--primary-soft, #eaf1ff); color: var(--primary, #2f6bff); }
.us-c-role, .us-c-status { flex: 0 0 120px; }
.us-c-time { flex: 0 0 110px; color: var(--text-3, #8a919f); font-size: 12px; }
.us-c-ops { flex: 0 0 80px; display: flex; justify-content: flex-end; }
.us-sel { width: 100%; height: 30px; padding: 0 8px; border-radius: 7px; border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 12.5px; outline: none; }
.us-sel:disabled { opacity: .6; cursor: not-allowed; }

.us-empty { padding: 44px 14px; text-align: center; color: var(--text-3, #8a919f); font-size: 13px; }
.us-hint { font-size: 11.5px; color: var(--text-3, #8a919f); margin: 10px 0 0; line-height: 1.6; }
</style>
