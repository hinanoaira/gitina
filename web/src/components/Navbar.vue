<template>
  <header class="bg-github-subtle border-b border-github-border sticky top-0 z-50">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between">
      <!-- Left: Logo & Title -->
      <div class="flex items-center space-x-3">
        <router-link to="/" class="flex items-center space-x-2.5 text-white hover:text-github-blue transition-colors">
          <svg class="w-8 h-8 text-github-blue" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="6" y1="3" x2="6" y2="15"></line>
            <circle cx="18" cy="6" r="3"></circle>
            <circle cx="6" cy="18" r="3"></circle>
            <path d="M18 9a9 9 0 0 1-9 9"></path>
          </svg>
          <span class="font-bold text-lg tracking-tight">Gitina</span>
        </router-link>
        <span class="text-xs text-github-muted bg-github-dark border border-github-border px-2 py-0.5 rounded-full">v0.1.0</span>
      </div>

      <!-- Right: User or Login -->
      <div class="flex items-center space-x-4">
        <template v-if="auth.loading">
          <div class="w-8 h-8 rounded-full bg-github-border animate-pulse"></div>
        </template>
        <template v-else-if="auth.user">
          <div class="flex items-center space-x-3">
            <span class="text-sm font-medium text-github-text flex items-center space-x-1.5">
              <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
              <span>{{ auth.user }}</span>
              <span v-if="auth.isAdmin" class="text-xs bg-purple-900/60 text-purple-300 border border-purple-700 px-1.5 py-0.2 rounded font-mono">admin</span>
            </span>
            <button
              @click="auth.logout"
              class="text-xs text-github-muted hover:text-red-400 transition-colors px-2 py-1 border border-github-border rounded hover:border-red-900"
            >
              Sign out
            </button>
          </div>
        </template>
        <template v-else>
          <button
            @click="auth.login()"
            class="text-xs font-semibold bg-github-green hover:bg-github-greenHover text-white px-3.5 py-1.5 rounded-md transition-colors shadow-sm"
          >
            Sign in with GitHub
          </button>
        </template>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
</script>
