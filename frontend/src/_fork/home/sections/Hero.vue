<script setup lang="ts">
import { computed } from 'vue'
import { useAuthStore } from '@/stores'
import { FORK_APP_CONFIG } from '../config'

const authStore = useAuthStore()

const primaryHref = computed(() => {
  if (!authStore.isAuthenticated) return '/login'
  return authStore.isAdmin ? '/admin/dashboard' : '/dashboard'
})
</script>

<template>
  <section id="hero" class="hero">
    <div class="hero-deco" aria-hidden="true">
      <div class="hero-grid" />
      <div class="hero-orb hero-orb--indigo" />
      <div class="hero-orb hero-orb--blue" />
    </div>

    <div class="hero-inner">
      <div class="hero-pill">
        <span class="hero-pill-dot" />
        {{ FORK_APP_CONFIG.pill }}
      </div>

      <h1 class="hero-h1">
        <span class="hero-h1-line">{{ FORK_APP_CONFIG.headline[0] }}</span>
        <span class="hero-h1-line">
          {{ FORK_APP_CONFIG.headline[1] }}<span class="hero-h1-accent">{{ FORK_APP_CONFIG.headlineAccent }}</span>
        </span>
      </h1>

      <p class="hero-sub">{{ FORK_APP_CONFIG.subtitle }}</p>

      <div class="hero-actions">
        <router-link :to="primaryHref" class="hero-btn-primary">
          立即开始
          <svg
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M5 12h14M12 5l7 7-7 7" />
          </svg>
        </router-link>
      </div>

      <div class="hero-logos">
        <span>{{ FORK_APP_CONFIG.logos[0] }}</span>
        <span class="hero-logos-sep" />
        <span>{{ FORK_APP_CONFIG.logos[1] }}</span>
        <span class="hero-logos-sep" />
        <span>{{ FORK_APP_CONFIG.logos[2] }}</span>
      </div>
    </div>
  </section>
</template>
