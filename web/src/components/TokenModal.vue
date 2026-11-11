<template>
  <div v-if="isOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
    <div class="bg-github-subtle border border-github-border rounded-lg max-w-lg w-full p-6 shadow-2xl">
      <div class="flex items-center justify-between border-b border-github-border pb-3 mb-4">
        <h3 class="text-base font-bold text-white flex items-center space-x-2">
          <Key class="w-5 h-5 text-github-blue" />
          <span>Generate Personal Access Token</span>
        </h3>
        <button @click="close" class="text-github-muted hover:text-white transition-colors">
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Generated Token View -->
      <div v-if="generatedToken" class="space-y-4">
        <div class="p-3 bg-emerald-950/40 border border-emerald-700/60 rounded-md text-xs text-emerald-300">
          <strong>Token generated successfully!</strong> Make sure to copy your personal access token now. You won't be able to see it again!
        </div>

        <div>
          <label class="block text-xs font-semibold text-github-muted uppercase mb-1">Your Token</label>
          <div class="flex items-center space-x-2">
            <input
              type="text"
              readonly
              :value="generatedToken.token"
              class="w-full bg-github-darker border border-github-border px-3 py-2 rounded text-xs font-mono text-emerald-400 select-all focus:outline-none"
            />
            <button
              @click="copyToken"
              class="px-3 py-2 bg-github-green hover:bg-github-greenHover text-white rounded text-xs font-semibold flex items-center space-x-1 flex-shrink-0 transition-colors"
            >
              <Check v-if="copied" class="w-4 h-4" />
              <Copy v-else class="w-4 h-4" />
              <span>{{ copied ? 'Copied' : 'Copy' }}</span>
            </button>
          </div>
        </div>

        <div class="text-xs text-github-muted space-y-1">
          <div>Scope: <span class="font-mono text-github-text font-semibold">{{ generatedToken.scope.toUpperCase() }}</span></div>
          <div v-if="generatedToken.expires_at">Expires: <span class="text-github-text">{{ new Date(generatedToken.expires_at).toLocaleDateString() }}</span></div>
          <div v-else>Expires: <span class="text-github-text font-semibold">Never</span></div>
        </div>

        <div class="pt-2 flex justify-end">
          <button
            @click="close"
            class="px-4 py-2 bg-github-dark hover:bg-github-border border border-github-border rounded text-xs font-semibold text-white transition-colors"
          >
            Done
          </button>
        </div>
      </div>

      <!-- Form View -->
      <form v-else @submit.prevent="generate" class="space-y-4">
        <div>
          <label class="block text-xs font-semibold text-github-muted uppercase mb-1.5">Repository</label>
          <div class="text-xs font-mono text-github-text bg-github-darker px-3 py-2 border border-github-border rounded">
            {{ repoPath }}
          </div>
        </div>

        <div>
          <label class="block text-xs font-semibold text-github-muted uppercase mb-1.5">Expiration</label>
          <select
            v-model="days"
            class="w-full bg-github-dark border border-github-border px-3 py-2 rounded text-xs text-github-text focus:outline-none focus:border-github-blue"
          >
            <option value="30">30 days</option>
            <option value="90">90 days</option>
            <option value="365">1 year</option>
            <option value="0">No expiration</option>
          </select>
        </div>

        <div>
          <label class="block text-xs font-semibold text-github-muted uppercase mb-1.5">Scope</label>
          <div class="space-y-2 text-xs">
            <label class="flex items-start space-x-2.5 p-2.5 rounded border border-github-border bg-github-dark/50 cursor-pointer hover:bg-github-dark">
              <input type="radio" value="ro" v-model="scope" class="mt-0.5 text-github-blue focus:ring-0" />
              <div>
                <div class="font-semibold text-white">Read Only (RO)</div>
                <div class="text-github-muted text-[11px]">Can clone, pull, and read repository content.</div>
              </div>
            </label>
            <label
              class="flex items-start space-x-2.5 p-2.5 rounded border border-github-border bg-github-dark/50 cursor-pointer hover:bg-github-dark"
              :class="{ 'opacity-50 cursor-not-allowed': !canWrite }"
            >
              <input
                type="radio"
                value="rw"
                v-model="scope"
                :disabled="!canWrite"
                class="mt-0.5 text-github-blue focus:ring-0"
              />
              <div>
                <div class="font-semibold text-white flex items-center space-x-1.5">
                  <span>Read / Write (RW)</span>
                  <span v-if="!canWrite" class="text-[10px] bg-red-900/50 text-red-300 px-1 rounded">Requires Write permission</span>
                </div>
                <div class="text-github-muted text-[11px]">Can clone, pull, push commits and manage LFS objects.</div>
              </div>
            </label>
          </div>
        </div>

        <div v-if="errorMsg" class="p-2.5 bg-red-950/40 border border-red-800 rounded text-xs text-red-300">
          {{ errorMsg }}
        </div>

        <div class="flex justify-end space-x-2 pt-2 border-t border-github-border">
          <button
            type="button"
            @click="close"
            class="px-4 py-2 bg-github-dark hover:bg-github-border border border-github-border rounded text-xs font-semibold text-github-muted hover:text-white transition-colors"
          >
            Cancel
          </button>
          <button
            type="submit"
            :disabled="loading"
            class="px-4 py-2 bg-github-green hover:bg-github-greenHover disabled:opacity-50 text-white rounded text-xs font-semibold transition-colors flex items-center space-x-1"
          >
            <span v-if="loading">Generating...</span>
            <span v-else>Generate Token</span>
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Key, X, Copy, Check } from '@lucide/vue'
import { api } from '../api'
import type { TokenResponse } from '../types'

const props = defineProps<{
  isOpen: boolean
  repoPath: string
  canWrite: boolean
}>()

const emit = defineEmits(['close'])

const days = ref('30')
const scope = ref(props.canWrite ? 'rw' : 'ro')
const loading = ref(false)
const errorMsg = ref('')
const generatedToken = ref<TokenResponse | null>(null)
const copied = ref(false)

async function generate() {
  loading.value = true
  errorMsg.value = ''
  try {
    const res = await api.generateToken(props.repoPath, days.value, scope.value)
    generatedToken.value = res
  } catch (err: any) {
    errorMsg.value = err.message || 'Failed to generate token'
  } finally {
    loading.value = false
  }
}

async function copyToken() {
  if (!generatedToken.value) return
  await navigator.clipboard.writeText(generatedToken.value.token)
  copied.value = true
  setTimeout(() => {
    copied.value = false
  }, 2000)
}

function close() {
  generatedToken.value = null
  errorMsg.value = ''
  emit('close')
}
</script>
