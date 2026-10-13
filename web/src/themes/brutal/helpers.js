/**
 * Brutal（新粗野风）主题公共派生层
 * 严格按《AiKlog 博客主题开发规范（第三方完整交付版）》
 * §2.2 posts 字段 / §2.3 标题·分类·日期派生 / §7.1 AI 问答 实现。
 * 只使用契约字段 token / path / preview / file.{name,slug,updated_at,size}，禁止臆造。
 */
import { computed, inject } from 'vue'

/** 兼容 ref 与普通对象两种包装（不同宿主版本） */
export function useBlog(ctxRef) {
  if (ctxRef) return ctxRef
  const raw = inject('themeContext', null)
  return computed(() => raw?.value || raw || { posts: [], tags: [], loading: true, error: '' })
}

/** 标题：去扩展名（.md / .markdown）→ 取末段 */
export function postTitle(p) {
  const n = p?.file?.name || p?.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || '无标题'
}

/** 分类：path 首段；无 '/' 返回空串（展示层再决定是否写「未分类」） */
export function postCat(p) {
  const path = p?.path || ''
  return path.includes('/') ? path.split('/')[0] : ''
}

/** 分类展示名（补「未分类」兜底） */
export function catName(p) {
  return postCat(p) || '未分类'
}

/** 日期：updated_at（毫秒，兼容秒级/字符串） */
export function postDate(p, style = 'ymd') {
  const v = p?.file?.updated_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  const z = (n) => String(n).padStart(2, '0')
  if (style === 'short') return `${z(d.getMonth() + 1)}-${z(d.getDate())}`
  if (style === 'month') return `${d.getFullYear()}-${z(d.getMonth() + 1)}`
  if (style === 'year') return String(d.getFullYear())
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}`
}

/** 摘要：清 Markdown 记号后截断 */
export function postSummary(p, len = 88) {
  return (p?.preview || '')
    .replace(/^#+\s.*$/gm, '')
    .replace(/!?\[.*?\]\(.*?\)/g, '')
    .replace(/[#*`>_~]/g, '')
    .trim()
    .slice(0, len)
}

/** 附件大小（真实字段，非估算） */
export function postSize(p) {
  const s = p?.file?.size || 0
  if (!s) return ''
  return s > 1024 * 1024 ? (s / 1024 / 1024).toFixed(1) + ' MB' : Math.max(1, Math.round(s / 1024)) + ' KB'
}

/** 距今相对时间 */
export function daysAgo(p) {
  const v = p?.file?.updated_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const days = Math.floor((Date.now() - ms) / 86400000)
  if (days <= 0) return '今天'
  if (days === 1) return '昨天'
  if (days < 30) return days + ' 天前'
  return ''
}

/** 链接：一律用 ctx.postUrl（SSR 地址），不自行拼 #/p/ */
export function postHref(ctx, p) {
  if (ctx?.postUrl) return ctx.postUrl(p)
  return '#'
}

/** 序号：01 / 02 …（列表序号，非数据字段） */
export function seqNo(i) {
  return String(i + 1).padStart(2, '0')
}

/** 封面图案轮换（纯 CSS 生成，无封面图字段可用） */
const PATTERNS = ['a', 'b', 'c', 'd', 'e', 'f']
export function patClass(i) {
  return 'bt-pat-' + PATTERNS[i % PATTERNS.length]
}

/** 分类 → 稳定配色（同一分类永远同一颜色，便于读者建立视觉记忆） */
export function catColor(name) {
  let h = 0
  for (const ch of String(name || '文')) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return 'c' + (h % 5 + 1)
}

/** 分类计数（分类 → 篇数），按篇数倒序 */
export function catCounts(posts) {
  const map = new Map()
  for (const p of posts || []) {
    const name = catName(p)
    map.set(name, (map.get(name) || 0) + 1)
  }
  return [...map.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, count]) => ({ name, count }))
}

/** 年份归档分组（updated_at 派生） */
export function yearGroups(posts) {
  const map = new Map()
  for (const p of posts || []) {
    const y = postDate(p, 'year') || '未知'
    if (!map.has(y)) map.set(y, [])
    map.get(y).push(p)
  }
  return [...map.entries()]
    .sort((a, b) => String(b[0]).localeCompare(String(a[0])))
    .map(([year, list]) => ({ year, list }))
}

/**
 * 内页参数读取：优先 ctx.page（系统侧注入），
 * 缺失时回退解析 SPA hash，保证当前版本可预览。
 */
export function readPageParam(ctx, keys, patterns = []) {
  const page = ctx?.page || {}
  for (const k of keys) {
    const v = page[k]
    if (v) return String(v)
  }
  const hash = typeof window !== 'undefined' ? window.location.hash || '' : ''
  for (const re of patterns) {
    const m = hash.match(re)
    if (m && m[1]) return decodeURIComponent(m[1])
  }
  return ''
}

/** 页头/页脚共用链接 */
export const LIST_HREF = '#/blog?view=public'
export const RSS_HREF = '/api/v1/blog/feed.xml'

/** AI 问答：POST /api/v1/public/blog/ask，读 reply，处理 429（规范 §7.1） */
export async function askBlogAI(question, { onLoading } = {}) {
  const q = String(question || '').trim()
  if (!q) return ''
  onLoading?.(true)
  try {
    const res = await fetch('/api/v1/public/blog/ask', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question: q }),
    })
    if (res.status === 429) return '提问太频繁（每 5 分钟限 8 次），请稍后再试'
    const data = await res.json().catch(() => ({}))
    return data.reply || '暂无回答'
  } catch {
    return '请求失败，请稍后重试'
  } finally {
    onLoading?.(false)
  }
}
