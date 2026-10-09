<template>
  <textarea
    ref="ta"
    :value="modelValue"
    class="aik-plain-editor"
    :placeholder="placeholder"
    :style="{ height: typeof height === 'number' ? height + 'px' : height }"
    spellcheck="false"
    @input="$emit('update:modelValue', $event.target.value)"
    @paste="onPaste"
    @drop.prevent="onDrop"
    @dragover.prevent
    @keydown.ctrl.s.prevent="$emit('save')"
    @keydown.meta.s.prevent="$emit('save')"
  ></textarea>
</template>

<script>
/**
 * 极简 Markdown 编辑器（编辑器本体轨 plain）：裸 textarea，所见即源码。
 * 从 WriteView 抽出（2026-10-09 编辑器插件化），与 VditorEditor 同协议：
 * - props: modelValue / placeholder / height
 * - emits: update:modelValue / save / paste(原生事件) / drop(原生事件)
 *   （粘贴/拖入图片的检测与上传留在宿主 WriteView 处理——它持有上传与封面逻辑）
 * - expose: focus / insertAtCursor / getValue / setValue
 * 图片粘贴、分屏预览等宿主能力通过原生事件与 v-model 保持解耦。
 */
export default {
  name: 'PlainEditor',
  props: {
    modelValue: { type: String, default: '' },
    placeholder: { type: String, default: '' },
    height: { type: [Number, String], default: '100%' },
  },
  emits: ['update:modelValue', 'save', 'paste', 'drop'],
  data() {
    return { ta: null }
  },
  mounted() {
    this.ta = this.$refs.ta
  },
  methods: {
    focus() {
      this.ta?.focus()
    },
    getValue() {
      return this.modelValue
    },
    setValue(md) {
      this.$emit('update:modelValue', md || '')
    },
    insertAtCursor(text) {
      const ta = this.ta
      if (!ta) {
        this.$emit('update:modelValue', (this.modelValue || '') + text)
        return
      }
      const s = ta.selectionStart ?? (this.modelValue || '').length
      const e = ta.selectionEnd ?? s
      const next = (this.modelValue || '').slice(0, s) + text + (this.modelValue || '').slice(e)
      this.$emit('update:modelValue', next)
      this.$nextTick(() => {
        ta.selectionStart = ta.selectionEnd = s + text.length
        ta.focus()
      })
    },
    onPaste(e) {
      this.$emit('paste', e)
    },
    onDrop(e) {
      this.$emit('drop', e)
    },
  },
}
</script>

<style scoped>
.aik-plain-editor {
  width: 100%;
  border: none;
  outline: none;
  resize: none;
  background: transparent;
  color: var(--text-1, inherit);
  font: 14px/1.75 var(--font-mono, ui-monospace, Menlo, Consolas, monospace);
  padding: 14px 16px;
  box-sizing: border-box;
  min-height: 240px;
}
</style>
