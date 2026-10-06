<template>
  <!-- 公开页访客入口：未登录 → 登录 / 注册；已登录 → 控制台 -->
  <div class="ge" :class="`ge--${variant}`">
    <template v-if="!authed">
      <a class="ge-btn" href="#/desk" :title="$t('登录爱库录控制台')">{{  $t('登录')  }}</a>
      <a class="ge-btn ge-btn--primary" href="#/desk?mode=register" :title="$t('注册新账号')">{{  $t('注册')  }}</a>
    </template>
    <a v-else class="ge-btn" href="#/files" :title="$t('进入控制台')">{{ $t('控制台') }}</a>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { isAuthed } from '@/api'

defineProps({
  // dock：悬浮右下（主题页无宿主顶栏时用）；bar：行内（宿主顶栏内用）
  variant: { type: String, default: 'dock' }
})

const authed = computed(() => isAuthed())
</script>

<style scoped>
.ge {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 悬浮形态：右下角固定，避开右上角的主题切换器 */
.ge--dock {
  position: fixed;
  right: 14px;
  bottom: 16px;
  z-index: 9997;
}

.ge-btn {
  display: inline-flex;
  align-items: center;
  height: 30px;
  padding: 0 14px;
  border-radius: 15px;
  border: 1px solid rgba(0, 0, 0, 0.12);
  background: rgba(255, 255, 255, 0.94);
  color: #111827;
  font-size: 13px;
  line-height: 1;
  text-decoration: none;
  white-space: nowrap;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  backdrop-filter: blur(6px);
  transition: transform 0.12s, box-shadow 0.12s, background 0.12s;
}

.ge-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.14);
}

.ge-btn--primary {
  background: #0d7a6a;
  border-color: #0d7a6a;
  color: #fff;
  font-weight: 600;
}

.ge-btn--primary:hover {
  background: #0b6a5c;
}

/* 行内形态：贴合宿主顶栏，去掉阴影与位移 */
.ge--bar {
  flex-shrink: 0;
}

.ge--bar .ge-btn {
  height: 26px;
  padding: 0 10px;
  border-radius: 6px;
  font-size: 12px;
  background: transparent;
  box-shadow: none;
  backdrop-filter: none;
}

.ge--bar .ge-btn--primary {
  background: #0d7a6a;
  border-color: #0d7a6a;
  color: #fff;
}

.ge--bar .ge-btn:hover {
  transform: none;
  box-shadow: none;
  background: rgba(0, 0, 0, 0.05);
}

.ge--bar .ge-btn--primary:hover {
  background: #0b6a5c;
}
</style>
