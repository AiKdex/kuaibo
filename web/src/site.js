// 全局站点名：所有公开页调用 publicSite() 成功后写入，
// 供路由守卫拼接浏览器标签标题、SEO 回退使用。
// 避免各主题/路由把品牌名「爱库录」写死，使「站点设置→博客名称」真正对访客生效。
import { ref } from 'vue'

// 全局响应式站点名；缺省为空串，调用方按「空串 → 爱库录」回退。
export const siteName = ref('')

// 由 publicSite() 的 API 响应写入；仅在有有效值时更新，避免把空值/无效值写回。
export function applySiteName(v) {
  if (v && String(v).trim()) siteName.value = String(v).trim()
}
