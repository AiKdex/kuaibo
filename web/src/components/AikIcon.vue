<template>
  <svg
    class="aik-icon"
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    draggable="false"
    style="user-select: none; -webkit-user-drag: none;"
  >
    <!-- 根据 name 渲染对应路径（见下方 paths 映射） -->
    <template v-for="(d, i) in paths" :key="i">
      <path v-if="d.type === 'path'" :d="d.d" />
      <circle v-else-if="d.type === 'circle'" :cx="d.cx" :cy="d.cy" :r="d.r" />
      <rect v-else-if="d.type === 'rect'" :x="d.x" :y="d.y" :width="d.w" :height="d.h" :rx="d.rx || 2" />
      <line v-else-if="d.type === 'line'" :x1="d.x1" :y1="d.y1" :x2="d.x2" :y2="d.y2" />
      <polyline v-else-if="d.type === 'polyline'" :points="d.points" />
    </template>
  </svg>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  name: { type: String, required: true },
  size: { type: [Number, String], default: 18 }
})

// AiKlog SVG 图标库：线性图标，stroke 统一 1.8，风格连贯
// 全部基于 24x24 viewBox
const ICONS = {
  // 文件与目录
  folder: [
    { type: 'path', d: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z' }
  ],
  file: [
    { type: 'path', d: 'M6 3h8l4 4v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z' },
    { type: 'polyline', points: '14 3 14 7 18 7' }
  ],
  fileText: [
    { type: 'path', d: 'M6 3h8l4 4v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z' },
    { type: 'polyline', points: '14 3 14 7 18 7' },
    { type: 'line', x1: '9', y1: '12', x2: '15', y2: '12' },
    { type: 'line', x1: '9', y1: '16', x2: '15', y2: '16' }
  ],
  fileImage: [
    { type: 'path', d: 'M6 3h8l4 4v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z' },
    { type: 'polyline', points: '14 3 14 7 18 7' },
    { type: 'circle', cx: '9.5', cy: '12.5', r: '1.5' },
    { type: 'path', d: 'M8 17l3-3 2 2 3-3' }
  ],
  fileAudio: [
    { type: 'path', d: 'M6 3h8l4 4v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z' },
    { type: 'polyline', points: '14 3 14 7 18 7' },
    { type: 'path', d: 'M11 15.5a2 2 0 1 0 1 1.7V13l3-1v3' }
  ],
  fileVideo: [
    { type: 'path', d: 'M6 3h8l4 4v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z' },
    { type: 'polyline', points: '14 3 14 7 18 7' },
    { type: 'polygon', points: '10 12 14 14.5 10 17' }
  ],
  fileArchive: [
    { type: 'path', d: 'M6 3h12v18H6z' },
    { type: 'line', x1: '9', y1: '7', x2: '15', y2: '7' },
    { type: 'line', x1: '9', y1: '11', x2: '15', y2: '11' },
    { type: 'line', x1: '9', y1: '15', x2: '15', y2: '15' }
  ],
  fileCode: [
    { type: 'path', d: 'M6 3h8l4 4v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z' },
    { type: 'polyline', points: '14 3 14 7 18 7' },
    { type: 'polyline', points: '10 12 8 14 10 16' },
    { type: 'polyline', points: '14 12 16 14 14 16' }
  ],
  filePdf: [
    { type: 'path', d: 'M6 3h8l4 4v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z' },
    { type: 'polyline', points: '14 3 14 7 18 7' },
    { type: 'path', d: 'M9 12v4h2a1.5 1.5 0 0 0 0-3H9' }
  ],

  // 导航与操作
  search: [
    { type: 'circle', cx: '11', cy: '11', r: '7' },
    { type: 'line', x1: '21', y1: '21', x2: '16.65', y2: '16.65' }
  ],
  plus: [
    { type: 'line', x1: '12', y1: '5', x2: '12', y2: '19' },
    { type: 'line', x1: '5', y1: '12', x2: '19', y2: '12' }
  ],
  upload: [
    { type: 'path', d: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4' },
    { type: 'polyline', points: '17 8 12 3 7 8' },
    { type: 'line', x1: '12', y1: '3', x2: '12', y2: '15' }
  ],
  download: [
    { type: 'path', d: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4' },
    { type: 'polyline', points: '7 10 12 15 17 10' },
    { type: 'line', x1: '12', y1: '15', x2: '12', y2: '3' }
  ],
  trash: [
    { type: 'polyline', points: '3 6 5 6 21 6' },
    { type: 'path', d: 'M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2' },
    { type: 'line', x1: '10', y1: '11', x2: '10', y2: '17' },
    { type: 'line', x1: '14', y1: '11', x2: '14', y2: '17' }
  ],
  edit: [
    { type: 'path', d: 'M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7' },
    { type: 'path', d: 'M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z' }
  ],
  move: [
    { type: 'polyline', points: '5 9 2 12 5 15' },
    { type: 'polyline', points: '9 5 12 2 15 5' },
    { type: 'polyline', points: '15 19 12 22 9 19' },
    { type: 'polyline', points: '19 9 22 12 19 15' },
    { type: 'line', x1: '2', y1: '12', x2: '22', y2: '12' },
    { type: 'line', x1: '12', y1: '2', x2: '12', y2: '22' }
  ],
  more: [
    { type: 'circle', cx: '5', cy: '12', r: '1' },
    { type: 'circle', cx: '12', cy: '12', r: '1' },
    { type: 'circle', cx: '19', cy: '12', r: '1' }
  ],
  chevronRight: [{ type: 'polyline', points: '9 18 15 12 9 6' }],
  chevronDown: [{ type: 'polyline', points: '6 9 12 15 18 9' }],
  chevronLeft: [{ type: 'polyline', points: '15 18 9 12 15 6' }],
  arrowUp: [
    { type: 'line', x1: '12', y1: '19', x2: '12', y2: '5' },
    { type: 'polyline', points: '5 12 12 5 19 12' }
  ],
  arrowDown: [
    { type: 'line', x1: '12', y1: '5', x2: '12', y2: '19' },
    { type: 'polyline', points: '19 12 12 19 5 12' }
  ],
  arrowLeft: [
    { type: 'line', x1: '19', y1: '12', x2: '5', y2: '12' },
    { type: 'polyline', points: '12 19 5 12 12 5' }
  ],
  close: [
    { type: 'line', x1: '18', y1: '6', x2: '6', y2: '18' },
    { type: 'line', x1: '6', y1: '6', x2: '18', y2: '18' }
  ],
  grid: [
    { type: 'rect', x: '3', y: '3', w: '7', h: '7' },
    { type: 'rect', x: '14', y: '3', w: '7', h: '7' },
    { type: 'rect', x: '3', y: '14', w: '7', h: '7' },
    { type: 'rect', x: '14', y: '14', w: '7', h: '7' }
  ],
  list: [
    { type: 'line', x1: '8', y1: '6', x2: '21', y2: '6' },
    { type: 'line', x1: '8', y1: '12', x2: '21', y2: '12' },
    { type: 'line', x1: '8', y1: '18', x2: '21', y2: '18' },
    { type: 'line', x1: '3', y1: '6', x2: '3.01', y2: '6' },
    { type: 'line', x1: '3', y1: '12', x2: '3.01', y2: '12' },
    { type: 'line', x1: '3', y1: '18', x2: '3.01', y2: '18' }
  ],

  // 智能 / AI
  sparkles: [
    { type: 'path', d: 'M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8L12 3z' },
    { type: 'path', d: 'M19 15l.9 2.6L22.5 18.5l-2.6.9L19 22l-.9-2.6-2.6-.9 2.6-.9L19 15z' }
  ],
  brain: [
    { type: 'path', d: 'M9.5 2A2.5 2.5 0 0 0 7 4.5v.5A2.5 2.5 0 0 0 4.5 7.5v1A2.5 2.5 0 0 0 2 11v2a2.5 2.5 0 0 0 2.5 2.5h.5A2.5 2.5 0 0 0 7.5 18v.5A2.5 2.5 0 0 0 10 21h.5a2.5 2.5 0 0 0 2.5-2.5V4.5A2.5 2.5 0 0 0 10.5 2H9.5z' },
    { type: 'path', d: 'M14.5 2A2.5 2.5 0 0 1 17 4.5v.5a2.5 2.5 0 0 1 2.5 2.5v1a2.5 2.5 0 0 1 2.5 2.5v2a2.5 2.5 0 0 1-2.5 2.5h-.5a2.5 2.5 0 0 1-2.5 2.5v.5a2.5 2.5 0 0 1-2.5 2.5h-.5a2.5 2.5 0 0 1-2.5-2.5' }
  ],
  chat: [
    { type: 'path', d: 'M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z' }
  ],
  star: [
    { type: 'path', d: 'M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z' }
  ],
  clock: [
    { type: 'circle', cx: '12', cy: '12', r: '9' },
    { type: 'polyline', points: '12 7 12 12 15 14' }
  ],
  alert: [
    { type: 'path', d: 'M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z' },
    { type: 'line', x1: '12', y1: '9', x2: '12', y2: '13' },
    { type: 'line', x1: '12', y1: '17', x2: '12.01', y2: '17' }
  ],
  bell: [
    { type: 'path', d: 'M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9' },
    { type: 'path', d: 'M13.73 21a2 2 0 0 1-3.46 0' }
  ],
  tag: [
    { type: 'path', d: 'M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z' },
    { type: 'line', x1: '7', y1: '7', x2: '7.01', y2: '7' }
  ],
  settings: [
    { type: 'circle', cx: '12', cy: '12', r: '3' },
    { type: 'path', d: 'M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z' }
  ],
  user: [
    { type: 'circle', cx: '12', cy: '8', r: '4' },
    { type: 'path', d: 'M4 21v-1a7 7 0 0 1 14 0v1' }
  ],
  lock: [
    { type: 'rect', x: '5', y: '11', w: '14', h: '9', rx: '2' },
    { type: 'path', d: 'M8 11V8a4 4 0 0 1 8 0v3' }
  ],
  logout: [
    { type: 'path', d: 'M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4' },
    { type: 'polyline', points: '16 17 21 12 16 7' },
    { type: 'line', x1: '21', y1: '12', x2: '9', y2: '12' }
  ],
  home: [
    { type: 'path', d: 'M3 10.5L12 3l9 7.5' },
    { type: 'path', d: 'M5 9.5V21h14V9.5' },
    { type: 'polyline', points: '9 21 9 14 15 14 15 21' }
  ],
  refresh: [
    { type: 'polyline', points: '23 4 23 10 17 10' },
    { type: 'path', d: 'M20.49 15a9 9 0 1 1-2.12-9.36L23 10' }
  ],
  fullscreen: [
    { type: 'path', d: 'M8 3H5a2 2 0 0 0-2 2v3' },
    { type: 'path', d: 'M21 8V5a2 2 0 0 0-2-2h-3' },
    { type: 'path', d: 'M3 16v3a2 2 0 0 0 2 2h3' },
    { type: 'path', d: 'M16 21h3a2 2 0 0 0 2-2v-3' }
  ],
  book: [
    { type: 'path', d: 'M4 19.5A2.5 2.5 0 0 1 6.5 17H20' },
    { type: 'path', d: 'M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z' }
  ],
  link: [
    { type: 'path', d: 'M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71' },
    { type: 'path', d: 'M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71' }
  ],
  copy: [
    { type: 'rect', x: '9', y: '9', w: '13', h: '13', rx: '2' },
    { type: 'path', d: 'M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1' }
  ],
  cut: [
    { type: 'circle', cx: '6', cy: '6', r: '3' },
    { type: 'circle', cx: '6', cy: '18', r: '3' },
    { type: 'line', x1: '8.12', y1: '8.12', x2: '20', y2: '20' },
    { type: 'line', x1: '8.12', y1: '15.88', x2: '20', y2: '4' }
  ],
  clipboard: [
    { type: 'rect', x: '8', y: '2', w: '8', h: '4', rx: '1' },
    { type: 'path', d: 'M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2' },
    { type: 'polyline', points: '9 12 11 14 15 10' }
  ],
  eye: [
    { type: 'path', d: 'M1 12s4-7 11-7 11 7 11 7-4 7-11 7-11-7-11-7z' },
    { type: 'circle', cx: '12', cy: '12', r: '3' }
  ],
  share: [
    { type: 'circle', cx: '18', cy: '5', r: '3' },
    { type: 'circle', cx: '6', cy: '12', r: '3' },
    { type: 'circle', cx: '18', cy: '19', r: '3' },
    { type: 'line', x1: '8.59', y1: '13.51', x2: '15.42', y2: '17.49' },
    { type: 'line', x1: '15.41', y1: '6.51', x2: '8.59', y2: '10.49' }
  ],
  check: [{ type: 'polyline', points: '20 6 9 17 4 12' }],
  info: [
    { type: 'circle', cx: '12', cy: '12', r: '9' },
    { type: 'line', x1: '12', y1: '11', x2: '12', y2: '16' },
    { type: 'line', x1: '12', y1: '8', x2: '12.01', y2: '8' }
  ],
  database: [
    { type: 'path', d: 'M3 5c0 1.66 4 3 9 3s9-1.34 9-3-4-3-9-3-9 1.34-9 3z' },
    { type: 'path', d: 'M3 12c0 1.66 4 3 9 3s9-1.34 9-3' },
    { type: 'path', d: 'M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5' }
  ],
  filter: [
    { type: 'polygon', points: '22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3' }
  ],
  heart: [
    { type: 'path', d: 'M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z' }
  ],
  menu: [
    { type: 'line', x1: '3', y1: '6', x2: '21', y2: '6' },
    { type: 'line', x1: '3', y1: '12', x2: '21', y2: '12' },
    { type: 'line', x1: '3', y1: '18', x2: '21', y2: '18' }
  ],
  // 新增：侧栏优化补齐（feather 风格，仅用受支持的 path/circle）
  layers: [
    { type: 'path', d: 'M12 2 2 7l10 5 10-5-10-5z' },
    { type: 'path', d: 'M2 17l10 5 10-5' },
    { type: 'path', d: 'M2 12l10 5 10-5' }
  ],
  globe: [
    { type: 'circle', cx: '12', cy: '12', r: '10' },
    { type: 'path', d: 'M2 12h20' },
    { type: 'path', d: 'M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z' }
  ],
  cart: [
    { type: 'path', d: 'M3 4h2l2.4 12.3a1 1 0 0 0 1 .8h8.7a1 1 0 0 0 1-.8L20 8H6' },
    { type: 'circle', cx: '9', cy: '20', r: '1.3' },
    { type: 'circle', cx: '17', cy: '20', r: '1.3' }
  ]
}

const paths = computed(() => ICONS[props.name] || ICONS.file)
</script>

<style scoped>
.aik-icon {
  display: inline-block;
  vertical-align: -0.15em;
  flex-shrink: 0;
}
</style>
