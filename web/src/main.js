import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useThemeStore } from './stores/theme'
import './styles/tokens.css'
import './styles/main.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)

// 主题先行应用（避免闪烁）
useThemeStore().apply()

app.mount('#app')
