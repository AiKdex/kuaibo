<template>
  <!-- 博客功能插件挂载点：渲染该挂载点下已启用的插件组件，注入通用数据契约 ctx -->
  <div class="blog-plugin-slot" :data-mount="mount">
    <component
      v-for="p in list"
      :is="p.component"
      :key="p.id"
      :ctx="ctx"
      class="blog-plugin"
    />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { pluginsFor } from '@/blogPlugins'

const props = defineProps({
  mount: { type: String, required: true }, // post_bottom | sidebar | list_item | head
  ctx: { type: Object, default: () => ({}) }
})

const list = computed(() => pluginsFor(props.mount))
</script>

<style scoped>
.blog-plugin-slot {
  display: block;
}
.blog-plugin-slot:empty {
  display: none;
}
</style>
