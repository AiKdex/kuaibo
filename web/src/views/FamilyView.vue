<template>
  <div class="fv-view">
    <div class="fv-head">
      <div>
        <h2>{{ $t('js.1954') }}</h2>
        <p class="fv-sub">{{ $t('js.1955') }}</p>
      </div>
      <div class="fv-tools">
        <button class="btn" @click="loadAll" :disabled="loading">
          <AikIcon name="refresh" :size="14" />{{ $t('common.refresh') }}</button>
      </div>
    </div>

    <!-- 模块开关（owner/admin） -->
    <div v-if="isAdmin" class="fv-switches">
      <label class="fv-switch">
        <span class="fv-switch-label">
          <b>{{ $t('js.1956') }}</b>
          <span class="muted">{{ $t('js.1957') }}</span>
        </span>
        <input type="checkbox" v-model="swEnabled" :disabled="saving" @change="saveSwitch('family.enabled', swEnabled)" />
      </label>
    </div>
    <div v-if="!swEnabled" class="fv-disabled">{{ $t('js.1958') }}</div>

    <template v-if="swEnabled">
      <div class="fv-tabs">
        <button v-for="tb in tabs" :key="tb.key" class="fv-tab" :class="{ active: tab === tb.key }" @click="tab = tb.key">
          <AikIcon :name="tb.icon" :size="14" />{{ $t(tb.label) }}
        </button>
      </div>

      <!-- ===== Tab1 一生时间轴 ===== -->
      <div v-show="tab === 'timeline'" class="fv-pane">
        <div class="fv-pane-tip">
          <AikIcon name="info" :size="13" />{{ $t('js.1959') }}
          <a class="link" href="#/files">{{ $t('js.1960') }}</a>
        </div>
        <div v-if="timelineLoading" class="fv-empty-sm">{{ $t('common.loading') }}</div>
        <div v-else-if="!timeline.length" class="fv-empty">
          <AikIcon name="clock" :size="30" />
          <p>{{ $t('js.1961') }}</p>
          <p class="muted">{{ $t('js.1962') }}</p>
        </div>
        <div v-else class="fv-timeline">
          <template v-for="g in stageGroups" :key="g.stage">
            <div class="fv-stage-head">
              <b>{{ stageLabel(g.stage) }}</b>
              <span class="fv-stage-cnt">{{ g.items.length }} {{ $t('js.1963') }}</span>
            </div>
            <div v-for="it in g.items" :key="it.file_id" class="fv-event" @click="openEvent(it)">
              <div class="fv-event-date">{{ fmtDate(it.occurred_at) }}</div>
              <div class="fv-event-body">
                <div class="fv-event-title">{{ it.title }}</div>
                <div v-if="it.preview" class="fv-event-preview">{{ it.preview }}</div>
              </div>
              <AikIcon name="chevronRight" :size="14" class="fv-event-go" />
            </div>
          </template>
        </div>
      </div>

      <!-- ===== Tab2 家族树 ===== -->
      <div v-show="tab === 'tree'" class="fv-pane">
        <div class="fv-two-col">
          <!-- 成员 -->
          <div class="fv-col">
            <div class="fv-pane-title">
              {{ $t('js.1964') }} <span class="fv-cnt">{{ members.length }}</span>
              <button class="btn btn-sm" @click="memberForm = { name: '', gender: '', birth_date: '', note: '' }; memberEditing = null">
                <AikIcon name="plus" :size="13" />{{ $t('js.1965') }}</button>
            </div>
            <!-- 新增/编辑成员 -->
            <div v-if="memberForm" class="fv-inline-card">
              <div class="fv-form-row">
                <input v-model="memberForm.name" class="input" :placeholder="$t('js.1966')" />
                <select v-model="memberForm.gender" class="input fv-select">
                  <option value="">{{ $t('js.1967') }}</option>
                  <option value="male">{{ $t('js.1968') }}</option>
                  <option value="female">{{ $t('js.1969') }}</option>
                </select>
              </div>
              <div class="fv-form-row">
                <input v-model="memberForm.birth_date" class="input" type="date" />
                <input v-model="memberForm.note" class="input" :placeholder="$t('js.1970')" />
              </div>
              <div class="fv-form-ops">
                <button class="btn btn-sm" :disabled="!memberForm.name.trim()" @click="saveMember">{{ $t('common.confirm') }}</button>
                <button class="btn btn-sm btn-ghost" @click="memberForm = null">{{ $t('common.cancel') }}</button>
              </div>
            </div>
            <div v-if="membersLoading" class="fv-empty-sm">{{ $t('common.loading') }}</div>
            <div v-else-if="!members.length" class="fv-empty-sm">{{ $t('js.1971') }}</div>
            <div v-else class="fv-member-grid">
              <div v-for="m in members" :key="m.id" class="fv-member-card">
                <div class="fv-member-avatar">{{ (m.name || '?').slice(0, 1) }}</div>
                <div class="fv-member-info">
                  <b>{{ m.name }}</b>
                  <div class="muted">{{ genderLabel(m.gender) }}{{ m.birth_date ? ' · ' + m.birth_date : '' }}</div>
                  <div v-if="m.note" class="fv-member-note">{{ m.note }}</div>
                </div>
                <div class="fv-member-ops">
                  <button class="btn btn-sm btn-ghost" :title="$t('common.rename')" @click="startEditMember(m)">
                    <AikIcon name="edit" :size="12" /></button>
                  <button class="btn btn-sm btn-danger-ghost" :title="$t('common.delete')" @click="deleteMember(m)">
                    <AikIcon name="trash" :size="12" /></button>
                </div>
              </div>
            </div>
          </div>
          <!-- 关系 -->
          <div class="fv-col">
            <div class="fv-pane-title">
              {{ $t('js.1972') }} <span class="fv-cnt">{{ relations.length }}</span>
              <button class="btn btn-sm" :disabled="members.length < 2" @click="relForm = { from_id: '', to_id: '', relation: 'parent_of' }">
                <AikIcon name="plus" :size="13" />{{ $t('js.1973') }}</button>
            </div>
            <div v-if="relForm" class="fv-inline-card">
              <div class="fv-form-row">
                <select v-model="relForm.from_id" class="input">
                  <option value="" disabled>{{ $t('js.1974') }}</option>
                  <option v-for="m in members" :key="m.id" :value="m.id">{{ m.name }}</option>
                </select>
                <select v-model="relForm.to_id" class="input">
                  <option value="" disabled>{{ $t('js.1975') }}</option>
                  <option v-for="m in members" :key="m.id" :value="m.id">{{ m.name }}</option>
                </select>
              </div>
              <div class="fv-form-row">
                <select v-model="relForm.relation" class="input">
                  <option value="parent_of">{{ $t('js.1976') }}</option>
                  <option value="child_of">{{ $t('js.1977') }}</option>
                  <option value="spouse_of">{{ $t('js.1978') }}</option>
                  <option value="sibling_of">{{ $t('js.1979') }}</option>
                  <option value="other">{{ $t('js.1980') }}</option>
                </select>
                <div class="fv-form-ops">
                  <button class="btn btn-sm" :disabled="!relForm.from_id || !relForm.to_id || relForm.from_id === relForm.to_id" @click="saveRelation">{{ $t('common.confirm') }}</button>
                  <button class="btn btn-sm btn-ghost" @click="relForm = null">{{ $t('common.cancel') }}</button>
                </div>
              </div>
            </div>
            <div v-if="relationsLoading" class="fv-empty-sm">{{ $t('common.loading') }}</div>
            <div v-else-if="!relations.length" class="fv-empty-sm">{{ $t('js.1981') }}</div>
            <div v-else class="fv-rel-list">
              <div v-for="r in relations" :key="r.id" class="fv-rel-row">
                <span class="fv-rel-from">{{ memberName(r.from_id) }}</span>
                <span class="fv-rel-edge">{{ relLabel(r.relation) }}</span>
                <span class="fv-rel-to">{{ memberName(r.to_id) }}</span>
                <button class="btn btn-sm btn-danger-ghost" :title="$t('common.delete')" @click="deleteRelation(r)">
                  <AikIcon name="trash" :size="12" /></button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- ===== Tab3 纪念日 ===== -->
      <div v-show="tab === 'anniv'" class="fv-pane">
        <div class="fv-pane-title">
          {{ $t('js.1982') }}
          <button class="btn btn-sm" @click="annivForm = { title: '', kind: 'custom', date: '', ref_member_id: '', remind_days: 1, enabled: true }; annivEditing = null">
            <AikIcon name="plus" :size="13" />{{ $t('js.1983') }}</button>
        </div>
        <div v-if="annivForm" class="fv-inline-card">
          <div class="fv-form-row">
            <input v-model="annivForm.title" class="input" :placeholder="$t('js.1984')" />
            <input v-model="annivForm.date" class="input" type="date" />
          </div>
          <div class="fv-form-row">
            <select v-model="annivForm.kind" class="input">
              <option value="birthday">{{ $t('js.1985') }}</option>
              <option value="anniversary">{{ $t('js.1986') }}</option>
              <option value="custom">{{ $t('js.1987') }}</option>
            </select>
            <select v-model="annivForm.ref_member_id" class="input">
              <option value="">{{ $t('js.1988') }}</option>
              <option v-for="m in members" :key="m.id" :value="m.id">{{ m.name }}</option>
            </select>
            <input v-model.number="annivForm.remind_days" class="input fv-remind" type="number" min="0" max="30" />
          </div>
          <div class="fv-form-ops">
            <button class="btn btn-sm" :disabled="!annivForm.title.trim() || !annivForm.date" @click="saveAnniv">{{ $t('common.confirm') }}</button>
            <button class="btn btn-sm btn-ghost" @click="annivForm = null">{{ $t('common.cancel') }}</button>
          </div>
        </div>
        <div v-if="annivLoading" class="fv-empty-sm">{{ $t('common.loading') }}</div>
        <template v-else-if="anniversaries.length">
          <div class="fv-anniv-soon">
            <div class="fv-pane-title">{{ $t('js.1989') }}</div>
            <div v-if="!soonAnniv.length" class="fv-empty-sm">{{ $t('js.1990') }}</div>
            <div v-else class="fv-anniv-grid">
              <div v-for="a in soonAnniv" :key="a.id" class="fv-anniv-card">
                <div class="fv-anniv-day">{{ fmtDay(a.next_date) }}</div>
                <div class="fv-anniv-info">
                  <b>{{ a.title }}</b>
                  <div class="muted">{{ memberName(a.ref_member_id) }}{{ a.remind_days ? ' · ' + $t('js.1991', { v0: a.remind_days }) : '' }}</div>
                </div>
              </div>
            </div>
          </div>
          <div class="fv-pane-title">{{ $t('js.1992') }}</div>
          <div class="fv-anniv-list">
            <div v-for="a in anniversaries" :key="a.id" class="fv-anniv-row" :class="{ off: !a.enabled }">
              <span class="fv-anniv-date">{{ a.date }}</span>
              <b>{{ a.title }}</b>
              <span class="muted">{{ annivKindLabel(a.kind) }} · {{ $t('js.1993') }} {{ a.next_date || '—' }}</span>
              <span class="fv-anniv-ops">
                <button class="btn btn-sm btn-ghost" :title="$t('common.rename')" @click="startEditAnniv(a)"><AikIcon name="edit" :size="12" /></button>
                <button class="btn btn-sm btn-danger-ghost" :title="$t('common.delete')" @click="deleteAnniv(a)"><AikIcon name="trash" :size="12" /></button>
              </span>
            </div>
          </div>
        </template>
        <div v-else class="fv-empty">
          <AikIcon name="book" :size="30" />
          <p>{{ $t('js.1994') }}</p>
          <p class="muted">{{ $t('js.1995') }}</p>
        </div>
      </div>

      <!-- ===== Tab4 共同空间 ===== -->
      <div v-show="tab === 'space'" class="fv-pane">
        <div class="fv-space-card">
          <AikIcon name="share" :size="34" />
          <h3>{{ $t('js.1996') }}</h3>
          <p>{{ $t('js.1997') }}</p>
          <ul class="fv-space-list">
            <li>{{ $t('js.1998') }}</li>
            <li>{{ $t('js.1999') }}</li>
            <li>{{ $t('js.2000') }}</li>
          </ul>
          <a class="btn btn-primary" href="#/collab">{{ $t('js.2001') }}</a>
        </div>
      </div>

      <!-- ===== Tab5 AI 人生参谋（二期①） ===== -->
      <div v-show="tab === 'advise'" class="fv-pane">
        <div class="fv-pane-tip">
          <AikIcon name="sparkles" :size="13" />{{ $t('js.2007') }}
        </div>
        <div class="fv-advise-form">
          <textarea v-model="adviseQuestion" class="input fv-advise-input" rows="3"
            :placeholder="$t('js.2008')" @keydown.enter.exact.prevent="askAdvise"></textarea>
          <div class="fv-form-ops">
            <button class="btn btn-primary" :disabled="!adviseQuestion.trim() || adviseLoading" @click="askAdvise">
              <AikIcon name="sparkles" :size="14" />{{ $t('js.2009') }}</button>
            <button v-if="advice" class="btn btn-sm btn-ghost" @click="advice = ''; adviseContext = ''">{{ $t('common.cancel') }}</button>
          </div>
        </div>
        <div v-if="adviseLoading" class="fv-empty-sm">{{ $t('js.2010') }}</div>
        <div v-else-if="advice" class="fv-advise-result">
          <div class="fv-pane-title">{{ $t('js.2011') }}</div>
          <div class="fv-advise-body" v-html="renderMarkdown(advice)"></div>
          <details v-if="adviseContext" class="fv-advise-ctx">
            <summary>{{ $t('js.2012') }}</summary>
            <pre class="fv-advise-pre">{{ adviseContext }}</pre>
          </details>
        </div>
        <div v-else class="fv-empty">
          <AikIcon name="chat" :size="30" />
          <p>{{ $t('js.2013') }}</p>
          <p class="muted">{{ $t('js.2014') }}</p>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import { requestJSON, updateSetting, authMe } from '@/api'
