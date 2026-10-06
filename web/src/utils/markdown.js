// Markdown 渲染：marked + marked-highlight + highlight.js + DOMPurify（XSS 防护）
// 阶段 1：文本类文件（md/txt/log/json/html）转 HTML 供阅读视图展示
// 阶段 2（2026-09-17）：KaTeX 数学公式（$…$ / $$…$$）+ Mermaid 图增强 + TOC 提取
import { marked } from 'marked'
import { markedHighlight } from 'marked-highlight'
import hljs from 'highlight.js/lib/common'
import DOMPurify from 'dompurify'
import katex from 'katex'
import 'katex/dist/katex.min.css'

// marked v5 起 highlight 选项移出核心，须用 marked-highlight 扩展（否则代码高亮静默失效）
marked.use(
  markedHighlight({
    langPrefix: 'hljs language-',
    highlight(code, lang) {
      if (lang && hljs.getLanguage(lang)) {
        try {
          return hljs.highlight(code, { language: lang }).value
        } catch (_) {
          /* fallthrough */
        }
      }
      return hljs.highlightAuto(code).value
    }
  })
)

marked.setOptions({
  gfm: true,
  breaks: true
})

// Obsidian 兼容双链渲染：[[标题]] / [[标题#锚点]] / [[标题|显示名]] → 可点击链接（阅读态）。
// 渲染为 <a class="wiki-link" data-title data-anchor>，点击由阅读页 click 委托解析跳转；
// 未匹配文件时前端置灰（Obsidian 未创建链接同款）。
marked.use({
  extensions: [
    {
      name: 'wikilink',
      level: 'inline',
      start(src) {
        return src.indexOf('[[')
      },
      tokenizer(src) {
        const m = /^\[\[([^\[\]|#]+?)(?:#([^\[\]|]+?))?(?:\|([^\]]*?))?\]\]/.exec(src)
        if (!m) return undefined
        return {
          type: 'wikilink',
          raw: m[0],
          title: m[1].trim(),
          anchor: (m[2] || '').trim(),
          display: (m[3] || m[1]).trim()
        }
      },
      renderer(token) {
        const title = token.title.replace(/"/g, '&quot;')
        const anchor = token.anchor.replace(/"/g, '&quot;')
        const display = token.display || token.title
        return `<a class="wiki-link" data-title="${title}" data-anchor="${anchor}">${display}</a>`
      }
    }
  ]
})

// @提及渲染（B12）：两条规则与后端 service/mention.go 的 mentionExplicitRe / mentionPlainRe
// **逐字一致** —— 「会发通知的 @」和「被高亮的 @」必须是同一批，两边各写一套迟早错配。
// 顺带修掉一个真缺陷：`@[显示名](user_id)` 是合法 Markdown 链接语法，此前会被渲染成
// `<a href="user_id">`，即一条指向 user_id 的死链（点了 404）。
// 纯写法的前界检查只能放在 start()：marked 的 tokenizer 只拿到从 '@' 开始的剩余串，看不到前一个字符。
marked.use({
  extensions: [
    {
      name: 'mention',
      level: 'inline',
      start(src) {
        for (let i = 0; i < src.length; i++) {
          if (src[i] !== '@') continue
          // 前一个字符是「词内字符」→ 跳过（否则 someone@example.com 里的域名会被高亮）
          if (i > 0 && /[A-Za-z0-9_.+-]/.test(src[i - 1])) continue
          return i
        }
        return undefined
      },
      tokenizer(src) {
        let m = /^@\[([^\]\n]{1,64})\]\(([0-9a-zA-Z_-]{8,64})\)/.exec(src)
        if (m) return { type: 'mention', raw: m[0], name: m[1], userId: m[2] }
        m = /^@([\p{L}\p{N}_.-]{1,32})/u.exec(src)
        if (m) return { type: 'mention', raw: m[0], name: m[1], userId: '' }
        return undefined
      },
      renderer(token) {
        const esc = (v) =>
          String(v)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
        const id = token.userId ? ` data-mention-id="${esc(token.userId)}"` : ''
        // 本壳没有公开用户主页 → 不做跳转（不造死链）；显式写法把 user_id 落到 data 属性。
        return `<span class="mention"${id}>@${esc(token.name)}</span>`
      }
    }
  ]
})

// 标题锚点 id：h- 前缀 + 纯文本（去内联 HTML）转安全串；重复标题追加序号保唯一。
// 供 TOC 胶囊（MarkdownArticle）与检索 chunk 锚点跳转共用。
const headingIdSeen = new Map()
function headingIdFor(text) {
  const t = (text || '').replace(/<[^>]*>/g, '').trim()
  const base = ('h-' + t).slice(0, 96)
  const n = headingIdSeen.get(base) || 0
  headingIdSeen.set(base, n + 1)
  return n === 0 ? base : base + '-' + (n + 1)
}

// 标题渲染器：给 h1-h6 注入稳定 id（重复标题自动 -N）
marked.use({
  renderer: {
    heading(token) {
      const depth = token.depth || 1
      const inner = this.parser.parseInline(token.tokens)
      const text = token.text || ''
      const id = headingIdFor(text)
      return `<h${depth} id="${id}">${inner}</h${depth}>`
    }
  }
})

export function renderMarkdown(src) {
  if (!src) return ''
  headingIdSeen.clear()
  // 博客公开页 /p/{token} 任何人可访问：marked 默认不过滤 HTML，
  // 必须经 DOMPurify 消毒，防 <script>/<img onerror> 等 XSS payload 执行。
  const raw = marked.parse(src)
  return DOMPurify.sanitize(raw)
}

// ---- KaTeX 数学公式（$…$ 行内 / $$…$$ 块级）----
// 注意：货币场景 "$100 和 $200" 不触发（行内开价符后不允许紧跟空白/美元）。
function renderMath(tex, displayMode) {
  try {
    return katex.renderToString(tex, { displayMode, throwOnError: false, output: 'html' })
  } catch (_) {
    return '<code>' + tex.replace(/&/g, '&amp;').replace(/</g, '&lt;') + '</code>'
  }
}

marked.use({
  extensions: [
    {
      name: 'blockMath',
      level: 'block',
      start(src) {
        return src.indexOf('$$')
      },
      tokenizer(src) {
        const m = /^\$\$([\s\S]+?)\$\$(?:\n|$)/.exec(src)
        if (m) return { type: 'blockMath', raw: m[0], text: m[1].trim() }
        return undefined
      },
      renderer(token) {
        return '<div class="math-block">' + renderMath(token.text, true) + '</div>'
      }
    },
    {
      name: 'inlineMath',
      level: 'inline',
      start(src) {
        const i = src.indexOf('$')
        return i < 0 ? undefined : i
      },
      tokenizer(src) {
        // 开价符后紧跟空白则视为普通 $ 字符（"价格 $5 起"）
        const m = /^\$(?!\s)(?:\\.|[^$\n])+?(?<!\s)\$/.exec(src)
        if (m && m[0].length > 2) return { type: 'inlineMath', raw: m[0], text: m[0].slice(1, -1) }
        return undefined
      },
      renderer(token) {
        return renderMath(token.text, false)
      }
    }
  ]
})

// ---- Mermaid 图增强 ----
// 渲染结果中 ```mermaid 代码块保持为 <pre><code class="language-mermaid">（可过 DOMPurify），
// 由本函数在挂载后按需动态 import('mermaid') 替换为 SVG —— 不用图则不加载（代码分包）。
let mermaidReady = null
export async function enhanceMermaid(rootEl) {
  if (!rootEl || !rootEl.querySelectorAll) return
  const blocks = rootEl.querySelectorAll('pre > code.language-mermaid')
  if (!blocks.length) return
  if (!mermaidReady) {
    mermaidReady = import('mermaid').then((m) => {
      m.default.initialize({ startOnLoad: false, theme: 'neutral', securityLevel: 'strict' })
      return m.default
    })
  }
  let mermaid
  try {
    mermaid = await mermaidReady
  } catch (_) {
    return // 加载失败保留源码展示
  }
  let seq = 0
  for (const code of blocks) {
    const pre = code.parentElement
    if (!pre || pre.dataset.mmdDone) continue
    pre.dataset.mmdDone = '1'
    try {
      const { svg } = await mermaid.render('mmd-' + Date.now() + '-' + seq++, code.textContent || '')
      const div = document.createElement('div')
      div.className = 'mermaid-figure'
      div.innerHTML = svg // securityLevel: strict，mermaid 内部已消毒
      pre.replaceWith(div)
    } catch (_) {
      delete pre.dataset.mmdDone // 渲染失败还原为源码
    }
  }
}

// ---- TOC 提取（编辑器大纲 / 阅读页目录共用）----
// 与标题锚点 id（headingIdFor）同规则；跳转时按 idx 兜底（防个别字符差异失配）。
export function extractToc(src) {
  const out = []
  const seen = new Map()
  let inFence = false
  for (const line of String(src || '').split(/\r?\n/)) {
    if (/^\s*(```|~~~)/.test(line)) {
      inFence = !inFence
      continue
    }
    if (inFence) continue
    const m = /^(#{1,6})\s+(.+?)\s*#*\s*$/.exec(line)
    if (!m) continue
    const text = m[2].replace(/<[^>]*>/g, '').trim()
    if (!text) continue
    const key = ('h-' + text).slice(0, 96)
    const n = seen.get(key) || 0
    seen.set(key, n + 1)
    out.push({ depth: m[1].length, text, id: n === 0 ? key : key + '-' + (n + 1), idx: out.length })
  }
  return out
}
