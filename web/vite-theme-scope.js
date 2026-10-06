/**
 * Vite 插件：博客主题 CSS 按「视图」自动加作用域。
 *
 * 解决的问题：主题 style.css 被全局 import（让文章页也跟主题），但若主题在
 * 列表页与文章页复用同名类（如 .bt-cover / .bt-side），裸选择器会跨视图泄漏，
 * 导致「封面被盖 / 侧栏错位 / 跑马灯闪烁」等回归。
 *
 * 通用做法（一次生效所有主题，主题作者无需手加前缀）：
 *   - themes/<id>/style.css   → 整体加 `.th-<id>` 作用域（列表 + 文章共享的令牌与原子类）
 *   - themes/<id>/post.css    → 整体加 `.th-<id>.th-view-post` 作用域（仅文章视图生效）
 *
 * 宿主配合：列表视图根包 `.th-<id> .th-view-list`，文章视图根包 `.th-<id> .th-view-post`。
 * 这样 post.css 的选择器天然只在文章页命中，列表页永远拿不到，跨视图零泄漏；
 * 而 style.css 的共享原子（.bt-btn / .bt-tag 等）两端都能用。
 *
 * 细节：
 *   - `:root` 与 `.th-<id>` 令牌块保持为 `.th-<id>`（compound，挂在视图根上，两端都可继承），
 *     不变成后代选择器，否则令牌会丢失。
 *   - `@keyframes` 内部的步进选择器（from/to/0%~100%）不加深作用域前缀。
 */
import postcss from 'postcss'

const THEME_CSS_RE = /[/\\]web[/\\]src[/\\]themes[/\\]([^/\\]+)[/\\](style|post)\.css$/

export function themeViewScope() {
  return {
    name: 'aiklog-theme-view-scope',
    async transform(code, id) {
      const m = id.match(THEME_CSS_RE)
      if (!m) return null

      const tid = m[1]
      const isPost = m[2] === 'post'
      const idClass = `.th-${tid}`
      // style.css：列表+文章共享 → 作用域到 .th-<id>
      // post.css ：文章专属   → 作用域到 .th-<id>.th-view-post（仅文章视图）
      const viewClass = isPost ? `${idClass}.th-view-post` : idClass

      const scopePlugin = {
        postcssPlugin: 'aiklog-theme-scope-internal',
        Once(root) {
          root.walkRules((rule) => {
            const parent = rule.parent
            // 跳过 @keyframes 步进选择器
            if (parent && parent.type === 'atrule' && /keyframes$/i.test(parent.name)) return
            rule.selectors = rule.selectors.map((sel) => {
              const s = sel.trim()
              // 令牌块：:root / .th-<id> 保持为 .th-<id>（compound，挂在视图根）
              if (s === ':root' || s === idClass) return idClass
              return `${viewClass} ${s}`
            })
          })
        },
      }

      const result = await postcss([scopePlugin]).process(code, { from: id, map: false })
      return { code: result.css, map: null }
    },
  }
}

export default themeViewScope
