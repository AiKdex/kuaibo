<template>
  <div class="cd-mask" @click.self="$emit('cancel')">
    <div class="cd-modal">
      <h3 class="cd-title">{{  title  }}</h3>
      <p v-if="message" class="cd-desc">{{  message  }}</p>
      <div class="cd-actions">
        <button class="btn btn-sm" @click="$emit('cancel')">{{  cancelText  }}</button>
        <button class="btn btn-sm" :class="danger ? 'btn-danger' : 'btn-primary'" @click="$emit('confirm')">
          {{  confirmText  }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>import { t } from '@/i18n'

defineProps({
  title: { type: String, required: true },
  message: { type: String, default: '' },
  confirmText: { type: String, default: t('确定') },
  cancelText: { type: String, default: t('取消') },
  danger: { type: Boolean, default: false }
})
defineEmits(['confirm', 'cancel'])
</script>

<style scoped>
.cd-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 999;
}
.cd-modal {
  width: 360px;
  max-width: calc(100vw - 48px);
  background: var(--surface-1, #fff);
  border: 1px solid var(--border, #e4e3dd);
  border-radius: 12px;
  padding: 20px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.18);
}
.cd-title {
  font-size: 15px;
  font-weight: 600;
  margin: 0 0 10px;
  color: var(--text, #1a1b1c);
  line-height: 1.5;
}
.cd-desc {
  font-size: 13px;
  color: var(--text-2, #6b7280);
  margin: 0 0 18px;
  line-height: 1.6;
  word-break: break-word;
}
.cd-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
