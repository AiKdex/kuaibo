<template>
  <div class="amr">
    <div class="amr-head">
      <h2>市场审核</h2>
      <p class="muted">第三方开发者与应用上架前的平台审核闸门（仅站长可见）。</p>
    </div>

    <div v-if="forbidden" class="amr-err">无权限：仅站点管理员可访问审核台。</div>
    <div v-if="err" class="amr-err">{{ err }}</div>
    <div v-if="msg" class="amr-ok">{{ msg }}</div>

    <div v-if="!forbidden" class="amr-nav">
      <button class="sv-tab" :class="{ on: tab === 'devs' }" @click="tab = 'devs'">开发者申请 ({{ devCount }})</button>
      <button class="sv-tab" :class="{ on: tab === 'listings' }" @click="tab = 'listings'">应用审核 ({{ listCount }})</button>
      <label class="amr-all"><input type="checkbox" v-model="showAll" @change="reload" /> 显示全部状态</label>
    </div>

    <!-- 开发者申请 -->
    <section v-if="!forbidden && tab === 'devs'" class="amr-card">
      <table class="amr-tbl" v-if="devs.length">
        <thead><tr><th>开发者</th><th>简介</th><th>状态</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="d in devs" :key="d.id">
            <td>{{ d.display_name }}</td>
            <td class="amr-bio">{{ d.bio || '—' }}</td>
            <td><span class="amr-badge" :class="'st-' + d.status">{{ devStatus(d.status) }}</span></td>
            <td class="amr-ops">
              <button v-if="d.status === 'pending'" class="btn btn-sm btn-primary" :disabled="busy" @click="approveDev(d.id)">通过</button>
              <button v-if="d.status === 'pending'" class="btn btn-sm" :disabled="busy" @click="rejectDev(d.id)">驳回</button>
              <span v-else class="muted">—</span>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="muted">暂无待审开发者申请。</p>
    </section>

    <!-- 应用审核 -->
    <section v-if="!forbidden && tab === 'listings'" class="amr-card">
      <table class="amr-tbl" v-if="items.length">
        <thead><tr><th>应用</th><th>类型</th><th>开发者</th><th>版本</th><th>状态</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="l in items" :key="l.id">
            <td>
              <div class="amr-name">{{ l.name }}</div>
              <div class="muted" style="font-size:12px">{{ (l.description || '').slice(0, 40) }}</div>
              <a v-if="l.download_url" :href="l.download_url" target="_blank" rel="noopener" class="amr-link">查看包</a>
            </td>
            <td>{{ l.kind }}</td>
            <td>{{ l.developer || '—' }}</td>
            <td>v{{ l.version }}</td>
            <td><span class="amr-badge" :class="'st-' + l.status">{{ listStatus(l.status) }}</span></td>
            <td class="amr-ops">
              <button v-if="l.status === 'pending'" class="btn btn-sm btn-primary" :disabled="busy" @click="approveList(l.id)">通过</button>
              <button v-if="l.status === 'pending'" class="btn btn-sm" :disabled="busy" @click="rejectList(l.id)">驳回</button>
              <span v-else class="muted">—</span>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="muted">暂无待审应用。</p>
    </section>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import {
  adminDevelopers, adminDeveloperApprove, adminDeveloperReject,
  adminMarketListings, adminMarketApprove, adminMarketReject,
} from '@/api'

const busy = ref(false)
const forbidden = ref(false)
const err = ref('')
const msg = ref('')
const tab = ref('devs')
const showAll = ref(false)

const devs = ref([])
const items = ref([])

const devCount = ref(0)
const listCount = ref(0)

function devStatus(s) { return { pending: '待审核', approved: '已通过', rejected: '已驳回' }[s] || s }
function listStatus(s) { return { draft: '草稿', pending: '审核中', approved: '已上架', rejected: '已驳回' }[s] || s }

function statusParam() { return showAll.value ? 'all' : 'pending' }

