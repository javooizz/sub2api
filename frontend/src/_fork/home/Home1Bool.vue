<script setup lang="ts">
import { onMounted, onBeforeUnmount } from 'vue'
import Hero from './sections/Hero.vue'
import TopNav from './sections/TopNav.vue'
import './styles/home1bool.css'

onMounted(() => {
  document.body.classList.add('home1bool-page')
})

onBeforeUnmount(() => {
  document.body.classList.remove('home1bool-page')
})
</script>

<template>
  <div class="home1bool-root">
    <TopNav />
    <Hero />
  </div>
</template>