import {
  familyMembers, familyMemberCreate, familyMemberUpdate, familyMemberDelete,
  familyRelations, familyRelationCreate, familyRelationDelete,
  familyTimeline, familyAnniversaries, familyAnniversaryCreate,
  familyAnniversaryUpdate, familyAnniversaryDelete, familyAdvise
} from '@/api'
import { renderMarkdown } from '@/utils/markdown'

const tabs = [
  { key: 'timeline', icon: 'clock', label: 'js.2002' },
  { key: 'tree', icon: 'database', label: 'js.2003' },
  { key: 'anniv', icon: 'book', label: 'js.2004' },
  { key: 'space', icon: 'share', label: 'js.2005' },
  { key: 'advise', icon: 'sparkles', label: 'js.2006' }
]
const tab = ref('timeline')

const isAdmin = ref(false)

const swEnabled = ref(false)
const saving = ref(false)
const loading = ref(false)

const members = ref([])
const relations = ref([])
const timeline = ref([])
const anniversaries = ref([])
const membersLoading = ref(false)
const relationsLoading = ref(false)
const timelineLoading = ref(false)
const annivLoading = ref(false)

const memberForm = ref(null)
const memberEditing = ref(null)
const relForm = ref(null)
const annivForm = ref(null)
const annivEditing = ref(null)

