/**
 * 主题集中注册入口 —— 公开博客的所有 SPA 视图共用（列表页 BlogView / 文章页 BlogPostView）。
 *
 * 「只内置一个主题」改版（2026-10-09）：
 *  - 常驻内置：default（宿主默认列表，index.js 注册）+ aiklog（爱库录样板房门面）—— 静态 import；
 *  - 其余 17 个主题改为懒加载：catalog.js 持有元数据（manifest.js，纯元数据常驻）与
 *    动态 import() 加载器（THEME_LOADERS，组件拆独立 chunk 按需请求）；
 *    「安装」（应用中心 → blog_plugins kind=theme）后由 BlogView/BlogPostView/ThemeSwitch
 *    调 ensureTheme(id) 按需拉起；未安装主题不注册、不进切换器、不请求 chunk。
 *  - 好处：首屏包不再含 17 个主题组件；主题是否对访客可见完全由后端已安装列表驱动。
 *
 * 为什么集中：加载器与元数据只在 catalog.js 一处维护，避免多个视图各写一份清单导致遗漏
 * （遗漏的表现是：主题在列表页可选、文章页却掉回默认皮）。
 *
 * 爱库录精简发行：不注册求智（采集型）主题。求智 manifest 声明 dataSources:['collect'] /
 * dataScope:'library'，读的是采集库（非「已分享」文件集合），且注明「公开部署时不得对外
 * 暴露此主题」；ThemeSwitch 直接 v-for 渲染 listThemes()（= 整个注册表，无 dataScope 门控），
 * 一旦注册即对访客可见 → 故取消注册。如需单机自用（采集看板），在 catalog.js 的
 * MANIFESTS/THEME_LOADERS 里把 qiuzhi 加回即可，源码保留在 themes/qiuzhi/。
 */
import '@/themes/aiklog' // 常驻内置：爱库录主题（AiKlog 样板房门面）

// 其余主题不再静态注册 —— 见 catalog.js 的 THEME_LOADERS + index.js 的 ensureTheme()。
// 视图层（BlogView / BlogPostView / ThemeSwitch / 管理端）负责：
//   1. 拉 GET /api/v1/public/blog/themes → applyInstalledThemes(ids)
//   2. 对当前生效主题 ensureTheme(id) 后再渲染（加载期间回落 default）
