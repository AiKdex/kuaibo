<template>
  <Teleport to="body">
    <div class="toast-host">
      <TransitionGroup name="toast-slide">
        <div v-for="t in store.items" :key="t.id" class="toast-item">
          <AikIcon name="check" :size="13" class="ti" />
          <span>{{  t.text  }}</span>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup>
import AikIcon from './AikIcon.vue'
import { useToastStore } from '@/stores/toast'

const store = useToastStore()
</script>

<style scoped>
.toast-host {
  position: fixed;
  top: 18px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 9999;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  pointer-events: none;
}

.toast-item {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 9px 18px;
  border-radius: 10px;
  background: rgba(24, 26, 32, 0.92);
  color: #f5f6f8;
  font-size: 13px;
  line-height: 1.4;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.28);
  backdrop-filter: blur(8px);
  white-space: nowrap;
  max-width: min(520px, 80vw);
  overflow: hidden;
  text-overflow: ellipsis;
}

.ti {
  color: #4ade80;
  flex-shrink: 0;
}

/* 滑动出现/消失 */
.toast-slide-enter-active {
  transition: all 0.28s cubic-bezier(0.21, 1.02, 0.73, 1);
}

.toast-slide-leave-active {
  transition: all 0.24s ease-in;
}

.toast-slide-enter-from {
  opacity: 0;
  transform: translateY(-16px) scale(0.96);
}

.toast-slide-leave-to {
  opacity: 0;
  transform: translateY(-10px) scale(0.96);
}
</style>