// ---- 通用 ----
async function saveSwitch(key, val) {
  saving.value = true
  try {
    await updateSetting(key, val ? 'true' : 'false')
  } catch (e) { /* 开关失败保持现状，刷新回读 */ }
  saving.value = false
}
function fmtDate(ms) {
  if (!ms) return ''
  const d = new Date(ms)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
function fmtDay(ymd) {
  if (!ymd) return '—'
  const p = ymd.split('-')
  return p.length === 3 ? `${p[1]}-${p[2]}` : ymd
}
function genderLabel(g) {
  if (g === 'male') return '♂'
  if (g === 'female') return '♀'
  return ''
}
function memberName(id) {
  if (!id) return ''
  const m = members.value.find((x) => x.id === id)
  return m ? m.name : '?'
}
function relLabel(r) {
  const map = { parent_of: '父/母 →', child_of: '子/女 →', spouse_of: '配偶 →', sibling_of: '兄弟/姐妹 →', other: '关系 →' }
  return map[r] || r + ' →'
}
function annivKindLabel(k) {
  const map = { birthday: '生日', anniversary: '纪念日', custom: '自定义' }
  return map[k] || k
}
function stageLabel(s) {
  const map = { school: '求学成长', love: '恋爱结婚', parent: '育儿成长', career: '中年事业', retire: '退休养老', family: '家族传承', other: '其他' }
  return map[s] || s || '其他'
}

// ---- 时间轴 ----
const stageOrder = ['school', 'love', 'parent', 'career', 'retire', 'family', 'other', '']
const stageGroups = computed(() => {
  const groups = []
  const map = {}
  for (const s of stageOrder) {
    const key = s || ''
    map[key] = { stage: key, items: [] }
    groups.push(map[key])
  }
  for (const it of timeline.value) {
    const key = it.stage || ''
    ;(map[key] || map['']).items.push(it)
  }
  return groups.filter((g) => g.items.length)
})
function openEvent(it) {
  location.hash = it.link
}

// ---- 成员 ----
async function loadMembers() {
  membersLoading.value = true
  try { members.value = await familyMembers() } catch (e) { /* 模块未启用或错误 */ }
  membersLoading.value = false
}
function startEditMember(m) {
  memberForm.value = { name: m.name, gender: m.gender || '', birth_date: m.birth_date || '', note: m.note || '' }
  memberEditing.value = m.id
}
async function saveMember() {
  const payload = memberForm.value
  try {
    if (memberEditing.value) {
      await familyMemberUpdate(memberEditing.value, payload)
    } else {
      await familyMemberCreate(payload)
    }
  } catch (e) { /* 校验失败由后端提示 */ }
  memberForm.value = null
  memberEditing.value = null
  await Promise.all([loadMembers(), loadRelations()])
}
async function deleteMember(m) {
  if (!window.confirm('删除成员「' + m.name + '」？其关系与关联纪念日将一并删除。')) return
  try { await familyMemberDelete(m.id) } catch (e) { /* */ }
  await Promise.all([loadMembers(), loadRelations()])
}

// ---- 关系 ----
async function loadRelations() {
  relationsLoading.value = true
  try { relations.value = await familyRelations() } catch (e) { /* */ }
  relationsLoading.value = false
}
async function saveRelation() {
  try { await familyRelationCreate(relForm.value) } catch (e) { /* */ }
  relForm.value = null
  await loadRelations()
}
async function deleteRelation(r) {
  try { await familyRelationDelete(r.id) } catch (e) { /* */ }
  await loadRelations()
}

// ---- 纪念日 ----
async function loadAnniv() {
  annivLoading.value = true
  try { anniversaries.value = await familyAnniversaries() } catch (e) { /* */ }
  annivLoading.value = false
}
const soonAnniv = computed(() => {
  const now = new Date()
  const soon = new Date(now.getTime() + 7 * 864e5)
  return anniversaries.value.filter((a) => {
    if (!a.enabled || !a.next_date) return false
    const t = new Date(a.next_date)
    return t >= now && t <= soon
  }).sort((a, b) => a.next_date.localeCompare(b.next_date))
})
function startEditAnniv(a) {
  annivForm.value = {
    title: a.title, kind: a.kind || 'custom', date: a.date ? fullDate(a.date) : '',
    ref_member_id: a.ref_member_id || '', remind_days: a.remind_days || 0, enabled: !!a.enabled
  }
  annivEditing.value = a.id
}
function fullDate(mmdd) {
  // MM-DD → 今年日期（编辑用 date 控件）
  const y = new Date().getFullYear()
  return `${y}-${mmdd}`
}
function toMMDD(iso) {
  const p = (iso || '').split('-')
  return p.length === 3 ? `${p[1]}-${p[2]}` : iso
}
async function saveAnniv() {
  const payload = {
    ...annivForm.value,
    date: toMMDD(annivForm.value.date),
    ref_member_id: annivForm.value.ref_member_id || ''
  }
  try {
    if (annivEditing.value) await familyAnniversaryUpdate(annivEditing.value, payload)
    else await familyAnniversaryCreate(payload)
  } catch (e) { /* */ }
  annivForm.value = null
  annivEditing.value = null
  await loadAnniv()
}
async function deleteAnniv(a) {
  if (!window.confirm('删除纪念日「' + a.title + '」？')) return
  try { await familyAnniversaryDelete(a.id) } catch (e) { /* */ }
  await loadAnniv()
}

// ---- AI 人生参谋（二期①） ----
const adviseQuestion = ref('')
const adviseLoading = ref(false)
const advice = ref('')
const adviseContext = ref('')
async function askAdvise() {
  const q = adviseQuestion.value.trim()
  if (!q || adviseLoading.value) return
  adviseLoading.value = true
  advice.value = ''
  try {
    const d = await familyAdvise(q)
    advice.value = d.advice || ''
    adviseContext.value = d.context || ''
  } catch (e) {
    advice.value = '**请求失败**：' + (e && e.message ? e.message : '请稍后重试')
  }
  adviseLoading.value = false
}

// ---- 加载 ----
async function loadAll() {
  loading.value = true
  // [family] fork 适配：角色经 /auth/me 判定（fork 无 localStorage aikmap_me 缓存）
  try {
    const me = await authMe()
    isAdmin.value = me && me.user && (me.user.role === 'owner' || me.user.role === 'admin')
  } catch (e) { isAdmin.value = false }
  try {
    const s = await requestJSON('/family/settings')
    swEnabled.value = !!s.enabled
  } catch (e) { swEnabled.value = false }
  if (!swEnabled.value) { loading.value = false; return }
  await Promise.all([
    loadMembers(), loadRelations(), loadAnniv(),
    (async () => {
      timelineLoading.value = true
      try { timeline.value = await familyTimeline() } catch (e) { /* */ }
      timelineLoading.value = false
    })()
  ])
  loading.value = false
}

onMounted(loadAll)
</script>

<style scoped>
.fv-view { padding: 4px 2px 40px; }
.fv-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 10px; }
.fv-head h2 { margin: 0 0 4px; font-size: 18px; }
.fv-sub { margin: 0; color: var(--muted, #8a919c); font-size: 12.5px; }
.fv-tools { display: flex; gap: 8px; }
.fv-switches { display: flex; gap: 18px; background: var(--bg-soft, #f6f7f9); border-radius: 10px; padding: 10px 14px; margin-bottom: 10px; }
.fv-switch { display: flex; align-items: center; gap: 10px; cursor: pointer; }
.fv-switch-label b { font-size: 13.5px; }
.fv-switch-label .muted { font-size: 12px; margin-left: 4px; }
.fv-disabled { padding: 18px; text-align: center; color: var(--muted, #8a919c); background: var(--bg-soft, #f6f7f9); border-radius: 10px; }
.fv-tabs { display: flex; gap: 6px; margin-bottom: 12px; flex-wrap: wrap; }
.fv-tab { display: inline-flex; align-items: center; gap: 6px; padding: 7px 14px; border: 1px solid var(--border, #e3e6ea); border-radius: 8px; background: transparent; cursor: pointer; font-size: 13px; color: var(--muted, #5c6470); }
.fv-tab.active { background: var(--accent, #2f6fed); border-color: var(--accent, #2f6fed); color: #fff; }
.fv-pane { min-height: 120px; }
.fv-pane-tip { display: flex; align-items: center; gap: 6px; padding: 9px 12px; background: var(--bg-soft, #f6f7f9); border-radius: 8px; font-size: 12.5px; margin-bottom: 10px; color: var(--muted, #5c6470); }
.fv-pane-tip .link { margin-left: 4px; }
.fv-pane-title { display: flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 600; margin: 10px 0 8px; }
.fv-cnt { color: var(--muted, #8a919c); font-weight: 400; }
.fv-empty { padding: 34px 12px; text-align: center; color: var(--muted, #8a919c); }
.fv-empty p { margin: 6px 0 0; font-size: 14px; }
.fv-empty-sm { padding: 18px; text-align: center; color: var(--muted, #8a919c); font-size: 13px; }

/* 时间轴 */
.fv-stage-head { display: flex; align-items: baseline; gap: 8px; margin: 14px 0 6px; padding-top: 4px; border-top: 1px dashed var(--border, #e3e6ea); }
.fv-stage-head b { font-size: 13.5px; }
.fv-stage-cnt { color: var(--muted, #8a919c); font-size: 12px; }
.fv-event { display: flex; align-items: center; gap: 12px; padding: 9px 10px; border-radius: 8px; cursor: pointer; }
.fv-event:hover { background: var(--bg-soft, #f6f7f9); }
.fv-event-date { min-width: 84px; font-size: 12px; color: var(--muted, #8a919c); font-variant-numeric: tabular-nums; }
.fv-event-body { flex: 1; min-width: 0; }
.fv-event-title { font-size: 13.5px; font-weight: 500; }
.fv-event-preview { font-size: 12px; color: var(--muted, #5c6470); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.fv-event-go { color: var(--muted, #a6adb8); }

/* 家族树 */
.fv-two-col { display: grid; grid-template-columns: 1fr 1fr; gap: 18px; }
@media (max-width: 900px) { .fv-two-col { grid-template-columns: 1fr; } }
.fv-col { min-width: 0; }
.fv-inline-card { background: var(--bg-soft, #f6f7f9); border-radius: 10px; padding: 10px 12px; margin-bottom: 10px; }
.fv-form-row { display: flex; gap: 8px; margin-bottom: 8px; flex-wrap: wrap; }
.fv-form-row .input { flex: 1; min-width: 120px; }
.fv-form-ops { display: flex; gap: 8px; justify-content: flex-end; }
.fv-select { flex: 0 0 110px !important; }
.fv-member-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(210px, 1fr)); gap: 10px; }
.fv-member-card { display: flex; align-items: center; gap: 10px; padding: 10px; border: 1px solid var(--border, #e3e6ea); border-radius: 10px; }
.fv-member-avatar { width: 36px; height: 36px; border-radius: 50%; background: var(--accent-soft, #e8f0ff); color: var(--accent, #2f6fed); display: flex; align-items: center; justify-content: center; font-weight: 600; flex-shrink: 0; }
.fv-member-info { flex: 1; min-width: 0; }
.fv-member-info b { font-size: 13.5px; }
.fv-member-note { font-size: 12px; color: var(--muted, #5c6470); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.fv-member-ops { display: flex; gap: 4px; }
.fv-rel-list { display: flex; flex-direction: column; gap: 6px; }
.fv-rel-row { display: flex; align-items: center; gap: 8px; padding: 8px 10px; border: 1px solid var(--border, #e3e6ea); border-radius: 8px; font-size: 13px; }
.fv-rel-from, .fv-rel-to { font-weight: 500; }
.fv-rel-edge { color: var(--muted, #8a919c); font-size: 12px; flex: 1; }

/* 纪念日 */
.fv-remind { flex: 0 0 90px !important; }
.fv-anniv-soon { margin-bottom: 6px; }
.fv-anniv-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); gap: 10px; margin-bottom: 8px; }
.fv-anniv-card { display: flex; align-items: center; gap: 10px; padding: 10px; border: 1px solid var(--accent, #2f6fed); border-radius: 10px; background: var(--accent-soft, #eef4ff); }
.fv-anniv-day { font-size: 17px; font-weight: 700; color: var(--accent, #2f6fed); font-variant-numeric: tabular-nums; }
.fv-anniv-list { display: flex; flex-direction: column; gap: 6px; }
.fv-anniv-row { display: flex; align-items: center; gap: 10px; padding: 8px 10px; border: 1px solid var(--border, #e3e6ea); border-radius: 8px; font-size: 13px; }
.fv-anniv-row.off { opacity: 0.55; }
.fv-anniv-date { font-variant-numeric: tabular-nums; color: var(--muted, #5c6470); min-width: 52px; }
.fv-anniv-ops { margin-left: auto; display: flex; gap: 4px; }

/* 共同空间 */
.fv-space-card { max-width: 460px; margin: 14px auto; text-align: center; padding: 26px 20px; border: 1px solid var(--border, #e3e6ea); border-radius: 14px; }
.fv-space-card h3 { margin: 8px 0 6px; font-size: 16px; }
.fv-space-card p { color: var(--muted, #5c6470); font-size: 13px; margin: 0 0 10px; }
.fv-space-list { text-align: left; color: var(--muted, #5c6470); font-size: 13px; margin: 0 0 16px; padding-left: 20px; }
.fv-space-list li { margin: 4px 0; }
</style>
