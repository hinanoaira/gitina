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
          <strong>Token generated successfully!</strong> Make sure to copy both your Username (ID) and Password (Token) now. You won't be able to see the password again!
        </div>

        <div class="space-y-3 bg-github-darker p-3.5 rounded-md border border-github-border">
          <!-- Basic Auth Username (ID) -->
          <div>
            <div class="flex items-center justify-between mb-1">
              <label class="text-[11px] font-bold text-github-blue uppercase tracking-wider">Basic Auth Username (ID)</label>
              <span class="text-[10px] text-github-muted">Pass as username in Git</span>
            </div>
            <div class="flex items-center space-x-2">
              <input
                type="text"
                readonly
                :value="generatedToken.id"
                class="w-full bg-github-dark border border-github-border px-3 py-1.5 rounded text-xs font-mono text-white select-all focus:outline-none"
              />
              <button
                @click="copyID"
                class="px-2.5 py-1.5 bg-github-subtle hover:bg-github-border border border-github-border text-white rounded text-xs font-semibold flex items-center space-x-1 flex-shrink-0 transition-colors"
              >
                <Check v-if="copiedID" class="w-3.5 h-3.5 text-emerald-400" />
                <Copy v-else class="w-3.5 h-3.5" />
                <span>{{ copiedID ? 'Copied' : 'Copy' }}</span>
              </button>
            </div>
          </div>

          <!-- Basic Auth Password (Secret Token) -->
          <div>
            <div class="flex items-center justify-between mb-1">
              <label class="text-[11px] font-bold text-emerald-400 uppercase tracking-wider">Basic Auth Password (Token)</label>
              <span class="text-[10px] text-github-muted">Pass as password in Git</span>
            </div>
            <div class="flex items-center space-x-2">
              <input
                type="text"
                readonly
                :value="generatedToken.token"
                class="w-full bg-github-dark border border-github-border px-3 py-1.5 rounded text-xs font-mono text-emerald-400 select-all focus:outline-none"
              />
              <button
                @click="copyToken"
                class="px-2.5 py-1.5 bg-github-green hover:bg-github-greenHover text-white rounded text-xs font-semibold flex items-center space-x-1 flex-shrink-0 transition-colors"
              >
                <Check v-if="copiedToken" class="w-3.5 h-3.5" />
                <Copy v-else class="w-3.5 h-3.5" />
                <span>{{ copiedToken ? 'Copied' : 'Copy' }}</span>
              </button>
            </div>
          </div>
        </div>

        <div class="text-xs text-github-muted space-y-1">
          <div>Name: <span class="text-white font-medium">{{ generatedToken.name }}</span></div>
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
          <label class="block text-xs font-semibold text-github-muted uppercase mb-1.5">Token Name / Purpose</label>
          <input
            type="text"
            v-model="tokenName"
            required
            placeholder="e.g. My Laptop, CI Runner, Backup script"
            class="w-full bg-github-dark border border-github-border px-3 py-2 rounded text-xs text-github-text placeholder-github-muted focus:outline-none focus:border-github-blue"
          />
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
            :disabled="loading || !tokenName.trim()"
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

const emit = defineEmits(['close', 'created'])

const tokenName = ref('')
const days = ref('30')
const scope = ref(props.canWrite ? 'rw' : 'ro')
const loading = ref(false)
const errorMsg = ref('')
const generatedToken = ref<TokenResponse | null>(null)
const copiedID = ref(false)
const copiedToken = ref(false)

async function generate() {
  if (!tokenName.value.trim()) return
  loading.value = true
  errorMsg.value = ''
  try {
    const res = await api.generateToken(props.repoPath, tokenName.value.trim(), days.value, scope.value)
    generatedToken.value = res
    emit('created')
  } catch (err: any) {
    errorMsg.value = err.message || 'Failed to generate token'
  } finally {
    loading.value = false
  }
}

async function copyID() {
  if (!generatedToken.value) return
  await navigator.clipboard.writeText(generatedToken.value.id)
  copiedID.value = true
  setTimeout(() => {
    copiedID.value = false
  }, 2000)
}

async function copyToken() {
  if (!generatedToken.value) return
  await navigator.clipboard.writeText(generatedToken.value.token)
  copiedToken.value = true
  setTimeout(() => {
    copiedToken.value = false
  }, 2000)
}

function close() {
  generatedToken.value = null
  tokenName.value = ''
  errorMsg.value = ''
  emit('close')
}
</script>
