import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import i18n, { initI18n } from './i18n'
import { useAppStore } from '@/stores/app'
import './style.css'

function initThemeClass() {
  const savedTheme = localStorage.getItem('theme')
  const shouldUseDark =
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', shouldUseDark)
}

async function bootstrap() {
  // Apply theme class globally before app mount to keep all routes consistent.
  initThemeClass()

  const app = createApp(App)
  const pinia = createPinia()
  app.use(pinia)

  // Initialize settings from injected config BEFORE mounting (prevents flash)
  // This must happen after pinia is installed but before router and i18n
  const appStore = useAppStore()
  appStore.initFromInjectedConfig()

  // [fork] 注入的 window.__APP_CONFIG__ 是缓存进 index.html 的服务端快照，可能陈旧
  //（如某 opt-in 开关在 HTML 缓存之后才切换，如 model_plaza_enabled）。
  // initFromInjectedConfig 只消除首屏闪烁，并非权威值；这里用上游已支持的 force 参数
  // 回查一次实时 public-settings 纠偏，让陈旧 opt-in 开关自愈，而不是整会话隐藏其菜单。
  // fire-and-forget：不阻塞首屏；不改 fetchPublicSettings 函数本身（仅以 force=true 调用）。
  void appStore.fetchPublicSettings(true)

  // Set document title immediately after config is loaded
  if (appStore.siteName && appStore.siteName !== 'Sub2API') {
    document.title = `${appStore.siteName} - AI API Gateway`
  }

  await initI18n()

  app.use(router)
  app.use(i18n)

  // 等待路由器完成初始导航后再挂载，避免竞态条件导致的空白渲染
  await router.isReady()
  app.mount('#app')
}

bootstrap()
