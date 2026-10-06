/**
 * Verdant（青野）主题公共派生层
 * 严格按《AiKlog 博客主题开发规范（第三方完整交付版）》v1.2
 * §2.2 posts 字段 / §2.3 标题·分类·日期派生 / §2.4 排序实现。
 * 只使用契约字段 token / path / preview / file.{name,slug,updated_at,size}，禁止臆造。
 */import { t } from '@/i18n'

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
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('无标题')
}

/** 分类：path 首段；无 '/' 返回空串（展示层再决定是否写「未分类」） */
export function postCat(p) {
  const path = p?.path || ''
  return path.includes('/') ? path.split('/')[0] : ''
}

/** 分类展示名（补「未分类」兜底） */
export function catName(p) {
  return postCat(p) || t('未分类')
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
  if (style === 'month') return `${d.getFullYear()} 年 ${z(d.getMonth() + 1)} 月`
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
  if (days <= 0) return t('今天')
  if (days === 1) return t('昨天')
  if (days < 30) return days + t('天前')
  return ''
}

/** 链接：一律用 ctx.postUrl（不自行拼 #/p/ 或 #/post/） */
export function postHref(ctx, p) {
  if (ctx?.postUrl) return ctx.postUrl(p)
  return '#'
}

/** 无文章时的兜底地址（相对路径，不含 hash 锚点） */
export const LIST_HREF = '#/blog?view=public'
export const RSS_HREF = '/api/v1/blog/feed.xml'

/** 序号：01 / 02 …（版面序号，非数据字段） */
export function seqNo(i) {
  return String(i + 1).padStart(2, '0')
}

/**
 * 分类 → 稳定配色（同一分类永远同一色，便于读者建立视觉记忆）。
 * 色板取自设计稿里那组低饱和自然渐变，只作装饰，不表达任何数据语义。
 */
export function catTone(name) {
  let h = 0
  for (const ch of String(name || t('文'))) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return 'tone' + (h % 4 + 1)
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

/** 年份归档分组（updated_at 派生，非接口提供） */
export function yearGroups(posts) {
  const map = new Map()
  for (const p of posts || []) {
    const y = postDate(p, 'year') || t('未知')
    if (!map.has(y)) map.set(y, [])
    map.get(y).push(p)
  }
  return [...map.entries()]
    .sort((a, b) => String(b[0]).localeCompare(String(a[0])))
    .map(([year, list]) => ({ year, list }))
}

/** 月份归档分组（updated_at 派生） */
export function monthGroups(posts) {
  const map = new Map()
  for (const p of posts || []) {
    const m = postDate(p, 'month') || t('未知')
    if (!map.has(m)) map.set(m, [])
    map.get(m).push(p)
  }
  return [...map.entries()]
    .sort((a, b) => String(b[0]).localeCompare(String(a[0])))
    .map(([month, list]) => ({ month, list }))
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
