<template>
  <router-view />
  <ToastHost />
</template>

<script setup>
import ToastHost from '@/components/ToastHost.vue'
// 根组件：所有视图在其中渲染；全局侧栏/顶栏在 AppShell 内；ToastHost 覆盖所有页面
</script>
