<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteLogo = computed(
  () => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '/logo.png',
)
const siteName = computed(
  () => appStore.cachedPublicSettings?.site_name || appStore.siteName || '1Bool API',
)

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const userInitial = computed(() => {
  const email = authStore.user?.email
  return email ? email.charAt(0).toUpperCase() : 'U'
})

const isDark = ref(
  typeof document !== 'undefined' && document.documentElement.classList.contains('dark'),
)

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

onMounted(() => {
  isDark.value = document.documentElement.classList.contains('dark')
})
</script>

<template>
  <header class="fork-topnav">
    <div class="fork-topnav-inner">
      <router-link to="/" class="fork-topnav-brand" :title="siteName">
        <img :src="siteLogo" :alt="siteName" />
      </router-link>

      <div class="fork-topnav-actions">
        <LocaleSwitcher />

        <button
          type="button"
          class="fork-topnav-icon-btn"
          :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          @click="toggleTheme"
        >
          <Icon v-if="isDark" name="sun" size="md" />
          <Icon v-else name="moon" size="md" />
        </button>

        <router-link v-if="isAuthenticated" :to="dashboardPath" class="fork-topnav-console">
          <span class="fork-topnav-console-avatar">{{ userInitial }}</span>
          <span class="fork-topnav-console-text">{{ t('home.dashboard') }}</span>
          <svg
            width="12"
            height="12"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M7 17L17 7M9 7h8v8" />
          </svg>
        </router-link>
        <template v-else>
          <router-link to="/login" class="fork-topnav-login">{{ t('home.login') }}</router-link>
          <router-link to="/register" class="fork-topnav-register">{{
            t('auth.createAccount')
          }}</router-link>
        </template>
      </div>
    </div>
  </header>
</template>

<style scoped>
/* TopNav 使用 home1bool.css 中按主题切换的 --nav-* 变量。
   注意：本组件位于 .home1bool-root 内，变量会自动级联生效。 */
.fork-topnav {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 50;
  background: var(--nav-bg);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--nav-border);
  transition: background 0.3s ease, border-color 0.3s ease;
}

.fork-topnav-inner {
  max-width: 1200px;
  margin: 0 auto;
  padding: 12px 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.fork-topnav-brand {
  display: inline-flex;
  align-items: center;
  text-decoration: none;
}

.fork-topnav-brand img {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  object-fit: cover;
  box-shadow: 0 0 0 1px var(--nav-border);
}

.fork-topnav-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--nav-text);
}

.fork-topnav-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: transparent;
  border: 0;
  color: var(--nav-text-muted);
  cursor: pointer;
  transition: background 0.2s ease, color 0.2s ease;
}
.fork-topnav-icon-btn:hover {
  background: var(--nav-icon-hover-bg);
  color: var(--nav-text-strong);
}
.fork-topnav-icon-btn:focus-visible {
  outline: 2px solid var(--hero-h1-accent-b);
  outline-offset: 2px;
}

.fork-topnav-login {
  padding: 7px 14px;
  font-size: 13px;
  font-weight: 500;
  color: var(--nav-text);
  text-decoration: none;
  border-radius: 999px;
  transition: color 0.2s ease;
}
.fork-topnav-login:hover {
  color: var(--nav-text-strong);
}

.fork-topnav-register {
  display: inline-flex;
  align-items: center;
  padding: 7px 16px;
  font-size: 13px;
  font-weight: 600;
  color: var(--nav-cta-text);
  background: var(--nav-cta-bg);
  border-radius: 999px;
  text-decoration: none;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.fork-topnav-register:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 16px var(--nav-border);
}

.fork-topnav-console {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 12px 4px 4px;
  border-radius: 999px;
  background: var(--nav-console-bg);
  border: 1px solid var(--nav-console-border);
  text-decoration: none;
  color: var(--nav-text);
  font-size: 13px;
  font-weight: 500;
  transition: background 0.2s ease, border-color 0.2s ease;
}
.fork-topnav-console:hover {
  background: var(--nav-icon-hover-bg);
  border-color: var(--hero-border-strong);
}

.fork-topnav-console-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--hero-h1-accent-a) 0%, var(--hero-h1-accent-c) 100%);
  color: #ffffff;
  font-size: 11px;
  font-weight: 700;
}

.fork-topnav-console svg {
  opacity: 0.7;
}

@media (max-width: 640px) {
  .fork-topnav-inner {
    padding: 10px 16px;
  }
  .fork-topnav-actions {
    gap: 6px;
  }
  .fork-topnav-console-text,
  .fork-topnav-register {
    display: none;
  }
}
</style>
