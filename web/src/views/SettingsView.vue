<template>
  <div class="settings-view">
    <div class="sv-head">
      <div>
        <h2>设置</h2>
        <p class="sv-sub">站点 · AI · 消息 · 应用 · 数据</p>
      </div>
    </div>

    <!-- 顶部分组标签（?tab= 深链可直达） -->
    <nav class="sv-nav-tabs">
      <button v-for="t in navTabs" :key="t.key" class="sv-tab-btn" :class="{ on: tab === t.key }" @click="switchTab(t.key)">
        {{ t.label }}<span v-if="t.count" class="sv-tab-count">{{ t.count }}</span>
      </button>
    </nav>

    <!-- 站点（多用户：开放注册开关） -->
    <section v-if="multiUser" v-show="tab === 'site'" class="sv-card">
      <div class="sv-card-h">
        <h3>站点</h3>
        <p>开放注册开关：关闭后新用户无法自助注册（已注册用户不受影响）——分发场景建议开启，私有部署建议关闭</p>
      </div>
      <div class="sv-cap">
        <div class="sv-cap-info">
          <span class="sv-cap-name">开放注册</span>
          <span class="sv-badge" :class="{ on: registrationOpen }">{{ registrationOpen ? '已开启' : '已关闭' }}</span>
        </div>
        <div class="sv-cap-actions">
          <button class="btn btn-sm" :disabled="saving" @click="toggleRegistration">
            {{ registrationOpen ? '关闭注册' : '开启注册' }}
          </button>
        </div>
      </div>
    </section>

    <!-- AI 模型（能力级 provider 绑定） -->
    <section v-show="tab === 'ai'" class="sv-card">
      <div class="sv-card-h">
        <h3>AI 模型</h3>
        <p>各能力独立绑定 provider 与模型，切换即时生效；「自备模型」支持任意 OpenAI 兼容接口（含本地 Ollama）</p>
      </div>
      <div v-for="cap in caps" :key="cap.name" class="sv-cap">
        <div class="sv-cap-info">
          <span class="sv-cap-name">{{ cap.label }}</span>
          <span class="sv-badge" :class="{ on: !!cap.active }">{{ cap.active || '未配置' }}</span>
          <span v-if="cap.model" class="sv-cap-model">{{ cap.model }}</span>
          <span v-if="cap.custom?.configured" class="sv-custom-tag">自备</span>
        </div>
        <div class="sv-cap-actions">
          <select :value="cap.active" @change="onSwitch(cap.name, $event)">
            <option :value="cap.active" disabled v-if="cap.active">{{ cap.active }}</option>
            <option value="">选择 provider…</option>
            <option v-for="o in cap.options" :key="o" :value="o" :disabled="o === cap.active">{{ o }}</option>
          </select>
          <button class="btn btn-sm" @click="openCustom(cap.name)">{{ customOpen === cap.name ? '收起' : '自备模型' }}</button>
        </div>
        <div v-if="customOpen === cap.name" class="sv-custom">
          <input v-model="customForm.endpoint" placeholder="API 地址（OpenAI 兼容，如 http://127.0.0.1:11434/v1）" />
          <input v-model="customForm.model" placeholder="模型名（如 bge-m3 / glm-5.2）" />
          <input v-model="customForm.apiKey" type="password" placeholder="API Key（本地服务可留空）" />
          <div class="sv-custom-actions">
            <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveCustom(cap.name)">保存</button>
            <button v-if="cap.custom?.configured" class="btn btn-sm" @click="clearCustom(cap.name)">清除自备</button>
          </div>
        </div>
      </div>
    </section>

    <!-- IM 对接 -->
    <section v-show="tab === 'im'" class="sv-card">
      <div class="sv-card-h">
        <h3>IM 对接</h3>
        <p>对接 Telegram / 企业微信：发消息即可发博客。命令：<code>/post 正文</code> 发布、<code>/draft 正文</code> 存草稿、<code>/search 关键词</code> 检索、<code>/list</code> 最近文章、<code>/help</code> 帮助</p>
      </div>

      <!-- Telegram -->
      <div class="sv-im">
        <div class="sv-im-block-h">
          <span class="sv-im-plt">Telegram</span>
          <span class="sv-badge" :class="{ on: imTgOn }">{{ imTgOn ? '已启用' : '未启用' }}</span>
        </div>
        <div class="sv-im-state-row">
          <span v-if="imTgOn" class="sv-muted">在 Telegram 搜索你的 Bot，发送 <code>/post 正文</code> 即可发博客</span>
          <span v-else class="sv-muted">填写 Bot Token 并保存后启用</span>
        </div>
        <label>Bot Token</label>
        <input v-model="tg.botToken" type="password" placeholder="123456789:AAH..." />
        <label>允许的 Chat ID（逗号分隔，留空 = 全部）</label>
        <input v-model="tg.allowedChats" placeholder="如 123456789, 987654321" />
        <div class="sv-im-actions">
          <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveIM">保存配置</button>
          <span class="sv-muted">Webhook：<code>{{ tgHook }}</code></span>
        </div>
      </div>

      <!-- 企业微信 -->
      <div class="sv-im">
        <div class="sv-im-block-h">
          <span class="sv-im-plt">企业微信</span>
          <span class="sv-badge" :class="{ on: imWcOn }">{{ imWcOn ? '已启用' : '未启用' }}</span>
        </div>
        <div class="sv-im-state-row">
          <span v-if="imWcOn" class="sv-muted">在企业微信应用/群机器人收发消息，发 <code>/post 正文</code> 即可发博客</span>
          <span v-else class="sv-muted">填企业微信自建应用信息并保存后启用（群机器人 Webhook 或自建应用二选一）</span>
        </div>
        <label>群机器人 Webhook 地址（任选其一）</label>
        <input v-model="wc.webhookUrl" placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..." />
        <label>自建应用 CorpID</label>
        <input v-model="wc.corpId" placeholder="企业 ID" />
        <label>AgentID</label>
        <input v-model="wc.agentId" placeholder="应用 AgentId" />
        <label>Secret</label>
        <input v-model="wc.secret" type="password" placeholder="应用 Secret" />
        <label>回调 Token</label>
        <input v-model="wc.token" placeholder="接收消息回调 Token" />
        <label>EncodingAESKey</label>
        <input v-model="wc.encodingAesKey" placeholder="接收消息回调 EncodingAESKey" />
        <div class="sv-im-actions">
          <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveWeCom">保存配置</button>
          <span class="sv-muted">回调地址：<code>{{ wcHook }}</code></span>
        </div>
      </div>
    </section>

    <!-- Agent 参数 -->
    <section v-show="tab === 'ai'" class="sv-card">
      <div class="sv-card-h">
        <h3>Agent 参数</h3>
        <p>工具轮数上限是单次对话最多调用工具的次数（免费/付费分级参考项）</p>
      </div>
      <div class="sv-agent">
        <label>工具轮数上限（1-32）</label>
        <input v-model.number="maxRounds" type="number" min="1" max="32" style="width: 120px" />
        <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveMaxRounds">保存</button>
      </div>
    </section>

    <!-- 入库解读 -->
    <section v-show="tab === 'ai'" class="sv-card">
      <div class="sv-card-h">
        <h3>入库解读</h3>
        <p>文件入库后后台 AI 自动解读（摘要+标签+实体，进知识库/图谱）；按类型筛选可减少无谓 token 消耗</p>
      </div>
      <div class="sv-agent">
        <label>解读类型白名单（逗号分隔扩展名，如 .md,.docx,.pdf）</label>
        <input v-model="summarizeExts" placeholder="留空 = 全部文本类型都解读" style="width: 320px" />
        <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveSummarizeExts">保存</button>
        <p class="sv-muted" style="margin-top:6px">批量导入大批文件时建议先只开 .md 等高频类型；白名单变更即时生效，已解读文件不受影响</p>
      </div>
    </section>

    <!-- 应用中心（在线目录） -->
    <section v-show="tab === 'apps'" class="sv-card">
      <div class="sv-card-h">
        <h3>应用中心 · 在线目录</h3>
        <p>从应用市场在线目录一键安装插件与主题（来源 <code>{{ marketUrl }}</code> · 本壳 <code>{{ marketShell }}</code>）</p>
      </div>
      <div class="sv-im-actions" style="margin-bottom:10px">
        <button class="btn btn-sm btn-primary" :disabled="marketBusy" @click="loadMarket">刷新目录</button>
        <span v-if="marketErr" class="sv-muted" style="color:#b91c1c">{{ marketErr }}</span>
      </div>
      <!-- 许可证：付费应用安装门禁（community 免费 / pro 解锁 tier=paid） -->
      <div class="sv-license">
        <span class="sv-lic-badge" :class="{ pro: license?.edition === 'pro' }">{{ license?.edition === 'pro' ? 'PRO' : 'COMMUNITY' }}</span>
        <span v-if="license?.expires_at" class="sv-muted" style="font-size:12px">到期 {{ String(license.expires_at).slice(0, 10) }}</span>
        <span v-if="license?.reason" class="sv-muted" style="color:#b91c1c;font-size:12px">{{ license.reason }}</span>
        <span class="sv-muted" style="font-size:12px">输入签发的 license key 解锁付费应用安装；留空保持社区版</span>
        <input v-model="licenseKey" style="max-width:260px" placeholder="license key…" />
        <button class="btn btn-sm" :disabled="licBusy || !licenseKey" @click="activateLicense">激活</button>
        <button v-if="license?.key_set" class="btn btn-sm" :disabled="licBusy" @click="deactivateLicense">清除</button>
        <span v-if="licMsg" class="sv-muted" style="font-size:12px">{{ licMsg }}</span>
      </div>
      <div v-if="marketPlugins.length || marketThemes.length || marketThird.length" class="sv-tabs">
        <button class="sv-tab" :class="{ on: mtab === 'plugins' }" @click="mtab = 'plugins'">插件 ({{ marketPlugins.length }})</button>
        <button class="sv-tab" :class="{ on: mtab === 'themes' }" @click="mtab = 'themes'">主题 ({{ marketThemes.length }})</button>
        <button class="sv-tab" :class="{ on: mtab === 'third' }" @click="mtab = 'third'">社区 ({{ marketThird.length }})</button>
      </div>
      <div v-if="mtab === 'plugins'" class="sv-app-grid">
        <div v-for="p in marketPlugins" :key="p.id" class="sv-app-item">
          <div class="sv-app-t">
            <span class="sv-app-name">{{ p.name }}</span>
            <span v-if="!p.applicable" class="sv-badge sv-badge-muted">本壳不可用</span>
            <span v-if="p.tier === 'paid'" class="sv-badge sv-badge-paid">付费</span>
            <span v-if="p.installed" class="sv-badge sv-badge-ok">已安装</span>
          </div>
          <p class="sv-app-desc">{{ p.description }}</p>
          <div class="sv-app-foot">
            <span class="sv-muted" style="font-size:12px">v{{ p.version }} · {{ p.author || '—' }}</span>
            <button v-if="!p.installed && p.applicable" class="btn btn-sm" :disabled="marketBusy" @click="installMarket(p)">安装</button>
            <span v-else-if="!p.applicable" class="sv-muted">不可用</span>
            <span v-else class="sv-muted">已安装</span>
          </div>
        </div>
        <div v-if="marketPlugins.length === 0" class="sv-muted">暂无插件</div>
      </div>
      <div v-if="mtab === 'themes'" class="sv-app-grid">
        <div v-for="t in marketThemes" :key="t.id" class="sv-app-item">
          <img v-if="t.cover" :src="t.cover" class="sv-app-cover" alt="" loading="lazy" />
          <div class="sv-app-t">
            <span class="sv-app-name">{{ t.name }}</span>
            <span v-if="!t.applicable" class="sv-badge sv-badge-muted">本壳不可用</span>
            <span v-if="t.tier === 'paid'" class="sv-badge sv-badge-paid">付费</span>
            <span v-if="t.installed" class="sv-badge sv-badge-ok">已安装</span>
          </div>
          <p class="sv-app-desc">{{ t.description }}</p>
          <div class="sv-app-foot">
            <span class="sv-muted" style="font-size:12px">v{{ t.version }} · {{ t.author || '—' }}</span>
            <button v-if="!t.installed && t.applicable" class="btn btn-sm" :disabled="marketBusy" @click="installMarket(t)">安装</button>
            <span v-else-if="!t.applicable" class="sv-muted">不可用</span>
            <span v-else class="sv-muted">已安装</span>
          </div>
        </div>
        <div v-if="marketThemes.length === 0" class="sv-muted">暂无主题</div>
      </div>
      <div v-if="mtab === 'third'" class="sv-app-grid">
        <div v-for="t in marketThird" :key="t.id" class="sv-app-item">
          <img v-if="t.icon" :src="t.icon" class="sv-app-cover" alt="" loading="lazy" />
          <div class="sv-app-t">
            <span class="sv-app-name">{{ t.name }}</span>
            <span class="sv-badge sv-badge-muted">社区</span>
            <span v-if="t.tier === 'paid'" class="sv-badge sv-badge-paid">付费</span>
          </div>
          <p class="sv-app-desc">{{ t.description }}</p>
          <div class="sv-app-foot">
            <span class="sv-muted" style="font-size:12px">v{{ t.version }} · {{ t.developer || '—' }}</span>
            <button class="btn btn-sm" :disabled="marketBusy" @click="installThird(t)">安装</button>
          </div>
        </div>
        <div v-if="marketThird.length === 0" class="sv-muted">暂无社区应用（第三方开发者上架后在此出现）</div>
      </div>
    </section>

    <!-- 已装应用（安装 zip / 启停 / 配置） -->
    <section v-show="tab === 'apps'" class="sv-card">
      <div class="sv-card-h">
        <h3>已装应用</h3>
        <p>本地管理已安装应用：安装 zip（内含 <code>manifest.json</code>）、启用/禁用、卸载与配置。</p>
      </div>
      <div class="sv-app-install">
        <label class="btn btn-sm sv-up">
          <input type="file" accept=".zip,application/zip" @change="onInstallZip" />
          安装应用 zip
        </label>
        <span class="sv-muted" style="font-size:12px">协议见 docs/PLUGIN-MARKET.md</span>
      </div>
      <table class="plug-table" style="margin-top:12px">
        <thead><tr><th>应用</th><th>版本</th><th>挂载点</th><th>状态</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="p in plugins" :key="'app-' + p.id">
            <td><code>{{ p.id }}</code><div class="sv-muted" style="font-size:12px">{{ p.name }}</div></td>
            <td>{{ p.version }}</td>
            <td>{{ (p.mount_points || []).join(' / ') || '—' }}</td>
            <td><span class="sv-badge" :class="{ on: p.enabled }">{{ p.enabled ? '启用' : '禁用' }}</span></td>
            <td>
              <button class="btn btn-sm" @click="togglePlugin(p)">{{ p.enabled ? '禁用' : '启用' }}</button>
              <button class="btn btn-sm" style="margin-left:6px" @click="removePlugin(p)">卸载</button>
              <button class="btn btn-sm" style="margin-left:6px" @click="openConfig(p)">配置</button>
            </td>
          </tr>
        </tbody>
      </table>
      <!-- 插件动态设置（settings_schema → KV；应用中心安装的插件可在此配置，如 RSS 条目数） -->
      <div v-if="cfgId" class="sv-cfg">
        <div class="sv-cfg-h">
          <strong>配置：{{ cfgName }}</strong>
          <button class="btn btn-sm" @click="closeConfig">关闭</button>
        </div>
        <div v-for="f in cfgSchema" :key="f.key" class="sv-cfg-row">
          <label>{{ f.label || f.key }}<span class="sv-muted">（{{ f.key }}）</span></label>
          <input v-if="f.type === 'number'" type="number" v-model="cfgValues[f.key]" class="input" />
          <input v-else-if="f.type === 'boolean'" type="checkbox" v-model="cfgValues[f.key]" />
          <input v-else type="text" v-model="cfgValues[f.key]" class="input" />
        </div>
        <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveConfig">保存设置</button>
      </div>
    </section>

    <!-- 插件调试（协议 v1.1 可视化：manifest / 事件 / KV 抽样） -->
    <section v-show="tab === 'apps'" class="sv-card">
      <div class="sv-card-h">
        <h3>博客插件调试</h3>
        <p>已登记插件 manifest、可投递事件名、KV 数据抽样——插件开发者对照 <code>docs/博客插件协议.md</code> 排查渲染与数据</p>
      </div>
      <div class="sv-plug">
        <div v-if="plugins.length === 0" class="sv-muted">暂无插件登记（系统内置插件随博客空间初始化自动注册）</div>
        <table v-else class="plug-table">
          <thead><tr><th>id</th><th>版本</th><th>挂载点</th><th>权限</th><th>min_core</th><th>状态</th><th>KV 抽样</th></tr></thead>
          <tbody>
            <tr v-for="p in plugins" :key="p.id">
              <td><code>{{ p.id }}</code><div class="sv-muted" style="font-size:12px">{{ p.name }}</div></td>
              <td>{{ p.version }}</td>
              <td>{{ (p.mount_points || []).join(' / ') }}</td>
              <td>{{ (p.api_permissions || []).join(' / ') || '—' }}</td>
              <td>{{ p.min_core_version || '0' }}</td>
              <td><span class="sv-badge" :class="{ on: p.enabled }">{{ p.enabled ? '启用' : '禁用' }}</span></td>
              <td>
                <code v-if="kvSamples[p.id]" class="kv-sample" :title="kvSamples[p.id].join('\n')">
                  {{ kvSamples[p.id].length }} 键：{{ kvSamples[p.id].slice(0, 3).join(' · ') }}{{ kvSamples[p.id].length > 3 ? ' …' : '' }}
                </code>
                <span v-else class="sv-muted">空</span>
              </td>
            </tr>
          </tbody>
        </table>
        <div class="sv-events">
          <span class="sv-events-label">事件名（event:post 可投递对象）</span>
          <span v-for="ev in eventNames" :key="ev" class="ev-chip" @click="copyEvent(ev)" :title="'点击复制 ' + ev">{{ ev }}</span>
        </div>
      </div>
    </section>

    <!-- AI 用量（配额分层：日配额消耗 + 当日 Top 来源） -->
    <section v-show="tab === 'ai'" class="sv-card">
      <div class="sv-card-h">
        <h3>AI 用量</h3>
        <p>博客 AI 问答近 7 日消耗与当日来源 Top（配额键：<code>blog.ai_ask_*</code>，可在设置接口调整）</p>
      </div>
      <div class="sv-plug">
        <div v-if="aiUsageDaily.length === 0" class="sv-muted">近 7 日暂无 AI 问答用量</div>
        <table v-else class="plug-table">
          <thead><tr><th>日期</th><th>问答次数</th></tr></thead>
          <tbody>
            <tr v-for="d in aiUsageDaily" :key="d.day">
              <td>{{ d.day }}</td>
              <td>{{ d.hits }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="aiUsageTop.length" class="sv-events">
          <span class="sv-events-label">今日来源 Top（已脱敏）</span>
          <span v-for="t in aiUsageTop" :key="t.ident" class="ev-chip">{{ t.ident }} · {{ t.hits }} 次</span>
        </div>
        <p class="sv-muted" style="font-size:12px">配额默认：游客 3 次/日 · 登录用户 10 次/日 · 管理员不限 · 全站日预算 200 次（超限当日降级）</p>
      </div>
    </section>

    <!-- 数据（导入导出，独立视图组件整体嵌入） -->
    <section v-show="tab === 'data'" class="sv-data-tab">
      <ImpexView />
    </section>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import ImpexView from '@/views/ImpexView.vue'

const toast = useToastStore()
const saving = ref(false)

// ---- 顶部分组标签（站点/AI/消息/应用/数据；?tab= 深链直达）----
const route = useRoute()
const router = useRouter()
const NAV_TAB_KEYS = ['site', 'ai', 'im', 'apps', 'data']
const tab = ref(NAV_TAB_KEYS.includes(route.query.tab) ? route.query.tab : 'site')
function switchTab(k) {
  tab.value = k
  router.replace({ query: { ...route.query, tab: k } })
}
const navTabs = computed(() => [
  { key: 'site', label: '站点' },
  { key: 'ai', label: 'AI' },
  { key: 'im', label: '消息' },
  { key: 'apps', label: '应用', count: plugins.value.length + marketPlugins.value.length + marketThemes.value.length || 0 },
  { key: 'data', label: '数据' }
])

// ---- 站点（多用户/开放注册）----
const multiUser = ref(true)
const registrationOpen = ref(true)

async function loadSite() {
  try {
    const d = await api.authConfig()
    if (d) multiUser.value = d.multi_user !== false
  } catch (_) { /* 未登录/旧后端：保持默认展示 */ }
  try {
    const s = await api.settings()
    const e = s && s['site.registration_open']
    if (e) registrationOpen.value = String(e.value) !== 'false'
  } catch (_) { /* 非 admin 无读取权限，保持默认展示 */ }
}

async function toggleRegistration() {
  const next = !registrationOpen.value
  saving.value = true
  try {
    await api.updateSetting('site.registration_open', next ? 'true' : 'false')
    registrationOpen.value = next
    toast.success(next ? '已开启开放注册' : '已关闭开放注册')
  } catch (e) {
    toast.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

// ---- AI 模型 ----
const caps = ref([])
const customOpen = ref('')
const customForm = ref({ endpoint: '', model: '', apiKey: '' })

const capLabels = {
  llm: '对话', embedding: '向量', asr: '语音识别', tts: '语音合成',
  image: '图像', rerank: '重排序', ocr: 'OCR', code: '代码'
}

async function loadProviders() {
  const d = await api.aiProviders()
  caps.value = (d.capabilities || []).map((c) => ({ ...c, label: capLabels[c.cap] || c.cap }))
}

function openCustom(cap) {
  if (customOpen.value === cap) {
    customOpen.value = ''
    return
  }
  const c = caps.value.find((x) => x.cap === cap)
  customOpen.value = cap
  customForm.value = {
    endpoint: c?.custom?.endpoint || '',
    model: c?.custom?.model || '',
    apiKey: ''
  }
}

async function onSwitch(cap, e) {
  const provider = e.target.value
  if (!provider) return
  try {
    await api.aiProviderSwitch(cap, provider)
    toast.success('已切换 ' + provider)
    await loadProviders()
  } catch (err) {
    toast.error(err.message)
  }
}

async function saveCustom(cap) {
  saving.value = true
  try {
    await api.aiCustomSave(cap, customForm.value.endpoint, customForm.value.model, customForm.value.apiKey)
    toast.success('自备模型已保存')
    customOpen.value = ''
    await loadProviders()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

async function clearCustom(cap) {
  try {
    await api.aiCustomClear(cap)
    toast.success('已清除自备模型')
    await loadProviders()
  } catch (err) {
    toast.error(err.message)
  }
}

// ---- IM 对接 ----
const tg = ref({ botToken: '', allowedChats: '' })
const wc = ref({ webhookUrl: '', corpId: '', agentId: '', secret: '', token: '', encodingAesKey: '' })
const imTgOn = ref(false)
const imWcOn = ref(false)
const tgHook = computed(() => `${location.origin}/api/v1/im/webhook/telegram`)
const wcHook = computed(() => `${location.origin}/api/v1/im/webhook/wecom`)

async function loadIM() {
  try {
    const d = await api.imStatus()
    imTgOn.value = !!(d.telegram && d.telegram.enabled)
    imWcOn.value = !!(d.wecom && d.wecom.enabled)
  } catch (_) {
    /* 后端未更新时忽略 */
  }
}

async function saveIM() {
  saving.value = true
  try {
    await api.imTelegramConfig({ botToken: tg.value.botToken, allowedChats: tg.value.allowedChats })
    toast.success('Telegram 配置已保存')
    tg.value.botToken = ''
    await loadIM()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

async function saveWeCom() {
  saving.value = true
  try {
    await api.imWeComConfig({
      webhookUrl: wc.value.webhookUrl,
      corpId: wc.value.corpId,
      agentId: wc.value.agentId,
      secret: wc.value.secret,
      token: wc.value.token,
      encodingAesKey: wc.value.encodingAesKey
    })
    toast.success('企业微信配置已保存')
    wc.value.secret = ''
    await loadIM()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

// ---- Agent 参数 ----
const maxRounds = ref(6)

async function loadAgent() {
  try {
    const s = await api.settings()
    const v = s['ai.agent.max_rounds']
    if (v && v.value) maxRounds.value = Number(v.value)
  } catch (_) { /* 忽略 */ }
}

async function saveMaxRounds() {
  const n = Number(maxRounds.value)
  if (!n || n < 1 || n > 32) {
    toast.error('轮数需在 1-32 之间')
    return
  }
  saving.value = true
  try {
    await api.updateSetting('ai.agent.max_rounds', String(n))
    toast.success('已保存，即时生效')
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

// ---- 入库解读 ----
const summarizeExts = ref('')

async function loadSummarize() {
  try {
    const st = await api.settings()
    const v = st['ai.summarize.exts']
    if (v) summarizeExts.value = v.value || ''
  } catch (_) { /* 忽略 */ }
}

async function saveSummarizeExts() {
  saving.value = true
  try {
    await api.updateSetting('ai.summarize.exts', summarizeExts.value.trim())
    toast.success('解读类型已保存，即时生效')
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

// ---- 插件调试（协议 v1.1）----
const plugins = ref([])
const kvSamples = ref({})

// 事件名（对应审计动作；event:post 权限可投递的对象清单）
const eventNames = [
  'auth.login', 'auth.logout', 'file.create', 'file.update', 'file.delete',
  'file.move', 'share.create', 'share.delete', 'blog.post', 'blog.update',
  'comment.create', 'collection.create', 'tag.create', 'collect.run',
  'export.created', 'plugin.register', 'im.telegram.message', 'settings.update',
]

function copyEvent(ev) {
  navigator.clipboard?.writeText(ev).catch(() => {})
  toast.success('已复制 ' + ev)
}

async function loadPlugins() {
  try {
    const d = await api.publicPlugins()
    plugins.value = d.items || []
    for (const p of plugins.value) {
      try {
        const kv = await api.pluginKVList(p.id)
        kvSamples.value[p.id] = (kv.items || []).map((x) => x.key).slice(0, 8)
      } catch (_) {
        kvSamples.value[p.id] = []
      }
    }
  } catch (_) { /* 后端未更新时忽略 */ }
}

async function onInstallZip(ev) {
  const f = ev.target.files?.[0]
  ev.target.value = ''
  if (!f) return
  try {
    const d = await api.installAppZip(f)
    toast.success(`已安装 ${d.id}`)
    await loadPlugins()
  } catch (e) {
    toast.error(e.message || '安装失败')
  }
}

async function togglePlugin(p) {
  try {
    await api.pluginToggle(p.id, !p.enabled)
    p.enabled = !p.enabled
    toast.success(p.enabled ? '已启用' : '已禁用')
  } catch (e) {
    toast.error(e.message)
  }
}

async function removePlugin(p) {
  if (!confirm(`卸载插件 ${p.id}？`)) return
  try {
    await api.pluginDelete(p.id)
    plugins.value = plugins.value.filter((x) => x.id !== p.id)
    toast.success('已卸载')
  } catch (e) {
    toast.error(e.message)
  }
}

// ---- 应用中心（在线目录）----
const mtab = ref('plugins')
const marketPlugins = ref([])
const marketThemes = ref([])
const marketThird = ref([])
const marketUrl = ref('')
const marketShell = ref('')
const marketErr = ref('')
const marketBusy = ref(false)

const license = ref(null)
const licenseKey = ref('')
const licBusy = ref(false)
const licMsg = ref('')

async function loadLicense() {
  try {
    license.value = await api.requestJSON('/license')
  } catch (_) { license.value = null }
}

async function activateLicense() {
  licBusy.value = true
  licMsg.value = ''
  try {
    license.value = await api.requestJSON('/license', { method: 'POST', body: JSON.stringify({ key: licenseKey.value }) })
    licMsg.value = license.value?.edition === 'pro' ? '已激活 PRO' : '已保存，但未达到 PRO 条件'
    licenseKey.value = ''
  } catch (e) {
    licMsg.value = '激活失败：' + (e?.message || e)
  } finally {
    licBusy.value = false
  }
}

async function deactivateLicense() {
  licBusy.value = true
  licMsg.value = ''
  try {
    license.value = await api.requestJSON('/license', { method: 'DELETE' })
    licMsg.value = '已清除许可证'
  } catch (e) {
    licMsg.value = '清除失败：' + (e?.message || e)
  } finally {
    licBusy.value = false
  }
}

async function loadMarket() {
  marketBusy.value = true
  marketErr.value = ''
  try {
    const m = await api.appsMarketList()
    marketPlugins.value = m.plugins || []
    marketThemes.value = m.themes || []
    marketThird.value = m.third_party || []
    marketUrl.value = m.index_url || ''
    marketShell.value = m.shell || 'aiklog'
  } catch (e) {
    marketErr.value = e?.message || String(e)
    marketPlugins.value = []
    marketThemes.value = []
    marketThird.value = []
  } finally {
    marketBusy.value = false
  }
}

async function installMarket(item) {
  marketBusy.value = true
  try {
    const r = await api.appsMarketInstall({ id: item.id })
    if (r?.ok) {
      toast.success('已安装：' + (item.name || item.id))
      await loadMarket()
      await loadPlugins()
    } else {
      toast.error(r?.error?.message || '安装失败')
    }
  } catch (e) {
    toast.error(e?.message || '安装失败')
  } finally {
    marketBusy.value = false
  }
}

// 第三方社区应用：按 listing_id 安装
async function installThird(item) {
  marketBusy.value = true
  try {
    const r = await api.appsMarketInstall({ listing_id: item.id })
    if (r?.ok) {
      toast.success('已安装：' + (item.name || item.id))
      await loadMarket()
      await loadPlugins()
    } else {
      toast.error(r?.error?.message || '安装失败')
    }
  } catch (e) {
    toast.error(e?.message || '安装失败')
  } finally {
    marketBusy.value = false
  }
}

// ---- 插件动态设置（settings_schema → KV）----
const cfgId = ref('')
const cfgName = ref('')
const cfgSchema = ref([])
const cfgValues = ref({})

async function openConfig(p) {
  cfgId.value = p.id
  cfgName.value = p.name || p.id
  try {
    const r = await api.pluginSettingsSchema(p.id)
    cfgSchema.value = r.settings_schema || []
  } catch (_) {
    cfgSchema.value = []
  }
  cfgValues.value = {}
  try {
    const kv = await api.pluginKVList(p.id)
    for (const k of (kv.items || [])) cfgValues.value[k.key] = k.value
  } catch (_) {}
}
function closeConfig() {
  cfgId.value = ''
  cfgSchema.value = []
  cfgValues.value = {}
}
async function saveConfig() {
  saving.value = true
  try {
    for (const f of cfgSchema.value) {
      const v = cfgValues.value[f.key]
      await api.pluginKVSet(cfgId.value, f.key, v == null ? '' : String(v))
    }
    toast.success('已保存设置')
    closeConfig()
  } catch (e) {
    toast.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

// ---- AI 用量（近 7 日 + 当日 Top）----
const aiUsageDaily = ref([])
const aiUsageTop = ref([])

async function loadAiUsage() {
  try {
    const d = await api.aiUsage(7)
    aiUsageDaily.value = d?.daily || []
    aiUsageTop.value = d?.top || []
  } catch (_) { /* 非 admin / 旧后端：静默隐藏 */ }
}

onMounted(() => {
  loadSite()
  loadProviders()
  loadIM()
  loadAgent()
  loadSummarize()
  loadPlugins()
  loadMarket()
  loadLicense()
  loadAiUsage()
})
</script>

<style scoped>
.settings-view {
  padding: 20px 24px;
  max-width: 900px;
}
/* 顶部分组标签栏 */
.sv-nav-tabs {
  display: flex;
  gap: 6px;
  margin: 4px 0 16px;
  border-bottom: 1px solid var(--border);
  padding-bottom: 10px;
  flex-wrap: wrap;
}
.sv-tab-btn {
  border: 1px solid transparent;
  background: none;
  cursor: pointer;
  font-size: 13px;
  color: var(--muted, #666);
  padding: 6px 14px;
  border-radius: 999px;
  transition: all 0.15s;
}
.sv-tab-btn:hover {
  color: var(--text, #222);
  background: var(--hover, rgba(127, 127, 127, 0.08));
}
.sv-tab-btn.on {
  color: var(--primary, #4f7cff);
  background: var(--primary-soft, rgba(79, 124, 255, 0.1));
  border-color: var(--primary-soft, rgba(79, 124, 255, 0.2));
  font-weight: 600;
}
.sv-tab-count {
  margin-left: 5px;
  font-size: 11px;
  opacity: 0.75;
}
.sv-data-tab {
  margin-top: 4px;
}
/* 插件调试 */
.sv-plug {
  border-top: 1px solid var(--border);
  padding-top: 12px;
}
.plug-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12.5px;
}
.plug-table th {
  text-align: left;
  color: var(--text-3);
  font-weight: 500;
  padding: 6px 8px;
  border-bottom: 1px solid var(--border);
}
.plug-table td {
  padding: 8px;
  border-bottom: 1px solid var(--border);
  vertical-align: top;
  color: var(--text);
}
.kv-sample {
  font-size: 11.5px;
  color: var(--text-2);
  background: var(--bg);
  padding: 2px 6px;
  border-radius: 4px;
  display: inline-block;
  max-width: 260px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sv-events {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.sv-events-label {
  font-size: 12px;
  color: var(--text-3);
  margin-right: 4px;
}
.ev-chip {
  font-size: 11.5px;
  font-family: var(--mono, monospace);
  background: var(--bg);
  border: 1px solid var(--border);
  color: var(--text-2);
  padding: 3px 8px;
  border-radius: 10px;
  cursor: pointer;
  user-select: none;
}
.ev-chip:hover {
  border-color: var(--accent, #3b82f6);
  color: var(--accent, #3b82f6);
}
/* 应用中心（在线目录） */
.sv-tabs {
  display: flex;
  gap: 8px;
  margin: 4px 0 12px;
}
.sv-tab {
  padding: 6px 14px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: transparent;
  cursor: pointer;
  font-size: 13px;
  color: var(--text);
}
.sv-tab.on {
  background: var(--accent, #2563eb);
  color: #fff;
  border-color: transparent;
}
.sv-app-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 12px;
}
.sv-app-item {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 12px 14px;
  background: var(--card-bg, #fff);
}
.sv-app-cover {
  width: 100%;
  aspect-ratio: 8 / 5;
  object-fit: cover;
  border-radius: 6px;
  margin-bottom: 10px;
  border: 1px solid var(--border);
  background: var(--bg-2, #f2f4f7);
}
.sv-license {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  border: 1px dashed var(--border);
  border-radius: 8px;
  padding: 8px 12px;
  margin-bottom: 12px;
}
.sv-license input {
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--bg-1, #fff);
  color: var(--text-1, #1f2329);
  font-size: 13px;
  outline: none;
}
.sv-lic-badge {
  font-size: 11px;
  font-weight: 600;
  border-radius: 999px;
  padding: 2px 10px;
  background: var(--bg-2, #f2f4f7);
  color: var(--text-2, #5a6472);
}
.sv-lic-badge.pro {
  background: #e7f5ec;
  color: #177245;
}
.sv-app-t {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.sv-app-name {
  font-weight: 600;
  font-size: 14px;
  color: var(--text);
}
.sv-app-desc {
  color: var(--text-2, #666);
  font-size: 13px;
  margin: 8px 0 10px;
  line-height: 1.5;
}
.sv-app-foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.sv-badge {
  font-size: 11px;
  padding: 2px 7px;
  border-radius: 4px;
}
.sv-badge-paid {
  background: #fef3c7;
  color: #92400e;
}
.sv-badge-ok {
  background: #e7f5ec;
  color: #177245;
}
.sv-badge-muted {
  background: #eee;
  color: #777;
}
/* 插件动态设置面板 */
.sv-cfg {
  margin-top: 14px;
  border: 1px dashed var(--border);
  border-radius: 8px;
  padding: 12px 14px;
  background: var(--bg);
}
.sv-cfg-h {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}
.sv-cfg-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 10px;
}
.sv-cfg-row label {
  font-size: 12.5px;
  color: var(--text-2);
}
.sv-cfg-row .input {
  width: 260px;
  max-width: 100%;
}

.sv-head h2 {
  font-size: 17px;
  font-weight: 600;
  color: var(--text);
  margin-bottom: 4px;
}
.sv-sub {
  font-size: 12.5px;
  color: var(--text-3);
  margin-bottom: 18px;
}
.sv-card {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  padding: 16px;
  margin-bottom: 16px;
}
.sv-card-h h3 {
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
  margin-bottom: 4px;
}
.sv-card-h p {
  font-size: 12px;
  color: var(--text-3);
  margin-bottom: 12px;
}
.sv-cap {
  border-top: 1px solid var(--border);
  padding: 12px 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
}
.sv-cap:first-of-type {
  border-top: none;
}
.sv-cap-info {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 200px;
}
.sv-cap-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--text);
  width: 64px;
}
.sv-badge {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 10px;
  background: var(--surface-2);
  color: var(--text-3);
}
.sv-badge.on {
  background: var(--primary-soft);
  color: var(--primary);
}
.sv-cap-model {
  font-size: 12px;
  color: var(--text-2);
}
.sv-custom-tag {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 10px;
  background: rgba(250, 173, 20, 0.14);
  color: #b45309;
}
.sv-cap-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sv-cap-actions select {
  height: 28px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
  padding: 0 8px;
  font-size: 12px;
}
.sv-custom {
  flex-basis: 100%;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
  padding: 12px;
  background: var(--surface-2);
  border-radius: 8px;
}
.sv-custom input,
.sv-im input,
.sv-agent input {
  height: 30px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
  padding: 0 10px;
  font-size: 12.5px;
  flex: 1;
  min-width: 180px;
}
.sv-custom-actions {
  display: flex;
  gap: 8px;
}
.sv-im label,
.sv-agent label {
  display: block;
  font-size: 12px;
  color: var(--text-2);
  margin: 10px 0 4px;
}
.sv-im-actions,
.sv-agent {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 12px;
}
.sv-im-state-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.sv-im + .sv-im {
  border-top: 1px solid var(--border);
  margin-top: 16px;
  padding-top: 16px;
}
.sv-im-block-h {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.sv-im-plt {
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}
.sv-muted {
  font-size: 12px;
  color: var(--text-3);
}
.sv-muted code {
  background: var(--surface-2);
  padding: 1px 6px;
  border-radius: 6px;
  font-size: 11px;
}
.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 14px;
  border-radius: 8px;
  font-size: 13px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  color: var(--text-2);
  white-space: nowrap;
  cursor: pointer;
}
.btn:hover:not(:disabled) {
  color: var(--text);
  border-color: var(--text-3);
}
.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.btn-primary {
  background: var(--primary);
  border-color: var(--primary);
  color: #fff;
}
.btn-primary:hover:not(:disabled) {
  color: #fff;
  opacity: 0.92;
}
.btn-sm {
  height: 28px;
  padding: 0 10px;
  font-size: 12px;
}
</style>
