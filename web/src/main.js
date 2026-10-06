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

app.mount('#app')
