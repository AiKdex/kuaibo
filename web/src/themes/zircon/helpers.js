/**
 * 青璃 Zircon 主题公共派生层。
 *
 * 严格按《AiKlog 博客主题开发规范（第三方完整交付版）》v1.2：
 *  §2.2 posts 条目字段 · §2.3 标题/分类/日期派生 · §4.7 归档派生 · §7.1 AI 问答。
 * 只使用契约字段 token / path / preview / file.{name,slug,updated_at,size}，不臆造任何字段。
 */import { t } from '@/i18n'

import { computed, inject } from 'vue'

/** 兼容 ref 与普通对象两种包装（不同宿主版本） */
export function useBlog() {
  const raw = inject('themeContext', null)
  return computed(() => raw?.value || raw || { posts: [], tags: [], loading: true, error: '' })
}

/** 标题：去扩展名（.md 与 .markdown 都要处理）→ 取末段 */
export function postTitle(p) {
  const n = p?.file?.name || p?.path || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('无标题')
}

/** 分类：path 首段；无 '/' 返回空串 */
export function postCat(p) {
  const path = p?.path || ''
  return path.includes('/') ? path.split('/')[0] : ''
}

/** 分类展示名（补「未分类」兜底） */
export function catName(p) {
  return postCat(p) || t('未分类')
}

/** 日期：file.updated_at（毫秒，兼容秒级与字符串） */
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

/** 摘要：清掉 Markdown 记号后截断（preview 由服务端生成，可能为空串） */
export function postExcerpt(p, len = 96) {
  const s = String(p?.preview || '')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/^#{1,6}\s+[^\n]+$/gm, '')
    .replace(/[#*`>_~]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  return s.length > len ? `${s.slice(0, len)}…` : s
}

/** 附件大小（真实字段，非估算） */
export function postSize(p) {
  const s = p?.file?.size || 0
  if (!s) return ''
  if (s < 1024) return `${s} B`
  if (s < 1024 * 1024) return `${(s / 1024).toFixed(1)} KB`
  return `${(s / 1024 / 1024).toFixed(2)} MB`
}

/** 单篇列表项的唯一键：博客为单分享，token 恒为 blog，用 path 兜底区分 */
export function postKey(p, i = 0) {
  return p?.path || p?.file?.slug || `zc-${i}`
}

/** 链接：一律用宿主注入的 postUrl（SPA 文章页），不自行拼 hash 路径 */
export function postHref(ctx, p) {
  try {
    return ctx?.postUrl ? ctx.postUrl(p) : ''
  } catch {
    return ''
  }
}

/** 分享链接：SSR 静态页 /{slug}（canonical / 分享快照场景） */
export function postShareHref(ctx, p) {
  try {
    return ctx?.postSsrUrl ? ctx.postSsrUrl(p) : ''
  } catch {
    return ''
  }
}

/** 分类计数（按篇数倒序，同数按名称稳定排序） */
export function catCounts(posts) {
  const map = new Map()
  for (const p of posts || []) {
    const name = catName(p)
    map.set(name, (map.get(name) || 0) + 1)
  }
  return [...map.entries()]
    .sort((a, b) => b[1] - a[1] || String(a[0]).localeCompare(String(b[0]), 'zh'))
    .map(([name, count]) => ({ name, count }))
}

/** 年份分组（由 posts 的 updated_at 前端派生，系统不提供归档接口） */
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

/**
 * 内页参数读取：优先 ctx.page（系统侧注入），缺失时回退解析 SPA hash —— 只读不写，
 * 全程不碰 location.hash（规范 §3.2 三条硬约束）。
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
    if (m && m[1]) {
      try {
        return decodeURIComponent(m[1])
      } catch {
        return m[1]
      }
    }
  }
  return ''
}

/** 作者名：file.author 属系统侧待透出字段，一律可选链降级 */
export function postAuthor(p) {
  return String(p?.file?.author?.name || p?.file?.author || '')
}

/** 站内固定链接 */
export const LIST_HREF = '#/blog?view=public'
export const SSR_LIST_HREF = '/blog'
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
    return data.reply || t('暂无回答')
  } catch {
    return t('请求失败，请稍后重试')
  } finally {
    onLoading?.(false)
  }
}
