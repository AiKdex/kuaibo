<template>
  <div class="dev-console">
    <div class="dc-head">
      <h2>开发者中心</h2>
      <p class="muted">面向第三方开发者的自管发布台：申请成为开发者 → 上传应用 → 提交审核 → 上架应用中心。</p>
    </div>

    <div v-if="loading && !me" class="dc-muted">加载中…</div>

    <div v-if="err" class="dc-err">{{ err }}</div>
    <div v-if="msg" class="dc-ok">{{ msg }}</div>

    <!-- 未申请 / 被驳回：申请表单 -->
    <section v-if="me && (!me.exists || me.status === 'rejected')" class="dc-card">
      <h3>{{ me.exists && me.status === 'rejected' ? '申请被驳回，可修改后重新提交' : '申请成为开发者' }}</h3>
      <p class="muted">提交后由站长审核通过即可发布第三方应用。</p>
      <label>开发者名称</label>
      <input v-model="applyName" placeholder="如 我的工作室" />
      <label>简介</label>
      <textarea v-model="applyBio" rows="3" placeholder="一句话介绍你 / 你的团队"></textarea>
      <div class="dc-row">
        <button class="btn btn-primary" :disabled="loading" @click="apply">提交申请</button>
      </div>
    </section>

    <!-- 待审核 -->
    <section v-if="me && me.exists && me.status === 'pending'" class="dc-card">
      <h3>申请审核中</h3>
      <p class="muted">你的开发者申请已提交，站长审核通过后即可发布应用。期间请勿重复提交。</p>
      <div class="dc-pill">状态：待审核（pending）</div>
    </section>

    <!-- 已通过：我的发布 -->
    <section v-if="me && me.status === 'approved'" class="dc-card">
      <div class="dc-card-h">
        <h3>我的发布</h3>
        <button class="btn btn-sm btn-primary" :disabled="loading" @click="openCreate">＋ 新建条目</button>
      </div>

      <table class="dc-tbl" v-if="listings.length">
        <thead><tr><th>名称</th><th>类型</th><th>版本</th><th>状态</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="l in listings" :key="l.id">
            <td>{{ l.name }}</td>
            <td>{{ l.kind }}</td>
            <td>v{{ l.version }}</td>
            <td><span class="dc-badge" :class="'st-' + l.status">{{ statusLabel(l.status) }}</span></td>
            <td class="dc-ops">
              <button class="btn btn-sm" :disabled="loading || l.status === 'pending'" @click="openEdit(l)">编辑</button>
              <button class="btn btn-sm" :disabled="loading || l.status === 'pending'" @click="submit(l.id)" title="提交审核">提交</button>
              <button class="btn btn-sm" :disabled="loading" @click="remove(l.id)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="muted">还没有发布任何条目，点击「新建条目」开始。</p>
    </section>

    <!-- 编辑 / 新建抽屉 -->
    <section v-if="showCreate && me && me.status === 'approved'" class="dc-card">
      <h3>{{ editId ? '编辑条目' : '新建条目' }}</h3>
      <div class="dc-grid">
        <div>
          <label>类型</label>
          <select v-model="form.kind">
            <option value="plugin">插件 plugin</option>
            <option value="theme">主题 theme</option>
            <option value="editor">编辑器 editor</option>
            <option value="ai">AI 应用 ai</option>
          </select>
        </div>
        <div>
          <label>名称</label>
          <input v-model="form.name" placeholder="应用名称" />
        </div>
        <div>
          <label>版本</label>
          <input v-model="form.version" placeholder="0.1.0" />
        </div>
        <div>
          <label>付费层级</label>
          <select v-model="form.tier">
            <option value="free">免费 free</option>
            <option value="paid">付费 paid</option>
          </select>
        </div>
        <div v-if="form.tier === 'paid'">
          <label>价格（分）</label>
          <input v-model.number="form.price_cents" type="number" min="0" />
        </div>
      </div>
      <label>描述</label>
      <textarea v-model="form.description" rows="3" placeholder="功能简介"></textarea>

      <label>图标</label>
      <div class="dc-up">
        <img v-if="form.icon" :src="form.icon" class="dc-icon" alt="" />
        <label class="btn btn-sm dc-up-btn">上传图标
          <input v-if="editId" type="file" accept="image/*" @change="uploadIcon" />
        </label>
        <span class="muted" v-if="!editId">保存条目后可上传</span>
      </div>

      <label>应用包（zip，含 manifest.json）</label>
      <div class="dc-up">
        <label class="btn btn-sm dc-up-btn">上传应用包
          <input v-if="editId" type="file" accept=".zip,application/zip" @change="uploadPkg" />
        </label>
        <span v-if="form.download_url" class="muted dc-pkg">已上传 · SHA256 {{ pkgSha.slice(0, 16) }}…</span>
        <span class="muted" v-else-if="editId">尚未上传</span>
        <span class="muted" v-else>保存条目后可上传</span>
      </div>

      <label>截图</label>
      <div class="dc-shots">
        <img v-for="(s, i) in (form.screenshots || [])" :key="i" :src="s" class="dc-shot" alt="" />
        <label class="btn btn-sm dc-up-btn">＋ 截图
          <input v-if="editId" type="file" accept="image/*" @change="uploadShot" />
        </label>
      </div>

      <div class="dc-row">
        <button class="btn btn-primary" :disabled="loading" @click="save">保存</button>
        <button class="btn btn-sm" @click="showCreate = false">收起</button>
        <span v-if="editId && form.download_url" class="muted">保存后可点列表「提交」进入审核</span>
      </div>
    </section>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import {
  developerMe, developerApply, developerListings, developerListingCreate,
  developerListingUpdate, developerListingSubmit, developerListingDelete, developerListingUpload,
} from '@/api'

