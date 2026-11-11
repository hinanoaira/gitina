<template>
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
    <!-- Top Header -->
    <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between pb-6 border-b border-github-border gap-4">
      <div>
        <h1 class="text-2xl font-bold text-white tracking-tight flex items-center space-x-2">
          <span>Repositories</span>
        </h1>
        <p class="text-xs text-github-muted mt-1">Lightweight self-hosted Git repositories with automated S3 tiered storage.</p>
      </div>

      <!-- Search Filter -->
      <div class="w-full sm:w-72">
        <input
          type="text"
          v-model="searchQuery"
          placeholder="Filter repositories..."
          class="w-full bg-github-dark border border-github-border px-3 py-1.5 rounded-md text-xs text-github-text placeholder-github-muted focus:outline-none focus:border-github-blue transition-colors"
        />
      </div>
    </div>

    <!-- Repository Cards Grid -->
    <div v-if="loading" class="py-16 text-center">
      <div class="inline-block w-8 h-8 border-2 border-github-border border-t-github-blue rounded-full animate-spin"></div>
      <p class="text-xs text-github-muted mt-3">Loading repositories...</p>
    </div>

    <div v-else-if="error" class="py-12 text-center text-red-400 text-sm">
      {{ error }}
    </div>

    <div v-else-if="filteredRepos.length === 0" class="py-16 text-center border border-dashed border-github-border rounded-lg mt-6">
      <FolderGit2 class="w-10 h-10 text-github-muted mx-auto mb-2 opacity-50" />
      <h3 class="text-sm font-semibold text-white">No repositories found</h3>
      <p class="text-xs text-github-muted mt-1">Push your first repository using git clone / git push!</p>
    </div>

    <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-6">
      <div
        v-for="repo in filteredRepos"
        :key="repo.path"
        @click="goToRepo(repo.path)"
        class="border border-github-border hover:border-github-border/80 bg-github-subtle/40 hover:bg-github-subtle/80 p-5 rounded-lg cursor-pointer transition-all flex flex-col justify-between group shadow-sm"
      >
        <div>
          <!-- Header -->
          <div class="flex items-start justify-between gap-2">
            <div class="flex items-center space-x-2 truncate">
              <FolderGit2 class="w-5 h-5 text-github-blue flex-shrink-0" />
              <span class="text-sm font-semibold text-github-blue group-hover:underline truncate font-mono">
                {{ repo.path }}
              </span>
            </div>
            <span
              class="text-[11px] px-2 py-0.5 rounded-full border font-medium flex-shrink-0"
              :class="repo.is_public ? 'bg-emerald-950/40 text-emerald-400 border-emerald-800/60' : 'bg-amber-950/40 text-amber-300 border-amber-800/60'"
            >
              {{ repo.is_public ? 'Public' : 'Private' }}
            </span>
          </div>

          <!-- Metadata -->
          <div class="mt-4 flex items-center space-x-4 text-xs text-github-muted">
            <span class="flex items-center space-x-1 font-mono">
              <HardDrive class="w-3.5 h-3.5" />
              <span>{{ formatSize(repo.disk_size) }}</span>
            </span>
            <span class="flex items-center space-x-1">
              <Layers class="w-3.5 h-3.5" />
              <span class="uppercase text-[10px] bg-github-dark border border-github-border px-1.5 py-0.2 rounded">{{ repo.storage }}</span>
            </span>
            <span v-if="repo.is_cached" class="flex items-center space-x-1 text-emerald-400 text-[10px]">
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
              <span>Cached</span>
            </span>
          </div>
        </div>

        <!-- Footer -->
        <div class="mt-4 pt-3 border-t border-github-border/40 text-[11px] text-github-muted flex items-center justify-between">
          <span>Updated {{ formatDate(repo.updated_at) }}</span>
          <span class="text-github-blue group-hover:translate-x-0.5 transition-transform">View &rarr;</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { FolderGit2, HardDrive, Layers } from '@lucide/vue'
import { api } from '../api'
import type { RepoSummary } from '../types'

const router = useRouter()
const repos = ref<RepoSummary[]>([])
const loading = ref(true)
const error = ref('')
const searchQuery = ref('')

const filteredRepos = computed(() => {
  if (!searchQuery.value) return repos.value
  const q = searchQuery.value.toLowerCase()
  return repos.value.filter(r => r.path.toLowerCase().includes(q))
})

onMounted(async () => {
  try {
    const res = await api.getRepos()
    repos.value = res.repos || []
  } catch (err: any) {
    error.value = err.message || 'Failed to load repositories'
  } finally {
    loading.value = false
  }
})

function goToRepo(repoPath: string) {
  router.push(`/${repoPath}`)
}

function formatSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function formatDate(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}
</script>
