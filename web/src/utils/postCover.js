/**
 * 文章封面提取（全局唯一来源）—— 博客所有主题共用，避免各主题各写一份。
 *
 * 优先级：
 *   1. 编辑器设置/上传的封面字段（post.cover 或 post.file.cover）
 *   2. 摘要(preview)里的第一张图片
 *   3. 都没有 → 返回空串（调用方据此回退占位或省略）
 *
 * 注意：封面字段经 BlogView 数据层已统一注入为 post.cover，
 * 但本函数仍兼容「只有 post.file.cover」的旧路径（如主题内页直接拿原始 post）。
 */
export function postCover(post) {
  const p = post || {}
  const c = p.cover || p.file?.cover
  if (typeof c === 'string' && c.trim()) return c.trim()
  const s = p.preview || ''
  const m = /!\[[^\]]*\]\((https?:\/\/[^)\s]+|\/[^)\s]+)\)/.exec(s)
  return m ? m[1] : ''
}