const loading = ref(false)
const msg = ref('')
const err = ref('')
const me = ref(null)

const applyName = ref('')
const applyBio = ref('')

const listings = ref([])
const showCreate = ref(false)
const editId = ref('')
const form = ref(blankForm())
const pkgSha = ref('')

function blankForm() {
  return { kind: 'plugin', name: '', description: '', version: '0.1.0', tier: 'free', price_cents: 0, icon: '', screenshots: [], download_url: '', sha256: '' }
}
function parseShots(s) {
  try { const a = JSON.parse(s || '[]'); return Array.isArray(a) ? a : [] } catch { return [] }
}
function statusLabel(s) {
  return { draft: '草稿', pending: '审核中', approved: '已上架', rejected: '已驳回' }[s] || s
}

async function loadMe() {
  loading.value = true; err.value = ''; msg.value = ''
  try {
    const d = await developerMe()
    me.value = d
    if (d.exists && d.status === 'rejected') applyName.value = d.display_name || ''
    if (d.status === 'approved') await loadListings()
  } catch (e) { err.value = e.message || String(e) }
  finally { loading.value = false }
}
async function apply() {
  if (!applyName.value.trim()) { err.value = '请填写开发者名称'; return }
  loading.value = true; err.value = ''
  try {
    await developerApply({ display_name: applyName.value.trim(), bio: applyBio.value })
    msg.value = '已提交申请，等待站长审核'
    await loadMe()
  } catch (e) { err.value = e.message || String(e) }
  finally { loading.value = false }
}
async function loadListings() {
  const d = await developerListings()
  listings.value = d.items || []
}
function openCreate() {
  editId.value = ''; form.value = blankForm(); pkgSha.value = ''; showCreate.value = true
}
function openEdit(l) {
  editId.value = l.id
  form.value = {
    kind: l.kind, name: l.name, description: l.description, version: l.version,
    tier: l.tier, price_cents: l.price_cents || 0, icon: l.icon || '',
    screenshots: parseShots(l.screenshots), download_url: l.download_url || '', sha256: l.sha256 || '',
  }
  pkgSha.value = l.sha256 || ''
  showCreate.value = true
}
async function save() {
  loading.value = true; err.value = ''
  try {
    const payload = { ...form.value, price_cents: Number(form.value.price_cents) || 0, screenshots: JSON.stringify(form.value.screenshots || []) }
    if (editId.value) {
      await developerListingUpdate(editId.value, payload); msg.value = '已保存'
    } else {
      const r = await developerListingCreate(payload); editId.value = r.id; msg.value = '已创建；请上传应用包后提交审核'
    }
    await loadListings()
  } catch (e) { err.value = e.message || String(e) }
  finally { loading.value = false }
}
async function uploadPkg(e) {
  const f = e.target.files?.[0]; if (!f || !editId.value) return
  loading.value = true; err.value = ''
  try {
    const r = await developerListingUpload(editId.value, 'package', f)
    form.value.download_url = r.url; form.value.sha256 = r.sha256; pkgSha.value = r.sha256
    msg.value = '应用包已上传'
  } catch (e) { err.value = e.message || String(e) }
  finally { loading.value = false; e.target.value = '' }
}
async function uploadIcon(e) {
  const f = e.target.files?.[0]; if (!f || !editId.value) return
  loading.value = true; err.value = ''
  try { const r = await developerListingUpload(editId.value, 'icon', f); form.value.icon = r.url; msg.value = '图标已上传' }
  catch (e) { err.value = e.message || String(e) }
  finally { loading.value = false; e.target.value = '' }
}
async function uploadShot(e) {
  const f = e.target.files?.[0]; if (!f || !editId.value) return
  loading.value = true; err.value = ''
  try {
    const r = await developerListingUpload(editId.value, 'screenshot', f)
    form.value.screenshots = [...(form.value.screenshots || []), r.url]; msg.value = '截图已上传'
  } catch (e) { err.value = e.message || String(e) }
  finally { loading.value = false; e.target.value = '' }
}
async function submit(id) {
  loading.value = true; err.value = ''
  try { await developerListingSubmit(id); msg.value = '已提交审核'; await loadListings() }
  catch (e) { err.value = e.message || String(e) }
  finally { loading.value = false }
}
async function remove(id) {
  if (!confirm('确认删除该条目？（仅草稿/驳回可删）')) return
  loading.value = true; err.value = ''
  try {
    await developerListingDelete(id); msg.value = '已删除'
    if (editId.value === id) { showCreate.value = false; editId.value = '' }
    await loadListings()
  } catch (e) { err.value = e.message || String(e) }
  finally { loading.value = false }
}

