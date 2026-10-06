import { reactive } from 'vue'
import { storeCartGet } from '@/api'

// 商城购物车共享状态：layout 与各视图共用，避免重复请求与计数不同步。
export const cartState = reactive({ count: 0, loaded: false })

export async function refreshCart() {
  try {
    const c = await storeCartGet()
    const lines = (c && c.lines) || []
    cartState.count = lines.reduce((s, l) => s + (l.qty || 0), 0)
  } catch (_) {
    cartState.count = 0
  }
  cartState.loaded = true
}