async function reload() {
  busy.value = true; err.value = ''; msg.value = ''
  const st = statusParam()
  try {
    const [dv, lv] = await Promise.all([
      adminDevelopers(st).catch((e) => (e?.message ? { __err: e } : { items: [] })),
      adminMarketListings(st).catch((e) => (e?.message ? { __err: e } : { items: [] })),
    ])
    if (dv && dv.__err) { if (String(dv.__err.message).includes('管理员') || dv.__err.message === 'BLOG_ADMIN_REQUIRED') forbidden.value = true; else err.value = dv.__err.message }
    else devs.value = dv.items || []
    if (lv && lv.__err) { if (String(lv.__err.message).includes('管理员') || lv.__err.message === 'BLOG_ADMIN_REQUIRED') forbidden.value = true; else err.value = lv.__err.message }
    else items.value = lv.items || []
    devCount.value = (devs.value || []).filter((d) => d.status === 'pending').length
    listCount.value = (items.value || []).filter((l) => l.status === 'pending').length
  } catch (e) { err.value = e.message || String(e) }
  finally { busy.value = false }
}

async function approveDev(id) { busy.value = true; err.value = ''; try { await adminDeveloperApprove(id); msg.value = '已通过开发者申请'; await reload() } catch (e) { err.value = e.message } finally { busy.value = false } }
async function rejectDev(id) { busy.value = true; err.value = ''; try { await adminDeveloperReject(id); msg.value = '已驳回'; await reload() } catch (e) { err.value = e.message } finally { busy.value = false } }
async function approveList(id) { busy.value = true; err.value = ''; try { await adminMarketApprove(id); msg.value = '应用已上架'; await reload() } catch (e) { err.value = e.message } finally { busy.value = false } }
async function rejectList(id) {
  const note = window.prompt('驳回理由（可选）') || ''
  busy.value = true; err.value = ''
  try { await adminMarketReject(id, note); msg.value = '已驳回应用'; await reload() } catch (e) { err.value = e.message } finally { busy.value = false }
}

onMounted(reload)
</script>

<style scoped>
.amr { max-width: 900px; margin: 0 auto; padding: 8px 4px 60px; }
.amr-head h2 { margin: 0 0 4px; }
.amr-err { color: #b91c1c; background: #fef2f2; border: 1px solid #fecaca; padding: 8px 12px; border-radius: 8px; margin: 8px 0; }
.amr-ok { color: #166534; background: #f0fdf4; border: 1px solid #bbf7d0; padding: 8px 12px; border-radius: 8px; margin: 8px 0; }
.amr-nav { display: flex; align-items: center; gap: 10px; margin: 12px 0; flex-wrap: wrap; }
.sv-tab { border: 1px solid var(--border, #d1d5db); background: var(--bg-2, #fff); color: inherit; padding: 6px 12px; border-radius: 8px; cursor: pointer; }
.sv-tab.on { background: var(--accent, #4f46e5); color: #fff; border-color: var(--accent, #4f46e5); }
.amr-all { font-size: 13px; color: #555; margin-left: auto; }
.amr-card { background: var(--bg-2, #fff); border: 1px solid var(--border, #e5e7eb); border-radius: 12px; padding: 8px 14px; margin: 10px 0; }
.amr-tbl { width: 100%; border-collapse: collapse; }
.amr-tbl th, .amr-tbl td { text-align: left; padding: 10px 8px; border-bottom: 1px solid var(--border, #eee); font-size: 14px; vertical-align: top; }
.amr-bio { max-width: 280px; color: #555; }
.amr-name { font-weight: 600; }
.amr-link { font-size: 12px; color: var(--accent, #4f46e5); margin-left: 8px; }
.amr-ops { white-space: nowrap; }
.amr-badge { display: inline-block; padding: 2px 8px; border-radius: 999px; font-size: 12px; background: #eef2ff; color: #3730a3; }
.amr-badge.st-draft { background: #f3f4f6; color: #374151; }
.amr-badge.st-pending { background: #fef3c7; color: #92400e; }
.amr-badge.st-approved { background: #dcfce7; color: #166534; }
.amr-badge.st-rejected { background: #fee2e2; color: #991b1b; }
</style>
