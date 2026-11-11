import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api'
import type { UserInfo } from '../types'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<string | null>(null)
  const isAdmin = ref(false)
  const baseUrl = ref('')
  const loading = ref(true)

  async function fetchUser() {
    loading.value = true
    try {
      const data: UserInfo = await api.getAuthMe()
      user.value = data.user || null
      isAdmin.value = data.is_admin
      baseUrl.value = data.base_url
    } catch {
      user.value = null
      isAdmin.value = false
    } finally {
      loading.value = false
    }
  }

  function login(redirectPath: string = window.location.pathname) {
    window.location.href = `/login/oauth/authorize?redirect_uri=${encodeURIComponent(redirectPath)}`
  }

  function logout() {
    window.location.href = '/login/oauth/logout'
  }

  return {
    user,
    isAdmin,
    baseUrl,
    loading,
    fetchUser,
    login,
    logout,
  }
})
