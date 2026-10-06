/**
 * EmForum 主题公共派生根 —— 严格按《AiKlog 博客主题开发规范（第三方完整交付版）》
 * §2.2 posts 字段 / §2.3 标题·分类·日期派生 实现，禁止臆造字段。
 */
import { t } from '@/i18n'

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
  if (style === 'time') return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())} ${z(d.getHours())}:${z(d.getMinutes())}`
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}`
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

/** 附件大小 */
export function postSize(p) {
  const s = p?.file?.size || 0
  if (!s) return ''
  return s > 1024 * 1024 ? (s / 1024 / 1024).toFixed(1) + ' MB' : Math.max(1, Math.round(s / 1024)) + ' KB'
}

/** 距今天数（用于归档/排序展示） */
export function daysAgo(p) {
  const v = p?.file?.updated_at
  if (!v) return ''
  const days = Math.floor((Date.now() - v) / 86400000)
  if (days <= 0) return t('今天')
  if (days === 1) return t('昨天')
  if (days < 30) return days + t('天前')
  return ''
}

/** 链接：一律用 ctx.postUrl（SSR 地址），不自行拼 #/p/ */
export function postHref(ctx, p) {
  if (ctx?.postUrl) return ctx.postUrl(p)
  return '#'
}

/** 版块（分类）图标与配色轮换 */
const BOARD_ICONS = ['✦', '❋', '✎', '◈', '★', '✺', '❖']
export function boardIcon(index) {
  return BOARD_ICONS[index % BOARD_ICONS.length]
}
/** 由分类名稳定哈希出 3 套头像配色 */
export function boardColor(name) {
  let h = 0
  for (const ch of String(name || t('文'))) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return 'c' + (h % 3 + 1)
}
/** 版块计数（分类 → 篇数） */
export function boardCounts(posts) {
  const map = new Map()
  for (const p of posts || []) {
    const name = postCat(p) || t('未分类')
    map.set(name, (map.get(name) || 0) + 1)
  }
  return [...map.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, count], i) => ({ name, count, icon: boardIcon(i) }))
}

/**
 * 内页参数读取：优先 ctx.page（二阶系统侧注入），
 * 缺失时回退解析 SPA hash（#/blog/cat/xxx、#/blog?q=xxx…），保证当前版本可预览。
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

/** 页头/页脚/侧栏共用链接 */
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
    if (res.status === 429) return t('提问太频繁（每 5 分钟限 8 次），请稍后再试')
    const data = await res.json().catch(() => ({}))
    return data.reply || t('暂无回答')
  } catch {
    return t('请求失败，请稍后重试')
  } finally {
    onLoading?.(false)
  }
}
