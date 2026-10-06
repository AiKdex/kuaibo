/**
 * 暖纸杂志 Parchment 主题公共派生工具 —— 严格按《AiKlog 博客主题开发规范》实现，禁止臆造字段。
 * 仅依赖 themeContext 契约：posts / tags / postUrl / site / page.*。
 */
import { t } from '@/i18n'
import { computed, inject } from 'vue'

/** 兼容 ref 与普通对象两种包装（不同宿主版本） */
export function useBlog() {
  const raw = inject('themeContext', null)
  return computed(() => raw?.value || raw || { posts: [], tags: [], loading: true, error: '' })
}

/** 标题：优先 post.title；否则取 file.name/path 末段去扩展名 */
export function postTitle(p) {
  if (p && p.title) return String(p.title)
  const n = (p && (p.file?.name || p.name || p.path)) || ''
  return String(n).replace(/\.(md|markdown)$/i, '').split('/').pop() || t('无标题')
}

/** 分类：优先 post.cat/category；否则取 path 首段 */
export function postCat(p) {
  if (p && (p.cat || p.category)) return p.cat || p.category
  const path = (p && (p.path || p.file?.path)) || ''
  return path.includes('/') ? path.split('/')[0] : ''
}

/** 日期：updated_at（毫秒，兼容秒级/字符串）；回退 updatedAt / created_at */
export function postDate(p, style = 'ymd') {
  const v = p?.file?.updated_at || p?.updatedAt || p?.file?.created_at || p?.created_at
  if (!v) return ''
  const ms = typeof v === 'number' ? (v > 1e12 ? v : v * 1000) : Date.parse(v)
  if (!Number.isFinite(ms)) return ''
  const d = new Date(ms)
  const z = (n) => String(n).padStart(2, '0')
  if (style === 'month') return `${d.getFullYear()}-${z(d.getMonth() + 1)}`
  if (style === 'short') return `${z(d.getMonth() + 1)}-${z(d.getDate())}`
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}`
}

/** 链接：一律用 ctx.postUrl（SSR 地址） */
export function postHref(ctx, p) {
  if (ctx && ctx.postUrl) return ctx.postUrl(p)
  return '#'
}

/** 摘要：清 Markdown 记号后截断 */
export function postSummary(p, len = 90) {
  return (p?.preview || '')
    .replace(/^#+\s.*$/gm, '')
    .replace(/!?\[.*?\]\(.*?\)/g, '')
    .replace(/[#*`>_~]/g, '')
    .trim()
    .slice(0, len)
}

/** 内页参数读取：优先 ctx.page（二阶系统侧注入），缺失时回退解析 SPA hash */
export function readPageParam(ctx, keys, patterns = []) {
  const page = (ctx && ctx.page) || {}
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

export const LIST_HREF = '#/blog?view=public'
export const RSS_HREF = '/api/v1/blog/feed.xml'
