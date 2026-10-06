<template>
  <div class="og-view">
    <div class="og-head">
      <div>
        <h2>{{  $t('组织架构')  }}</h2>
        <p class="og-sub">
          {{  $t('企业知识库的组织维度：组织树给内容打标、岗位快照记录成员归属、移交流负责人员变动时的资产交接。 子开关由服务端配置（')  }}<code>org.enabled</code> / <code>org.tree</code> / <code>org.department</code> / <code>org.transfer</code>）。
        </p>
      </div>
      <div class="og-tools">
        <button class="btn" :disabled="loading" @click="loadAll"><AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}</button>
      </div>
    </div>

    <div v-if="gates" class="og-gates">
      <span class="og-gate" :class="gates.enabled ? 'on' : 'off'">{{  $t('模块')  }} {{  gates.enabled ? $t('开') : $t('关')  }}</span>
      <span class="og-gate" :class="gates.tree ? 'on' : 'off'">{{  $t('组织树')  }} {{  gates.tree ? $t('开') : $t('关')  }}</span>
      <span class="og-gate" :class="gates.department ? 'on' : 'off'">{{  $t('部门空间')  }} {{  gates.department ? $t('开') : $t('关')  }}</span>
      <span class="og-gate" :class="gates.transfer ? 'on' : 'off'">{{  $t('移交流')  }} {{  gates.transfer ? $t('开') : $t('关')  }}</span>
      <span v-if="gates && !gates.enabled" class="og-hint">{{  $t('模块未启用时下列端点返回 403/404，属预期（能力未开启）。')  }}</span>
    </div>

    <div class="og-tabs">
      <button class="og-tab" :class="{ on: tab === 'tree' }" @click="tab = 'tree'">{{  $t('组织树')  }}</button>
      <button class="og-tab" :class="{ on: tab === 'transfer' }" @click="switchToTransfer()">
        {{  $t('移交流')  }} <span class="og-n">{{  transfers.length  }}</span>
      </button>
      <button class="og-tab" :class="{ on: tab === 'dept' }" @click="switchToDept()">{{  $t('部门空间')  }}</button>
    </div>

    <!-- ========== 组织树 ========== -->
    <div v-show="tab === 'tree'" class="og-main">
      <div class="og-card og-nodes">
        <div class="og-card-head">
          <b>{{  $t('组织节点')  }}</b>
          <span class="og-hint">{{  nodes.length  }} {{  $t('个')  }}</span>
          <button class="btn btn-sm og-right" @click="askRootName"><AikIcon name="plus" :size="13" />{{  $t('新建根节点')  }}</button>
        </div>
        <div v-if="!nodes.length" class="og-empty-sm">{{  $t('还没有组织节点，先建一个「公司」或「团队」。')  }}</div>
        <div
          v-for="n in nodes"
          :key="n.id"
          class="og-node"
          :class="{ sel: curNode && curNode.id === n.id }"
          :style="{ paddingLeft: 14 + depth(n) * 16 + 'px' }"
          @click="selectNode(n)"
        >
          <AikIcon name="grid" :size="13" />
          <span class="og-node-name" :title="n.path">{{  n.name  }}</span>
          <span class="og-node-nums">{{  n.member_count  }}{{  $t('人 ·')  }} {{  n.file_count  }}{{  $t('文件')  }}</span>
        </div>
      </div>

      <div class="og-card og-detail">
        <template v-if="curNode">
          <div class="og-card-head">
            <b>{{  curNode.name  }}</b>
            <span class="og-hint">{{  curNode.path  }}</span>
          </div>
          <div class="og-form">
            <div class="og-frow">
              <label>{{  $t('节点名称')  }}</label>
              <div class="og-inline">
                <input v-model="nodeForm.name" class="input" />
                <button class="btn btn-sm" :disabled="busy" @click="renameNode">{{  $t('改名')  }}</button>
              </div>
            </div>
            <div class="og-frow">
              <label>{{  $t('父节点（移动）')  }}</label>
              <div class="og-inline">
                <select v-model="nodeForm.parentId" class="input">
                  <option value="">{{  $t('— 顶层 —')  }}</option>
                  <option v-for="n in otherNodes" :key="n.id" :value="n.id">{{  n.path  }}</option>
                </select>
                <button class="btn btn-sm" :disabled="busy" @click="moveNode">{{  $t('移动')  }}</button>
              </div>
            </div>
            <div class="og-frow">
              <label>{{  $t('子节点')  }}</label>
              <div class="og-inline">
                <input v-model="nodeForm.childName" class="input" :placeholder="$t('新子节点名称')" />
                <button class="btn btn-sm" :disabled="busy" @click="createChild">{{  $t('添加')  }}</button>
              </div>
            </div>
            <div class="og-actions">
              <button class="btn btn-sm" :disabled="busy || gates.department === false" @click="createDeptFromNode">
                <AikIcon name="grid" :size="13" />{{  $t('建为部门空间')  }}
              </button>
              <button class="btn btn-sm btn-danger-ghost" :disabled="busy" @click="confirmNodeDel = curNode">
                <AikIcon name="trash" :size="13" />{{  $t('删除节点')  }}
              </button>
            </div>
          </div>

          <div class="og-card-head og-sub-head">
            <b>{{  $t('在任成员')  }}</b>
            <span class="og-hint">{{  members.length  }} {{  $t('人（岗位变更唯一入口，写岗位历史）')  }}</span>
          </div>
          <div v-if="!members.length" class="og-empty-sm">{{  $t('该节点暂无在任成员')  }}</div>
          <div v-for="m in members" :key="m.id" class="og-row">
            <span class="og-row-name">{{  userLabel(m.user_id)  }}</span>
            <span class="og-hint">{{  m.path || curNode.path  }}</span>
            <span class="og-badge">{{  m.until ? $t('历史') : $t('在任')  }}</span>
            <span class="og-hint">{{  fmtTime(m.since)  }}</span>
          </div>

          <div class="og-form og-assign">
            <label>{{  $t('设置岗位（把成员挂到本节点）')  }}</label>
            <div class="og-inline">
              <select v-if="users.length" v-model="assign.userId" class="input">
                <option value="">{{  $t('选择用户…')  }}</option>
                <option v-for="u in users" :key="u.id" :value="u.id">{{  u.display_name || u.username  }}</option>
              </select>
              <input v-else v-model="assign.userId" class="input" :placeholder="$t('用户 ID（无用户列表权限时手填）')" />
              <button class="btn btn-sm btn-primary" :disabled="busy || !assign.userId" @click="doAssign">{{  $t('挂岗')  }}</button>
            </div>
            <p class="og-hint">{{  $t('挂岗会关闭该成员在同一节点的历史岗位并新增一条记录，用于资产归属追溯。')  }}</p>
          </div>
        </template>
        <div v-else class="og-empty">{{ $t('选择左侧节点查看详情') }}</div>
      </div>
    </div>

    <!-- ========== 移交流 ========== -->
    <div v-show="tab === 'transfer'" class="og-main">
      <div class="og-card og-transfers">
        <div class="og-card-head">
          <b>{{ $t('移交工单') }}</b>
          <div class="og-scope">
            <button class="og-scope-btn" :class="{ on: scope === 'out' }" @click="setScope('out')">{{ $t('我发起') }}</button>
            <button class="og-scope-btn" :class="{ on: scope === 'in' }" @click="setScope('in')">{{ $t('待我接收') }}</button>
          </div>
        </div>
        <div v-if="!transfers.length" class="og-empty-sm">{{ $t('没有工单') }}</div>
        <div
          v-for="t in transfers"
          :key="t.id"
          class="og-tr"
          :class="{ sel: curTransfer && curTransfer.id === t.id }"
          @click="selectTransfer(t)"
        >
          <div class="og-tr-top">
            <span class="og-badge" :class="'st-' + t.status">{{  statusLabel(t.status)  }}</span>
            <b>{{  t.type === 'resign' ? $t('离职移交') : $t('调岗移交')  }}</b>
            <span class="og-hint">{{  userLabel(t.from_user)  }} → {{  userLabel(t.to_user)  }}</span>
            <span class="og-hint og-right">{{  fmtTime(t.created_at)  }}</span>
          </div>
          <div v-if="t.note" class="og-tr-note">{{  t.note  }}</div>
        </div>
      </div>

      <div class="og-card og-detail">
        <template v-if="curTransfer">
          <div class="og-card-head">
            <b>{{ $t('工单详情') }}</b>
            <span class="og-hint">{{  curTransfer.id  }}</span>
          </div>
          <div class="og-form">
            <div class="og-actions og-actions-left">
              <button class="btn btn-sm" :disabled="busy" @click="doPreview"><AikIcon name="list" :size="13" />{{ $t('预演清单') }}</button>
              <button class="btn btn-sm" :disabled="busy || curTransfer.status === 'executed'" @click="doExecute">
                <AikIcon name="check" :size="13" />{{ $t('执行移交') }}
              </button>
              <button class="btn btn-sm btn-primary" :disabled="busy || curTransfer.status !== 'executed'" @click="doAccept">
                <AikIcon name="check" :size="13" />{{ $t('确认接收') }}
              </button>
              <button class="btn btn-sm btn-danger-ghost" :disabled="busy || ['accepted', 'cancelled'].includes(curTransfer.status)" @click="doCancel">{{ $t('取消') }}</button>
            </div>
            <p class="og-hint">{{ $t('流程：发起(draft) → 预演 → 执行 → 接收人确认(accepted/归档) → 取消可随时中止。') }}</p>
          </div>

          <div class="og-card-head og-sub-head">
            <b>{{ $t('资产清单') }}</b>
            <span class="og-hint">{{  items.length  }} {{ $t('项（勾选 = 排除不交）') }}</span>
          </div>
          <div v-if="!items.length" class="og-empty-sm">{{ $t('还没有清单，先点「预演清单」生成。') }}</div>
          <div v-for="it in items" :key="it.id" class="og-row">
            <input
              type="checkbox"
              :checked="it.status !== 'skipped'"
              :disabled="busy"
              @change="toggleItem(it, $event.target.checked)"
            />
            <span class="og-row-name">{{  it.asset_name || it.asset_id  }}</span>
            <span class="og-badge">{{  it.asset_type  }}</span>
            <span class="og-hint">{{  it.action  }}</span>
            <span class="og-badge" :class="'st-' + it.status">{{  it.status  }}</span>
          </div>
        </template>
        <div v-else class="og-empty">{{ $t('选择左侧工单查看详情') }}</div>
      </div>
    </div>

    <!-- ========== 部门空间 ========== -->
    <div v-show="tab === 'dept'" class="og-main">
      <div class="og-card og-nodes">
        <div class="og-card-head">
          <b>{{ $t('部门空间') }}</b>
          <span class="og-hint">{{  depts.length  }} {{ $t('个') }}</span>
        </div>
        <div v-if="!depts.length" class="og-empty-sm">{{ $t('还没有部门空间。先在「组织树」选节点 → 「建为部门空间」。') }}</div>
        <div
          v-for="d in depts"
          :key="d.id"
          class="og-node"
          :class="{ sel: curDept && curDept.id === d.id }"
          @click="selectDept(d)"
        >
          <AikIcon name="grid" :size="13" />
          <span class="og-node-name">{{  d.name  }}</span>
          <span class="og-node-nums">{{  d.role || '—'  }}</span>
        </div>
      </div>

      <div class="og-card og-detail">
        <template v-if="curDept">
          <div class="og-card-head">
            <b>{{  curDept.name  }}</b>
            <span class="og-hint">{{ $t('节点') }} {{  curDept.node_path || curDept.node_id  }} {{ $t('· 负责人') }} {{  curDept.leader_name || curDept.leader_id || '—'  }}</span>
          </div>
          <div class="og-form">
            <label>{{ $t('加成员（部门级授权，不写岗位历史）') }}</label>
            <div class="og-inline">
              <select v-if="users.length" v-model="deptMember.userId" class="input">
                <option value="">{{ $t('选择用户…') }}</option>
                <option v-for="u in users" :key="u.id" :value="u.id">{{  u.display_name || u.username  }}</option>
              </select>
              <input v-else v-model="deptMember.userId" class="input" :placeholder="$t('用户 ID')" />
              <select v-model="deptMember.role" class="input og-role">
                <option value="editor">editor</option>
                <option value="viewer">viewer</option>
              </select>
              <button class="btn btn-sm btn-primary" :disabled="busy || !deptMember.userId" @click="addDeptMember">{{ $t('加入') }}</button>
            </div>
            <p class="og-hint">{{ $t('部门空间是付费能力（Pro）；站点 owner/admin 豁免。') }}</p>
          </div>
        </template>
        <div v-else class="og-empty">{{ $t('选择左侧部门查看详情') }}</div>
      </div>
    </div>

    <!-- 发起移交 -->
    <div class="og-card og-create">
      <div class="og-card-head">
        <b>{{ $t('发起移交') }}</b>
        <span class="og-hint">{{ $t('我名下文件') }} {{  myCount  }} {{ $t('个') }}</span>
      </div>
      <div class="og-form og-form-row">
        <div class="og-frow">
          <label>{{ $t('类型') }}</label>
          <select v-model="createForm.type" class="input">
            <option value="transfer">{{ $t('调岗移交') }}</option>
            <option value="resign">{{ $t('离职移交') }}</option>
          </select>
        </div>
        <div class="og-frow">
          <label>{{ $t('接手人') }}</label>
          <select v-if="users.length" v-model="createForm.toUser" class="input">
            <option value="">{{ $t('选择用户…') }}</option>
            <option v-for="u in users" :key="u.id" :value="u.id">{{  u.display_name || u.username  }}</option>
          </select>
          <input v-else v-model="createForm.toUser" class="input" :placeholder="$t('用户 ID')" />
        </div>
        <div class="og-frow og-frow-wide">
          <label>{{ $t('备注') }}</label>
          <input v-model="createForm.note" class="input" :placeholder="$t('选填，如「离职交接，7 日内完成」')" />
        </div>
        <div class="og-frow og-frow-btn">
          <label>&nbsp;</label>
          <button class="btn btn-primary" :disabled="busy || !createForm.toUser" @click="doCreateTransfer">{{ $t('发起') }}</button>
        </div>
      </div>
      <p class="og-hint">{{ $t('不勾选具体资产时，预演会按「我作为责任人的全部文件」生成清单。') }}</p>
    </div>

    <div v-if="mini.open" class="modal-mask" @click.self="mini.open = false">
      <div class="modal-box og-modal">
        <h3 class="og-modal-title">{{  mini.title  }}</h3>
        <div class="og-frow">
          <label>{{  mini.label  }}</label>
          <input v-model="mini.value" class="input" :placeholder="mini.placeholder" @keyup.enter="miniOk" />
        </div>
        <div class="og-modal-actions">
          <button class="btn" @click="mini.open = false">{{ $t('取消') }}</button>
          <button class="btn btn-primary" :disabled="!mini.value.trim()" @click="miniOk">{{ $t('确定') }}</button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      v-if="confirmNodeDel"
      :title="$t('删除组织节点')"
      :message="$t('删除节点「{v0}」？若节点仍有成员或挂载文件，服务端会拒绝。', { v0: confirmNodeDel.path })"
      confirm-text="删除"
      danger
      @confirm="doDeleteNode"
      @cancel="confirmNodeDel = null"
    />
  </div>
