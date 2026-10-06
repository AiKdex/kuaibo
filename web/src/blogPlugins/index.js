// blogPlugins/index.js 博客功能插件注册表（协议前端侧）
//
// 插件 = manifest（后端登记：挂载点/权限/启用状态）+ 前端组件（本注册表按 id 注册）。
// 内置插件随系统注册；第三方插件按 docs/博客插件协议.md 开发：后端 POST /api/v1/blog/plugins
// 登记 manifest，前端 import 插件包后调用 registerBlogPlugin 注册组件，启用即在对应挂载点渲染。
import { reactive, ref } from 'vue'
import { publicPlugins } from '@/api'

// 前端组件注册表：{ id, mountPoints: [], component }
const registry = reactive([])

// 后端启用状态（启动时拉取；失败时内置插件默认启用，保证离线可用）
const enabledIds = ref(new Set())
let loaded = false

export function registerBlogPlugin(def) {
  const i = registry.findIndex((p) => p.id === def.id)
  if (i >= 0) registry.splice(i, 1, def)
  else registry.push(def)
}

export async function loadEnabledPlugins() {
  if (loaded) return
  try {
    const d = await publicPlugins()
    enabledIds.value = new Set((d.items || []).filter((p) => p.enabled).map((p) => p.id))
  } catch (_) {
    // 网络/服务不可达：内置插件全部启用（离线预览可用）
    enabledIds.value = new Set(registry.map((p) => p.id))
  }
  loaded = true
}

// 某挂载点下已启用的插件（按注册顺序）
export function pluginsFor(mount) {
  return registry.filter((p) => p.mountPoints.includes(mount) && enabledIds.value.has(p.id))
}
