<template>
  <div class="min-h-screen flex flex-col bg-github-dark text-github-text">
    <Navbar />
    <main class="flex-grow">
      <router-view />
    </main>
    <footer class="border-t border-github-border/40 py-6 text-center text-xs text-github-muted">
      <div class="flex items-center justify-center space-x-2">
        <span class="font-semibold text-white">Gitina</span>
        <span>•</span>
        <span>Self-hosted Git & LFS server</span>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import Navbar from './components/Navbar.vue'
import { useAuthStore } from './stores/auth'

const auth = useAuthStore()

onMounted(() => {
  auth.fetchUser()
})
</script>
