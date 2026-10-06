// AiKlog 框架级 i18n —— B22 自上游移植（2026-09-26）
// 行为对齐上游：localStorage 记忆 -> 浏览器语言探测 -> zh-CN 兜底；
// 未翻译键回退中文原文；切换时同步 <html lang>。
import { ref, watch } from 'vue'
import { zhCN, enUS } from './messages'

export const LOCALES = [
  { value: 'zh-CN', labelKey: 'settings.langZh' },
  { value: 'en-US', labelKey: 'settings.langEn' },
]

const LANG_KEY = 'aiklog.lang'
const messages = { 'zh-CN': zhCN, 'en-US': enUS }

export function detectLocale() {
  try {
    const saved = localStorage.getItem(LANG_KEY)
    if (saved && messages[saved]) return saved
  } catch {}
  const nav = (navigator.language || 'zh-CN').toLowerCase()
  return nav.startsWith('zh') ? 'zh-CN' : 'en-US'
}

export const locale = ref(detectLocale())

export function setLocale(next) {
  if (!messages[next]) next = 'zh-CN'
  locale.value = next
  try { localStorage.setItem(LANG_KEY, next) } catch {}
  try { document.documentElement.lang = next } catch {}
}

watch(locale, (v) => {
  try { document.documentElement.lang = v } catch {}
}, { immediate: true })

// 取词：当前语言 -> 中文 -> 键原文；支持 {name} 占位符插值
export function t(key, params) {
  const dict = messages[locale.value]
  let s = (dict && dict[key]) || zhCN[key] || key
  if (params) {
    for (const k of Object.keys(params)) {
      s = String(s).split('{' + k + '}').join(String(params[k]))
    }
  }
  return s
}

export default {
  install(app) {
    app.config.globalProperties.$t = t
    app.config.globalProperties.$locale = locale
  },
}
