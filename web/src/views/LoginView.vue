<template>
  <div class="login-page">
    <div class="login-card">
      <div class="login-brand">
        <div class="brand-mark">K</div>
        <h1 class="brand-title">爱库录</h1>
        <p class="brand-sub">AiKlog · AI 知识库博客</p>
      </div>

      <form class="login-form" @submit.prevent="submit">
        <div v-if="mode === 'register'" class="field">
          <label>昵称（可选）</label>
          <input
            v-model="displayName"
            type="text"
            placeholder="怎么称呼你"
            spellcheck="false"
          />
        </div>
        <div class="field">
          <label>用户名</label>
          <input
            v-model="username"
            type="text"
            autocomplete="username"
            :placeholder="mode === 'register' ? '3-32 位，字母/数字/中文' : 'admin'"
            spellcheck="false"
            @keyup.enter="submit"
          />
        </div>
        <div class="field">
          <label>密码</label>
          <input
            v-model="password"
            type="password"
            :autocomplete="mode === 'register' ? 'new-password' : 'current-password'"
            :placeholder="mode === 'register' ? '至少 8 位' : '请输入密码'"
            @keyup.enter="submit"
          />
        </div>
        <div v-if="mode === 'register' && emailEnabled" class="field">
          <label>邮箱{{ emailRequired ? '' : '（可选，需验证码）' }}</label>
          <input
            v-model="email"
            type="email"
            autocomplete="email"
            placeholder="you@example.com"
            spellcheck="false"
          />
          <div class="login-code-row">
            <input
              v-model="code"
              class="login-code"
              type="text"
              inputmode="numeric"
              maxlength="6"
              placeholder="6 位验证码"
            />
            <button
              type="button"
              class="login-code-btn"
              :disabled="codeBusy || !email.trim()"
              @click="sendCode"
            >
              {{ codeBusy ? '发送中…' : '获取验证码' }}
            </button>
          </div>
        </div>
        <p v-if="error" class="login-error">{{ error }}</p>
        <button class="login-btn" type="submit" :disabled="busy">
          {{ busy ? '处理中…' : (mode === 'register' ? '注 册' : '登 录') }}
        </button>
      </form>

      <p v-if="canRegister" class="login-switch" @click="toggleMode">
        {{ mode === 'register' ? '已有账号？去登录' : '没有账号？注册一个' }}
      </p>
      <p v-if="mode === 'login'" class="login-hint">首次使用请在服务端启动日志中查看初始管理员密码</p>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { authLogin, authRegister, authMe, authConfig, emailCode, setAuthFlag } from '@/api'

const router = useRouter()
const route = useRoute()
const mode = ref('login')
const canRegister = ref(false)      // 多用户且开放注册时才展示注册入口
const emailEnabled = ref(false)     // 发信邮箱已配置（可走邮箱验证码）
const emailRequired = ref(false)    // 站点要求注册必填邮箱
const username = ref('admin')
const password = ref('')
const displayName = ref('')
const email = ref('')
const code = ref('')
const codeBusy = ref(false)
const error = ref('')
const busy = ref(false)

onMounted(() => {
  // 已登录则直接进入
  authMe().then(() => goNext()).catch(() => {})
  // 能力探测：多用户关闭或注册开关关闭时隐藏注册入口
  authConfig().then((d) => {
    if (!d) return
    const multi = d.multi_user !== false
    const open = d.registration_open !== false
    canRegister.value = multi && open
    emailEnabled.value = !!d.email_enabled
    emailRequired.value = !!d.email_required
    // 公开页「注册」入口带 ?mode=register 进来 → 直接落在注册表单（预填的 admin 占位符清掉）
    if (canRegister.value && route.query.mode === 'register') {
      mode.value = 'register'
      if (username.value === 'admin') username.value = ''
    } else if (!canRegister.value) {
      mode.value = 'login'
    }
  }).catch(() => {})
})

function goNext() {
  const next = route.query.next || '/files'
  router.replace(typeof next === 'string' ? next : '/files')
}