onMounted(loadMe)
</script>

<style scoped>
.dev-console { max-width: 880px; margin: 0 auto; padding: 8px 4px 60px; }
.dc-head h2 { margin: 0 0 4px; }
.dc-muted { color: #888; padding: 20px 0; }
.dc-err { color: #b91c1c; background: #fef2f2; border: 1px solid #fecaca; padding: 8px 12px; border-radius: 8px; margin: 8px 0; }
.dc-ok { color: #166534; background: #f0fdf4; border: 1px solid #bbf7d0; padding: 8px 12px; border-radius: 8px; margin: 8px 0; }
.dc-card { background: var(--bg-2, #fff); border: 1px solid var(--border, #e5e7eb); border-radius: 12px; padding: 16px 18px; margin: 14px 0; }
.dc-card-h { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.dc-card h3 { margin: 0 0 6px; }
label { display: block; font-size: 13px; color: #555; margin: 10px 0 4px; }
input, textarea, select { width: 100%; box-sizing: border-box; padding: 8px 10px; border: 1px solid var(--border, #d1d5db); border-radius: 8px; background: var(--bg, #fff); color: inherit; font: inherit; }
.dc-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 10px; }
.dc-row { margin-top: 12px; display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
.dc-pill { display: inline-block; padding: 4px 10px; border-radius: 999px; background: #fef3c7; color: #92400e; font-size: 13px; }
.dc-tbl { width: 100%; border-collapse: collapse; margin-top: 8px; }
.dc-tbl th, .dc-tbl td { text-align: left; padding: 8px 6px; border-bottom: 1px solid var(--border, #eee); font-size: 14px; }
.dc-ops { white-space: nowrap; }
.dc-badge { display: inline-block; padding: 2px 8px; border-radius: 999px; font-size: 12px; background: #eef2ff; color: #3730a3; }
.dc-badge.st-draft { background: #f3f4f6; color: #374151; }
.dc-badge.st-pending { background: #fef3c7; color: #92400e; }
.dc-badge.st-approved { background: #dcfce7; color: #166534; }
.dc-badge.st-rejected { background: #fee2e2; color: #991b1b; }
.dc-up { display: flex; align-items: center; gap: 10px; margin: 4px 0 8px; flex-wrap: wrap; }
.dc-up-btn { position: relative; overflow: hidden; cursor: pointer; }
.dc-up-btn input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
.dc-icon { width: 48px; height: 48px; border-radius: 8px; object-fit: cover; border: 1px solid var(--border, #eee); }
.dc-shots { display: flex; gap: 8px; flex-wrap: wrap; margin: 4px 0 8px; }
.dc-shot { width: 96px; height: 60px; object-fit: cover; border-radius: 6px; border: 1px solid var(--border, #eee); }
.dc-pkg { font-family: ui-monospace, monospace; }
</style>
