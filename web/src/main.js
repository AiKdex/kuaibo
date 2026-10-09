import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useThemeStore } from './stores/theme'
import i18n from './i18n'
import './styles/tokens.css'
import './styles/main.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(i18n)

// 主题先行应用（避免闪烁）
useThemeStore().apply()

// 启动即实探登录态：校准 cookie 会话（HttpOnly）下的真实登录状态，
// 让公开页导航（登录/注册 ↔ 控制台）、评论区等全部跟随，避免刷新后被误判为游客。
import { refreshAuth } from './api'
refreshAuth()

app.mount('#app')