function toggleMode() {
  mode.value = mode.value === 'register' ? 'login' : 'register'
  error.value = ''
  // 清掉 ?mode=register，避免刷新时又被拉回注册态
  if (route.query.mode) {
    const q = { ...route.query }
    delete q.mode
    router.replace({ path: '/desk', query: q })
  }
}

async function sendCode() {
  if (codeBusy.value || !email.value.trim()) return
  codeBusy.value = true
  error.value = ''
  try {
    const d = await emailCode(email.value.trim(), 'register')
    if (d && d.debug_code) {
      code.value = d.debug_code
      error.value = '调试模式：已自动填入验证码 ' + d.debug_code
    } else {
      error.value = '验证码已发送到 ' + email.value.trim() + '，请查收'
    }
  } catch (e) {
    error.value = e.message || '验证码发送失败'
  } finally {
    codeBusy.value = false
  }
}

async function submit() {
  if (busy.value) return
  if (!username.value.trim() || !password.value) {
    error.value = '请输入用户名和密码'
    return
  }
  busy.value = true
  error.value = ''
  try {
    if (mode.value === 'register') {
      await authRegister(username.value.trim(), password.value, displayName.value.trim(), email.value.trim(), code.value.trim())
    } else {
      await authLogin(username.value.trim(), password.value)
    }
    // 双轨会话：新会话走 HttpOnly cookie（自动携带，防 XSS 窃取），localStorage 不再存 token；
    // 登录标记（sessionStorage 布尔）供路由守卫同步判定；d.token 保留给 API 客户端/第三方脚本使用
    setAuthFlag()
    goNext()
  } catch (e) {
    error.value = e.message || (mode.value === 'register' ? '注册失败' : '登录失败')
  } finally {
    busy.value = false
  }
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg);
  padding: var(--sp-5);
}
.login-card {
  width: 100%;
  max-width: 380px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: var(--sp-6) var(--sp-5);
  box-shadow: 0 12px 40px rgba(22, 24, 43, 0.08);
}
.login-brand {
  text-align: center;
  margin-bottom: var(--sp-6);
}
.brand-mark {
  width: 52px;
  height: 52px;
  margin: 0 auto var(--sp-3);
  border-radius: 14px;
  background: linear-gradient(135deg, var(--primary), var(--accent));
  color: #fff;
  font-size: 26px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}
.brand-title {
  font-size: var(--fs-xlarge);
  font-weight: 700;
  color: var(--text);
  margin: 0;
}
.brand-sub {
  font-size: var(--fs-small);
  color: var(--text-3);
  margin: var(--sp-1) 0 0;
}
.field {
  margin-bottom: var(--sp-4);
}
.field label {
  display: block;
  font-size: var(--fs-small);
  color: var(--text-2);
  margin-bottom: var(--sp-2);
}
.field input {
  width: 100%;
  box-sizing: border-box;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  font-size: var(--fs-base);
  color: var(--text);
  background: var(--surface-2);
  outline: none;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.field input:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px var(--primary-soft);
}
.login-code-row {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
.login-code {
  flex: 1;
}
.login-code-btn {
  border: 1px solid var(--border, #e3e6eb);
  background: var(--surface, #fff);
  border-radius: 6px;
  padding: 6px 12px;
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
  color: var(--text-2, #4b5563);
}
.login-code-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.login-error {
  color: var(--danger);
  font-size: var(--fs-small);
  margin: 0 0 var(--sp-3);
}
.login-btn {
  width: 100%;
  padding: 10px 0;
  border: none;
  border-radius: var(--radius-sm);
  background: var(--primary);
  color: #fff;
  font-size: var(--fs-medium);
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.15s;
}
.login-btn:hover {
  opacity: 0.92;
}
.login-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.login-switch {
  text-align: center;
  font-size: var(--fs-small);
  color: var(--primary);
  margin: var(--sp-3) 0 0;
  cursor: pointer;
}
.login-switch:hover {
  text-decoration: underline;
}
.login-hint {
  text-align: center;
  font-size: var(--fs-micro);
  color: var(--text-3);
  margin: var(--sp-4) 0 0;
}
</style>
