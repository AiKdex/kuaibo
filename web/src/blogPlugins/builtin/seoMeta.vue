<template>
  <!-- head 挂载点示例：公开页注入 SEO meta（description/OG）。协议演示，第三方可替换/删除。
       渲染为隐藏节点，组件挂载/上下文变化时同步 document.head，卸载时移除。 -->
  <span class="head-plug-host" style="display: none"></span>
</template>

<script setup>
import { onMounted, onUnmounted, watch } from 'vue'

const props = defineProps({ ctx: { type: Object, default: () => ({}) } })
let nodes = []

function cleanup() {
  for (const n of nodes) n.remove()
  nodes = []
}

function apply() {
  cleanup()
  const title = props.ctx?.title || '爱库录博客'
  const desc = props.ctx?.description || '由爱库录知识库发布的公开文章'
  const mk = (attr, key, val) => {
    const el = document.createElement('meta')
    el.setAttribute(attr, key)
    el.setAttribute('content', val)
    document.head.appendChild(el)
    nodes.push(el)
  }
  mk('name', 'description', desc)
  mk('property', 'og:title', title)
  mk('property', 'og:description', desc)
  mk('property', 'og:type', 'article')
  const titleEl = document.createElement('title')
  titleEl.textContent = title
  document.head.appendChild(titleEl)
  nodes.push(titleEl)
}

onMounted(apply)
watch(() => props.ctx, apply)
onUnmounted(cleanup)
</script>
