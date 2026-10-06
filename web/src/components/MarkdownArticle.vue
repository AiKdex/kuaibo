<template>
  <article ref="artRef" class="md-article" :style="readingCssVars">
    <!-- 文章头：标题 + 元信息（博客化） -->
    <header class="art-head">
      <h1 class="art-title">{{  title  }}</h1>
      <div v-if="meta && meta.length" class="art-meta">
        <span v-for="(m, i) in meta" :key="i" class="art-meta-item">
          <template v-if="i > 0"><span class="art-meta-dot">·</span></template>{{  m  }}
        </span>
      </div>
      <div class="art-divider"></div>
    </header>

    <!-- 正文（Markdown 渲染结果，博客排版） -->
    <div class="art-body" v-html="html"></div>

    <!-- 页脚（可选：公开页显示来源） -->
    <footer v-if="footer" class="art-footer">{{  footer  }}</footer>

    <!-- 悬浮 TOC 胶囊：正文标题 ≥2 个时显示；点击滚动到锚点，滚动监听高亮当前章节 -->
    <div v-if="toc.length >= 2" class="md-toc" :class="{ open: tocOpen }">
      <button class="md-toc-toggle" :title="tocOpen ? $t('收起目录') : $t('展开目录')" @click="tocOpen = !tocOpen">
        <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="8" y1="6" x2="21" y2="6" /><line x1="8" y1="12" x2="21" y2="12" /><line x1="8" y1="18" x2="21" y2="18" />
          <line x1="3" y1="6" x2="3.01" y2="6" /><line x1="3" y1="12" x2="3.01" y2="12" /><line x1="3" y1="18" x2="3.01" y2="18" />
        </svg>
        <span class="md-toc-toggle-label">{{  $t('目录')  }}</span>
      </button>
      <div v-show="tocOpen" class="md-toc-panel">
        <div class="md-toc-title">{{  $t('本文目录')  }}</div>
        <div
          v-for="t in toc"
          :key="t.id"
          class="md-toc-item"
          :class="{ active: t.id === activeId }"
          :style="{ paddingLeft: 8 + (t.level - 1) * 14 + 'px' }"
          :title="t.text"
          @click="jumpTo(t.id)"
        >{{  t.text  }}</div>
      </div>
    </div>
  </article>
</template>

<script setup>
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { useReadingStore } from '@/stores/reading'
import { t } from '@/i18n'

const props = defineProps({
  title: { type: String, default: '' },
  html: { type: String, default: '' },
  meta: { type: Array, default: () => [] }, // 如 ['4.2 KB', t('2026-09-11 更新'), t('本地存储')]
  footer: { type: String, default: '' }
})

const reading = useReadingStore()
const readingCssVars = computed(() => reading.cssVars())

// —— 悬浮 TOC：正文渲染后提取 h1-h3（markdown.js 已注入 id 锚点）——
const artRef = ref(null)
const toc = ref([])
const tocOpen = ref(false)
const activeId = ref('')

function collectToc() {
  const root = artRef.value
  if (!root) return
  const heads = root.querySelectorAll('.art-body h1, .art-body h2, .art-body h3')
  const items = []
  heads.forEach((h) => {
    const id = h.getAttribute('id')
    if (!id) return
    items.push({ id, text: (h.textContent || '').trim().slice(0, 40), level: parseInt(h.tagName[1], 10) })
  })
  toc.value = items
  if (items.length >= 2) tocOpen.value = true
}

function jumpTo(id) {
  const root = artRef.value
  const el = root && root.querySelector('#' + CSS.escape(id))
  if (!el) return
  el.scrollIntoView({ behavior: 'smooth', block: 'start' })
  activeId.value = id
}

function onScroll() {
  const root = artRef.value
  if (!root) return
  const heads = toc.value
  if (!heads.length) return
  // 取最后一个位于视口上缘之上的标题为当前
  let cur = ''
  for (const t of heads) {
    const el = root.querySelector('#' + CSS.escape(t.id))
    if (el && el.getBoundingClientRect().top <= 90) cur = t.id
    else break
  }
  activeId.value = cur
}

function scheduleCollect() {
  nextTick(() => {
    collectToc()
    if (toc.value.length >= 2) {
      onScroll()
      // F3 复核：onScroll 是 setup 作用域内的稳定函数引用，按 DOM 规范
      // addEventListener 对 (type, listener, capture) 三元组相同者本就只登记一次，
      // 因此此前"正文每次变更叠加一个监听"并不成立。这里仍先 remove 再 add，
      // 把幂等性写成显式约束 —— 万一将来 onScroll 变成闭包，这行能防住真正的叠加泄漏。
      window.removeEventListener('scroll', onScroll)
      window.addEventListener('scroll', onScroll, { passive: true })
    }
  })
}

watch(
  () => props.html,
  () => scheduleCollect(),
  { immediate: true } // 挂载时 html 可能已是终值（父组件先加载内容再渲染本组件），immediate 兜底
)

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
})
</script>

<style scoped>
/* 博客化文章排版：应用内阅读页与公开博客页共用（发布/分享/知识库展示复用） */
.md-article {
  max-width: var(--reading-maxw, 100%);
  margin: 0 auto;
  padding: 48px 40px 88px;
  font-family: var(--reading-font-family, var(--font));
  font-size: var(--reading-size, 16px);
  line-height: var(--reading-lineheight, 1.8);
  color: var(--reading-text-active, var(--text));
  background: var(--reading-bg-active, var(--reading-bg));
  word-break: break-word;
}