</template>

<script setup>
import { t as i18t } from '@/i18n'
import { ref, computed, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const toast = useToastStore()

const tab = ref('tree')
const loading = ref(false)
const busy = ref(false)
const gates = ref(null)

const nodes = ref([])
const curNode = ref(null)
const members = ref([])
const nodeForm = ref({ name: '', parentId: '', childName: '' })
const confirmNodeDel = ref(null)

const users = ref([])
const assign = ref({ userId: '' })

const scope = ref('out')
const transfers = ref([])
const curTransfer = ref(null)
const items = ref([])
const myCount = ref(0)
const createForm = ref({ type: 'transfer', toUser: '', note: '' })

const depts = ref([])
const curDept = ref(null)
const deptMember = ref({ userId: '', role: 'editor' })

const otherNodes = computed(() => nodes.value.filter((n) => !curNode.value || n.id !== curNode.value.id))

function depth(n) {
  const p = n.path || n.name || ''
  return Math.max(0, p.split('/').filter(Boolean).length - 1)
}

function userLabel(id) {
  if (!id) return '—'
  const u = users.value.find((x) => x.id === id)
  return u ? u.display_name || u.username : id
}

function statusLabel(s) {
  const map = {
    draft: t(i18t('草稿')),
    previewed: t('已预演'),
    executed: t('已执行'),
    accepted: t('已接收'),
    archived: t('已归档'),
    cancelled: t('已取消')
  }
  return map[s] || s || '—'
}

function fmtTime(ts) {
  if (!ts) return '—'
  const d = new Date(ts < 1e12 ? ts * 1000 : ts)
  const p = (x) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

async function loadAll() {
  loading.value = true
  try {
    const g = await api.orgSettings().catch(() => null)
    gates.value = g?.org || null
  } finally {
    loading.value = false
  }
  // 总开关关闭时后端对所有 org 端点一律 403（门控语义），前端不再盲发请求：
  // 否则每次进页面都会弹「加载组织树失败」红 toast，与页面提示「模块未启用属预期」自相矛盾。
  if (!gates.value?.enabled) {
    nodes.value = []
    members.value = []
    transfers.value = []
    myCount.value = 0
    return
  }
  await Promise.all([loadNodes(), loadUsers()])
  await loadTransfers()
}

async function loadNodes() {
  try {
    const d = await api.orgTreeList()
    nodes.value = d.items || []
    if (curNode.value) {
      const still = nodes.value.find((n) => n.id === curNode.value.id)
      if (still) {
        curNode.value = still
        nodeForm.value.name = still.name
        await loadMembers(still.id)
      } else {
        curNode.value = null
        members.value = []
      }
    }
  } catch (e) {
    nodes.value = []
    toast.push(t('加载组织树失败：') + (e.message || e))
  }
}

async function loadUsers() {
  try {
    const d = await api.adminUsersList()
    users.value = d.users || []
  } catch (_) {
    users.value = []
  }
}

async function selectNode(n) {
  curNode.value = n
  nodeForm.value = { name: n.name, parentId: n.parent_id || '', childName: '' }
  await loadMembers(n.id)
}

async function loadMembers(nodeId) {
  try {
    const d = await api.orgNodeMembers(nodeId)
    members.value = d.items || []
  } catch (_) {
    members.value = []
  }
}

async function renameNode() {
  if (!curNode.value || !nodeForm.value.name.trim()) return
  busy.value = true
  try {
    await api.orgTreeUpdate(curNode.value.id, { name: nodeForm.value.name.trim() })
    toast.push(t('已改名'))
    await loadNodes()
  } catch (e) {
    toast.push(t('改名失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function moveNode() {
  if (!curNode.value) return
  busy.value = true
  try {
    await api.orgTreeUpdate(curNode.value.id, { parent_id: nodeForm.value.parentId })
    toast.push(t('已移动'))
    await loadNodes()
  } catch (e) {
    toast.push(t('移动失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

const mini = ref({ open: false, title: '', label: '', value: '', placeholder: '', onOk: null })

function openMini(title, label, value, placeholder, onOk) {
  mini.value = { open: true, title, label, value: value || '', placeholder: placeholder || '', onOk }
}

function miniOk() {
  const v = (mini.value.value || '').trim()
  if (!v) return
  const fn = mini.value.onOk
  mini.value.open = false
  if (fn) fn(v)
}

function askRootName() {
  openMini(t('新建根节点'), t('节点名称'), '', t('如 公司 / 团队'), (v) => doCreateNode(v, ''))
}

async function doCreateNode(name, parentId) {
  busy.value = true
  try {
    await api.orgTreeCreate(name, parentId)
    toast.push(t('节点已创建'))
    await loadNodes()
  } catch (e) {
    toast.push(t('创建节点失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function createChild() {
  const name = nodeForm.value.childName.trim()
  if (!curNode.value || !name) return
  await doCreateNode(name, curNode.value.id)
  nodeForm.value.childName = ''
}

async function doDeleteNode() {
  const n = confirmNodeDel.value
  confirmNodeDel.value = null
  if (!n) return
  busy.value = true
  try {
    await api.orgTreeDelete(n.id)
    if (curNode.value && curNode.value.id === n.id) curNode.value = null
    toast.push(i18t('节点已删除'))
    await loadNodes()
  } catch (e) {
    toast.push(t(i18t('删除失败：')) + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function doAssign() {
  if (!curNode.value || !assign.value.userId) return
  busy.value = true
  try {
    await api.orgMembershipsSet({ user_id: assign.value.userId, node_id: curNode.value.id })
    toast.push(t('岗位已设置'))
    assign.value.userId = ''
    await Promise.all([loadMembers(curNode.value.id), loadNodes()])
  } catch (e) {
    toast.push(t('挂岗失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

/* ---- 移交流 ---- */
function switchToTransfer() {
  tab.value = 'transfer'
  loadTransfers()
}

async function setScope(s) {
  scope.value = s
  await loadTransfers()
}

async function loadTransfers() {
  try {
    const d = await api.orgTransfers(scope.value)
    transfers.value = d.items || []
  } catch (_) {
    transfers.value = []
  }
  api.orgCustodianCount().then((d) => { myCount.value = d.count || 0 }).catch(() => {})
}

async function selectTransfer(t) {
  curTransfer.value = t
  items.value = []
  try {
    const d = await api.orgTransferItems(t.id)
    items.value = d.items || []
  } catch (_) {
    items.value = []
  }
}

async function doPreview() {
  if (!curTransfer.value) return
  busy.value = true
  try {
    const d = await api.orgTransferPreview(curTransfer.value.id)
    items.value = d.items || []
    toast.push(`预演完成，共 ${items.value.length} 项`)
    await loadTransfers()
  } catch (e) {
    toast.push(t('预演失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function doExecute() {
  if (!curTransfer.value) return
  busy.value = true
  try {
    await api.orgTransferExecute(curTransfer.value.id)
    toast.push(t('已执行，等待接手人确认'))
    await loadTransfers()
    await selectTransfer({ ...curTransfer.value, status: 'executed' })
  } catch (e) {
    toast.push(t('执行失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function doAccept() {
  if (!curTransfer.value) return
  busy.value = true
  try {
    await api.orgTransferAccept(curTransfer.value.id)
    toast.push(t('已确认接收'))
    await loadTransfers()
  } catch (e) {
    toast.push(t('确认失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function doCancel() {
  if (!curTransfer.value) return
  busy.value = true
  try {
    await api.orgTransferCancel(curTransfer.value.id)
    toast.push(t('工单已取消'))
    await loadTransfers()
  } catch (e) {
    toast.push(t('取消失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function toggleItem(it, checked) {
  if (!curTransfer.value) return
  busy.value = true
  try {
    await api.orgTransferItemSkip(curTransfer.value.id, it.id, !checked)
    it.status = checked ? 'pending' : 'skipped'
  } catch (e) {
    toast.push(t('更新清单失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function doCreateTransfer() {
  const f = createForm.value
  if (!f.toUser) return
  busy.value = true
  try {
    await api.orgTransferCreate({ type: f.type, to_user: f.toUser, note: f.note })
    toast.push(t('移交工单已发起（草稿），请先预演'))
    createForm.value = { type: 'transfer', toUser: '', note: '' }
    await loadTransfers()
  } catch (e) {
    toast.push(t('发起失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

/* ---- 部门空间 ---- */
function switchToDept() {
  tab.value = 'dept'
  loadDepts()
}

async function loadDepts() {
  try {
    const d = await api.orgDepartments()
    depts.value = d.items || []
  } catch (_) {
    depts.value = []
  }
}

function selectDept(d) {
  curDept.value = d
  deptMember.value = { userId: '', role: 'editor' }
}

function createDeptFromNode() {
  if (!curNode.value) return
  const nodeId = curNode.value.id
  const def = curNode.value.name
  openMini(t('建为部门空间'), t('部门名称'), def, '', (v) => doCreateDept(nodeId, v))
}

async function doCreateDept(nodeId, name) {
  busy.value = true
  try {
    await api.orgDepartmentCreate(nodeId, name)
    toast.push(t('部门空间已创建'))
    await loadDepts()
  } catch (e) {
    toast.push(t('创建部门失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function addDeptMember() {
  if (!curDept.value || !deptMember.value.userId) return
  busy.value = true
  try {
    await api.orgDepartmentMembers(curDept.value.id, {
      user_id: deptMember.value.userId,
      role: deptMember.value.role
    })
    toast.push(t('成员已加入'))
    deptMember.value.userId = ''
  } catch (e) {
    toast.push(t('加入失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

onMounted(loadAll)
</script>

<style scoped>
.og-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.og-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; margin-bottom: 12px; }
.og-head h2 { font-size: 20px; margin: 0 0 4px; }
.og-sub { color: var(--text-3, #8a919f); font-size: 13px; margin: 0; max-width: 800px; line-height: 1.6; }
.og-sub code { font-size: 12px; padding: 1px 5px; border-radius: 4px; background: var(--bg-3, #f0f1f4); }
.og-tools { display: inline-flex; gap: 8px; flex-shrink: 0; }

.og-gates { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 12px; }
.og-gate { font-size: 11.5px; padding: 2px 9px; border-radius: 9px; background: var(--bg-3, #f0f1f4); color: var(--text-3, #8a919f); }
.og-gate.on { background: rgba(46,164,79,.14); color: #2ea44f; }
.og-gate.off { background: rgba(224,62,62,.10); color: #c04a4a; }

.og-tabs { display: inline-flex; gap: 4px; padding: 3px; border-radius: 9px; background: var(--surface-2, #f7f8fa); margin-bottom: 14px; }
.og-tab { border: 0; background: transparent; padding: 6px 16px; border-radius: 7px; font-size: 13px; color: var(--text-2, #4a5164); cursor: pointer; }
.og-tab.on { background: var(--bg-1, #fff); color: var(--primary, #2f6bff); font-weight: 600; box-shadow: 0 1px 3px rgba(22,24,43,.08); }
.og-n { font-size: 11px; color: var(--text-3, #8a919f); }

.og-main { display: flex; gap: 14px; align-items: flex-start; margin-bottom: 14px; }
.og-nodes { flex: 0 0 330px; max-height: 560px; overflow-y: auto; }
.og-detail { flex: 1; min-width: 0; }

.og-card { border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--bg-2, #fff); margin-bottom: 12px; }
.og-card-head { display: flex; align-items: baseline; gap: 8px; padding: 11px 14px; border-bottom: 1px solid var(--border, #ececf1); }
.og-card-head b { font-size: 14px; }
.og-sub-head { border-top: 1px solid var(--border, #ececf1); }
.og-hint { font-size: 11.5px; color: var(--text-3, #8a919f); }
.og-right { margin-left: auto; }

.og-node { display: flex; align-items: center; gap: 8px; padding: 8px 14px; border-bottom: 1px solid var(--border, #f2f3f7); cursor: pointer; font-size: 13px; }
.og-node:last-child { border-bottom: 0; }
.og-node:hover { background: var(--surface-2, #f7f8fa); }
.og-node.sel { background: var(--primary-soft, #eaf1ff); }
.og-node-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text, #20242c); }
.og-node-nums { font-size: 11px; color: var(--text-3, #8a919f); flex-shrink: 0; }

.og-form { padding: 12px 14px; display: flex; flex-direction: column; gap: 10px; }
.og-form-row { flex-direction: row; flex-wrap: wrap; gap: 12px; }
.og-form-row .og-frow { flex: 1 1 180px; }
.og-frow { display: flex; flex-direction: column; gap: 5px; }
.og-frow label { font-size: 12.5px; color: var(--text-2, #4a5164); }
.og-frow-wide { flex: 2 1 260px; }
.og-frow-btn { flex: 0 0 auto; }
.og-inline { display: flex; gap: 8px; align-items: center; }
.og-inline .input { flex: 1; min-width: 0; }
.og-role { flex: 0 0 110px; }
.og-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.og-actions-left { justify-content: flex-start; }
.og-assign { border-top: 1px solid var(--border, #ececf1); }

.og-scope { margin-left: auto; display: inline-flex; gap: 4px; }
.og-scope-btn { border: 0; background: var(--bg-3, #f0f1f4); padding: 3px 10px; border-radius: 7px; font-size: 11.5px; color: var(--text-2, #4a5164); cursor: pointer; }
.og-scope-btn.on { background: var(--primary-soft, #eaf1ff); color: var(--primary, #2f6bff); font-weight: 600; }

.og-tr { padding: 10px 14px; border-bottom: 1px solid var(--border, #f2f3f7); cursor: pointer; }
.og-tr:last-child { border-bottom: 0; }
.og-tr:hover { background: var(--surface-2, #f7f8fa); }
.og-tr.sel { background: var(--primary-soft, #eaf1ff); }
.og-tr-top { display: flex; align-items: center; gap: 8px; font-size: 13px; flex-wrap: wrap; }
.og-tr-note { font-size: 12px; color: var(--text-3, #8a919f); margin-top: 4px; }

.og-row { display: flex; align-items: center; gap: 10px; padding: 8px 14px; border-bottom: 1px solid var(--border, #f2f3f7); font-size: 12.5px; }
.og-row:last-child { border-bottom: 0; }
.og-row-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text, #20242c); }
.og-badge { font-size: 11px; padding: 1px 7px; border-radius: 8px; background: var(--bg-3, #f0f1f4); color: var(--text-3, #8a919f); flex-shrink: 0; }
.og-badge.st-accepted, .og-badge.st-archived { background: rgba(46,164,79,.14); color: #2ea44f; }
.og-badge.st-executed, .og-badge.st-previewed { background: rgba(214,158,46,.16); color: #b8860b; }
.og-badge.st-cancelled, .og-badge.st-failed { background: rgba(224,62,62,.12); color: #e03e3e; }
.og-badge.st-skipped { background: var(--bg-3, #f0f1f4); color: var(--text-3, #a0a6b2); }

.og-empty { padding: 60px 14px; text-align: center; color: var(--text-3, #8a919f); font-size: 13px; }
.og-empty-sm { padding: 18px 14px; color: var(--text-3, #8a919f); font-size: 12.5px; }

.og-create .og-form-row { padding-bottom: 4px; }

.modal-mask { position: fixed; inset: 0; z-index: 400; background: rgba(22,24,43,.45); display: flex; align-items: center; justify-content: center; }
.modal-box { background: var(--surface, #fff); border-radius: var(--radius, 12px); padding: 22px 24px; box-shadow: 0 20px 50px rgba(22,24,43,.2); }
.og-modal { width: 420px; max-width: calc(100vw - 40px); display: flex; flex-direction: column; gap: 12px; }
.og-modal-title { margin: 0; font-size: 16px; color: var(--text, #20242c); }
.og-modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 4px; }

.input { width: 100%; box-sizing: border-box; height: 32px; padding: 0 10px; border-radius: var(--radius-sm, 8px); border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 12.5px; outline: none; transition: border-color .15s, box-shadow .15s; }
.input:focus { border-color: var(--primary, #2f6bff); background: var(--bg-1, #fff); box-shadow: 0 0 0 3px rgba(47,107,255,.12); }
</style>
