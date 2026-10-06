// 安全剪贴板工具：优先 Clipboard API；失败时返回 false（由调用方展示链接让用户手动复制），
// 不使用 document.execCommand('copy')——它在部分 Windows 内核/WebView 中会触发系统级弹窗。
export async function tryCopy(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch (_) {
    // 权限拒绝等，走手动复制
  }
  return false
}