/* 文章头 */
.art-head {
  margin-bottom: 36px;
}

.art-title {
  font-size: 2.05em;
  line-height: 1.35;
  font-weight: 700;
  margin: 0 0 16px;
  letter-spacing: 0.01em;
  color: var(--reading-text-active, var(--text));
}

.art-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  font-size: 13px;
  color: var(--text-3);
}

.art-meta-dot {
  margin: 0 6px;
  opacity: 0.6;
}

.art-divider {
  height: 1px;
  background: var(--border);
  margin-top: 24px;
}

/* 正文排版（对齐原阅读页 doc-body 体系，博客化微调） */
.art-body :deep(h1) {
  font-size: 1.72em;
  line-height: 1.4;
  margin: 1.6em 0 0.6em;
  font-weight: 700;
}

.art-body :deep(h2) {
  font-size: 1.34em;
  line-height: 1.4;
  margin: 1.8em 0 0.55em;
  font-weight: 700;
  padding-bottom: 0.3em;
  border-bottom: 1px solid var(--border);
}

.art-body :deep(h3) {
  font-size: 1.12em;
  margin: 1.5em 0 0.5em;
  font-weight: 600;
}

.art-body :deep(h4) {
  font-size: 1em;
  margin: 1.4em 0 0.4em;
  font-weight: 600;
}

.art-body :deep(p) {
  margin: 1.2em 0;
}

.art-body :deep(a) {
  color: var(--primary);
  text-decoration: underline;
  text-underline-offset: 2px;
}

.art-body :deep(ul),
.art-body :deep(ol) {
  margin: 1.1em 0;
  padding-left: 1.6em;
}

.art-body :deep(li) {
  margin: 0.35em 0;
}

.art-body :deep(blockquote) {
  margin: 1.3em 0;
  padding: 0.5em 1.2em;
  border-left: 3px solid var(--primary);
  background: var(--surface-2);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  color: var(--text-2);
}

.art-body :deep(code) {
  font-family: var(--font-mono);
  font-size: 0.88em;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 0.15em 0.4em;
}

.art-body :deep(pre) {
  margin: 1.3em 0;
  padding: 14px 16px;
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  border: 1px solid var(--border);
  overflow-x: auto;
}

.art-body :deep(pre code) {
  background: none;
  border: none;
  padding: 0;
  font-size: 0.86em;
  line-height: 1.6;
}

.art-body :deep(img) {
  max-width: 100%;
  border-radius: var(--radius-sm);
  margin: 1.2em 0;
}

.art-body :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin: 1.3em 0;
  font-size: 0.95em;
}

.art-body :deep(th),
.art-body :deep(td) {
  border: 1px solid var(--border);
  padding: 8px 12px;
  text-align: left;
}

.art-body :deep(th) {
  background: var(--surface-2);
  font-weight: 600;
}

.art-body :deep(hr) {
  border: none;
  border-top: 1px solid var(--border);
  margin: 2em 0;
}

.art-body :deep(strong) {
  font-weight: 700;
}

/* 页脚 */
.art-footer {
  margin-top: 64px;
  padding-top: 20px;
  border-top: 1px solid var(--border);
  font-size: 13px;
  color: var(--text-3);
  text-align: center;
}

@media (max-width: 768px) {
  .md-article {
    padding: 32px 20px 64px;
  }

  .art-title {
    font-size: 1.6em;
  }
}

/* —— 悬浮 TOC 胶囊（长文档导航；点击滚动锚点 + 滚动监听高亮当前章节）—— */
.md-toc {
  position: fixed;
  right: 18px;
  top: 50%;
  transform: translateY(-50%);
  z-index: 60;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
}
.md-toc-toggle {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 7px 10px;
  border: 1px solid var(--border, rgba(120, 130, 160, 0.25));
  border-radius: 20px;
  background: var(--reading-bg-active, #fff);
  color: var(--text-2, #57606a);
  cursor: pointer;
  font-size: 12px;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.08);
  transition: all 0.15s;
}
.md-toc-toggle:hover {
  color: var(--accent, #6c5ce7);
  border-color: var(--accent, #6c5ce7);
}
.md-toc-toggle-label {
  display: none;
}
.md-toc.open .md-toc-toggle-label {
  display: inline;
}
.md-toc-panel {
  margin-top: 8px;
  width: 248px;
  max-height: 60vh;
  overflow-y: auto;
  padding: 10px;
  border: 1px solid var(--border, rgba(120, 130, 160, 0.25));
  border-radius: 10px;
  background: var(--reading-bg-active, #fff);
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.12);
}
.md-toc-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-3, #8b949e);
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--border, rgba(120, 130, 160, 0.15));
}
.md-toc-item {
  padding: 5px 8px;
  font-size: 12.5px;
  line-height: 1.45;
  color: var(--text-2, #57606a);
  cursor: pointer;
  border-radius: 6px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.md-toc-item:hover {
  background: var(--hover, rgba(108, 92, 231, 0.08));
  color: var(--accent, #6c5ce7);
}
.md-toc-item.active {
  background: var(--accent, #6c5ce7);
  color: #fff;
}
@media (max-width: 900px) {
  .md-toc {
    right: 10px;
    top: auto;
    bottom: 18px;
    transform: none;
  }
  .md-toc-panel {
    width: 220px;
    max-height: 50vh;
  }
}
@media (max-width: 640px) {
  .md-toc {
    display: none;
  }
}
</style>
