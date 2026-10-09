/**
 * 主题安装辅助 —— 管理端「站点主题」下拉选中未安装主题时，保存/建站前自动安装。
 *
 * 背景：「只内置 aiklog，其余主题进应用中心」改版后，站点主题必须已安装
 * （blog_plugins kind=theme）才能通过后端 blog.theme 白名单校验。
 * 本 helper 让管理端体验保持顺畅：选中未安装主题 → 保存时静默调
 * POST /admin/apps/market/install {id, kind:'theme'}（内置源码主题免 zip，登记即生效）。
 */
import { requestJSON } from '@/api'
import { isThemeInstalled, applyInstalledThemes, installedThemeIds } from '@/themes'

/** 确保主题已安装；未安装则自动调应用中心安装接口。返回是否可用（安装成功或本就已装）。 */
export async function ensureThemeInstalled(id) {
  if (!id || id === 'default' || isThemeInstalled(id)) return true
  try {
    await requestJSON('/admin/apps/market/install', {
      method: 'POST',
      body: JSON.stringify({ id, kind: 'theme' }),
    })
  } catch (e) {
    console.warn('[themeInstall] 自动安装主题失败:', id, e?.message || e)
    return false
  }
  // 本地同步追加，免重拉列表（后端登记已生效）
  if (Array.isArray(installedThemeIds.value) && !installedThemeIds.value.includes(id)) {
    applyInstalledThemes([...installedThemeIds.value, id])
  }
  return true
}
