<template>
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
    <!-- Top Header -->
    <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between pb-6 border-b border-github-border gap-4">
      <div>
        <h1 class="text-2xl font-bold text-white tracking-tight flex items-center space-x-2">
          <span>Repositories</span>
        </h1>
      </div>

      <!-- Search Filter -->
      <div class="w-full sm:w-72">
        <input
          type="text"
          v-model="searchQuery"
          placeholder="Filter in current folder..."
          class="w-full bg-github-dark border border-github-border px-3 py-1.5 rounded-md text-xs text-github-text placeholder-github-muted focus:outline-none focus:border-github-blue transition-colors"
        />
      </div>
    </div>

    <!-- Breadcrumb Navigation & Folder Admin -->
    <div class="flex items-center justify-between mt-4 py-2 border-b border-github-border/40 text-xs">
      <!-- Breadcrumbs -->
      <div class="flex items-center space-x-1.5 font-mono overflow-x-auto py-1">
        <button
          @click="goToFolder('')"
          class="flex items-center space-x-1 transition-colors"
          :class="!currentFolder ? 'font-bold text-white' : 'text-github-blue hover:underline'"
        >
          <Home class="w-3.5 h-3.5" />
          <span>Root</span>
        </button>
        <template v-for="(b, idx) in breadcrumbs" :key="b.path">
          <span class="text-github-muted">/</span>
          <button
            v-if="idx < breadcrumbs.length - 1"
            @click="goToFolder(b.path)"
            class="text-github-blue hover:underline"
          >
            {{ b.name }}
          </button>
          <span v-else class="text-white font-bold">{{ b.name }}</span>
        </template>
      </div>

      <!-- Current Folder Settings Button (Admin only) -->
      <div v-if="auth.isAdmin && currentFolder && currentFolderMeta" class="flex items-center space-x-2 flex-shrink-0">
        <span
          class="text-[10px] uppercase font-mono px-2 py-0.5 rounded border"
          :class="getVisBadgeClass(currentFolderMeta.visibility)"
        >
          Folder: {{ currentFolderMeta.visibility }}
        </span>
        <button
          @click="showFolderModal = true"
          class="px-2.5 py-1 bg-github-subtle hover:bg-github-border border border-github-border rounded text-[11px] text-white flex items-center space-x-1 transition-colors"
        >
          <Settings class="w-3 h-3 text-github-blue" />
          <span>Folder Settings</span>
        </button>
      </div>
    </div>

    <!-- Loading / Error -->
    <div v-if="loading" class="py-16 text-center">
      <div class="inline-block w-8 h-8 border-2 border-github-border border-t-github-blue rounded-full animate-spin"></div>
      <p class="text-xs text-github-muted mt-3">Loading directory...</p>
    </div>

    <div v-else-if="error" class="py-12 text-center text-red-400 text-sm">
      {{ error }}
    </div>

    <div v-else-if="filteredFolders.length === 0 && filteredRepos.length === 0" class="py-16 text-center border border-dashed border-github-border rounded-lg mt-6 bg-github-dark/30">
      <FolderGit2 class="w-10 h-10 text-github-muted mx-auto mb-2 opacity-50" />
      <h3 class="text-sm font-semibold text-white">No items found</h3>
      <p class="text-xs text-github-muted mt-1">This directory is currently empty.</p>
      <button
        v-if="currentFolder"
        @click="goUpFolder"
        class="mt-4 px-3 py-1.5 bg-github-subtle hover:bg-github-border border border-github-border text-github-blue rounded text-xs transition-colors"
      >
        &larr; Go to parent directory
      </button>
    </div>

    <!-- Items Grid: Folders and Repositories -->
    <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-6">
      <!-- Subfolders -->
      <div
        v-for="folder in filteredFolders"
        :key="folder.path"
        @click="goToFolder(folder.path)"
        class="border border-github-border hover:border-github-blue/60 bg-github-subtle/30 hover:bg-github-subtle/70 p-4 rounded-lg cursor-pointer transition-all flex items-center justify-between group shadow-sm"
      >
        <div class="flex items-center space-x-3 truncate">
          <div class="p-2.5 bg-github-dark rounded-md border border-github-border/60 text-github-blue group-hover:scale-105 transition-transform">
            <Folder class="w-5 h-5" />
          </div>
          <div class="truncate">
            <div class="text-sm font-semibold text-white group-hover:text-github-blue font-mono truncate transition-colors">
              {{ folder.name }}/
            </div>
            <div class="text-[11px] text-github-muted mt-0.5">
              {{ folder.item_count }} item{{ folder.item_count === 1 ? '' : 's' }} inside
            </div>
          </div>
        </div>

        <div class="flex items-center space-x-2 flex-shrink-0">
          <span
            class="text-[10px] px-2 py-0.5 rounded-full border font-mono font-medium"
            :class="getVisBadgeClass(folder.visibility)"
          >
            {{ formatVis(folder.visibility) }}
          </span>
          <ChevronRight class="w-4 h-4 text-github-muted group-hover:text-github-blue group-hover:translate-x-0.5 transition-all" />
        </div>
      </div>

      <!-- Repositories -->
      <div
        v-for="repo in filteredRepos"
        :key="repo.path"
        @click="goToRepo(repo.path)"
        class="border border-github-border hover:border-github-border/80 bg-github-subtle/40 hover:bg-github-subtle/80 p-5 rounded-lg cursor-pointer transition-all flex flex-col justify-between group shadow-sm"
      >
        <div>
          <!-- Header -->
          <div class="flex items-start justify-between gap-2">
            <div class="flex items-center space-x-2.5 truncate">
              <FolderGit2 class="w-5 h-5 text-github-blue flex-shrink-0" />
              <span class="text-sm font-semibold text-github-blue group-hover:underline truncate font-mono">
                {{ repo.name || repo.path }}
              </span>
            </div>
            <span
              class="text-[11px] px-2 py-0.5 rounded-full border font-medium flex-shrink-0 font-mono"
              :class="getVisBadgeClass(repo.visibility || (repo.is_public ? 'public' : 'private'))"
            >
              {{ formatVis(repo.visibility || (repo.is_public ? 'public' : 'private')) }}
            </span>
          </div>

          <!-- Metadata -->
          <div class="mt-4 flex items-center space-x-4 text-xs text-github-muted">
            <span class="flex items-center space-x-1 font-mono">
              <HardDrive class="w-3.5 h-3.5" />
              <span>{{ formatSize(repo.disk_size) }}</span>
            </span>
          </div>
        </div>

        <!-- Footer -->
        <div class="mt-4 pt-3 border-t border-github-border/40 text-[11px] text-github-muted flex items-center justify-between">
          <span class="truncate font-mono text-[10px] text-github-muted/80">{{ repo.path }}</span>
          <span class="text-github-blue group-hover:translate-x-0.5 transition-transform flex-shrink-0">View &rarr;</span>
        </div>
      </div>
    </div>

    <!-- Folder Settings Modal (Admin only) -->
    <div v-if="showFolderModal && currentFolderMeta" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div class="bg-github-subtle border border-github-border rounded-lg max-w-lg w-full p-6 shadow-2xl space-y-5">
        <div class="flex items-center justify-between border-b border-github-border pb-3">
          <h3 class="text-base font-bold text-white flex items-center space-x-2">
            <Folder class="w-5 h-5 text-github-blue" />
            <span>Folder Settings: {{ currentFolder }}</span>
          </h3>
          <button @click="showFolderModal = false" class="text-github-muted hover:text-white transition-colors">
            <X class="w-5 h-5" />
          </button>
        </div>

        <!-- Visibility Setting -->
        <div class="space-y-2">
          <label class="block text-xs font-bold text-github-muted uppercase">Folder Visibility</label>
          <div class="grid grid-cols-3 gap-2 text-xs">
            <button
              v-for="v in ['public', 'unlisted', 'private']"
              :key="v"
              type="button"
              @click="setFolderVisibility(v)"
              :disabled="savingFolder"
              class="py-2 px-3 border rounded text-center capitalize transition-colors font-medium"
              :class="currentFolderMeta.visibility === v ? 'bg-github-blue text-white border-github-blue font-bold shadow' : 'bg-github-dark border-github-border text-github-muted hover:text-white'"
            >
              {{ v }}
            </button>
          </div>
          <p class="text-[11px] text-github-muted mt-1 leading-relaxed">
            <span v-if="currentFolderMeta.visibility === 'public'"><strong>Public:</strong> Anyone can see this folder in listings and browse public/unlisted items inside.</span>
            <span v-else-if="currentFolderMeta.visibility === 'unlisted'"><strong>Unlisted:</strong> Hidden from listings unless user has access, but direct URL links are accessible.</span>
            <span v-else><strong>Private:</strong> Only users with explicit permissions can see this folder.</span>
          </p>
        </div>

        <!-- Collaborators Section -->
        <div class="space-y-3 pt-3 border-t border-github-border">
          <label class="block text-xs font-bold text-github-muted uppercase">Inherited Collaborators</label>
          <form @submit.prevent="addFolderCollab" class="flex gap-2">
            <input
              type="text"
              v-model="newCollabUsername"
              placeholder="Username"
              required
              class="bg-github-dark border border-github-border px-3 py-1.5 rounded text-xs text-white placeholder-github-muted focus:outline-none focus:border-github-blue flex-grow"
            />
            <select
              v-model="newCollabRole"
              class="bg-github-dark border border-github-border px-2 py-1.5 rounded text-xs text-white focus:outline-none focus:border-github-blue"
            >
              <option value="ro">Read-only</option>
              <option value="rw">Read/Write</option>
            </select>
            <button
              type="submit"
              :disabled="savingFolder"
              class="px-3 py-1.5 bg-github-green hover:bg-github-greenHover text-white rounded text-xs font-semibold transition-colors flex-shrink-0"
            >
              Add
            </button>
          </form>

          <div class="border border-github-border rounded divide-y divide-github-border/40 text-xs max-h-40 overflow-y-auto">
            <div
              v-for="(role, uname) in currentFolderMeta.collaborators"
              :key="uname"
              class="p-2.5 flex items-center justify-between hover:bg-github-dark/40"
            >
              <div class="flex items-center space-x-2">
                <span class="font-mono text-white font-medium">{{ uname }}</span>
                <span
                  class="text-[10px] uppercase px-1.5 py-0.2 rounded font-mono"
                  :class="role === 'rw' ? 'bg-purple-950 text-purple-300 border border-purple-800' : 'bg-blue-950 text-blue-300 border border-blue-800'"
                >
                  {{ role === 'rw' ? 'Write' : 'Read' }}
                </span>
              </div>
              <button
                @click="removeFolderCollab(String(uname))"
                class="text-red-400 hover:text-red-300 transition-colors"
              >
                Remove
              </button>
            </div>
            <div v-if="!currentFolderMeta.collaborators || Object.keys(currentFolderMeta.collaborators).length === 0" class="p-3 text-center text-github-muted text-xs">
              No folder-level collaborators.
            </div>
          </div>
        </div>

        <div class="pt-3 border-t border-github-border flex justify-end">
          <button
            @click="showFolderModal = false"
            class="px-4 py-2 bg-github-dark hover:bg-github-border border border-github-border rounded text-xs font-semibold text-white transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  FolderGit2,
  Folder,
  Home,
  HardDrive,
  ChevronRight,
  Settings,
  X,
} from '@lucide/vue'
import { useAuthStore } from '../stores/auth'
import { api } from '../api'
import type { RepoSummary, FolderItem, BreadcrumbItem } from '../types'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const repos = ref<RepoSummary[]>([])
const folders = ref<FolderItem[]>([])
const breadcrumbs = ref<BreadcrumbItem[]>([])
const currentFolderMeta = ref<any>(null)
const loading = ref(true)
const error = ref('')
const searchQuery = ref('')

const showFolderModal = ref(false)
const savingFolder = ref(false)
const newCollabUsername = ref('')
const newCollabRole = ref('ro')

const currentFolder = computed(() => {
  return (route.query.folder as string) || ''
})

const filteredFolders = computed(() => {
  let list = folders.value
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase()
    list = list.filter(f => f.name.toLowerCase().includes(q))
  }
  return [...list].sort((a, b) => a.name.localeCompare(b.name, 'ja', { numeric: true }))
})

const filteredRepos = computed(() => {
  let list = repos.value
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase()
    list = list.filter(r => (r.name || r.path).toLowerCase().includes(q))
  }
  return [...list].sort((a, b) => (a.name || a.path).localeCompare(b.name || b.path, 'ja', { numeric: true }))
})

onMounted(async () => {
  await loadData()
})

watch(
  () => route.query.folder,
  async () => {
    await loadData()
  }
)

async function loadData() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.getRepos(currentFolder.value)
    repos.value = (res.repos || []).sort((a, b) =>
      (a.name || a.path).localeCompare(b.name || b.path, 'ja', { numeric: true })
    )
    folders.value = (res.folders || []).sort((a, b) =>
      a.name.localeCompare(b.name, 'ja', { numeric: true })
    )
    breadcrumbs.value = res.breadcrumbs || []
    currentFolderMeta.value = res.current_folder_meta || null
  } catch (err: any) {
    error.value = err.message || 'Failed to load directory'
  } finally {
    loading.value = false
  }
}

function goToFolder(folderPath: string) {
  router.push({
    path: '/',
    query: folderPath ? { folder: folderPath } : {},
  })
}

function goUpFolder() {
  if (!breadcrumbs.value || breadcrumbs.value.length <= 1) {
    goToFolder('')
    return
  }
  const parent = breadcrumbs.value[breadcrumbs.value.length - 2].path
  goToFolder(parent)
}

function goToRepo(repoPath: string) {
  router.push(`/${repoPath}`)
}

function formatVis(vis?: string): string {
  if (!vis) return 'Private'
  return vis.charAt(0).toUpperCase() + vis.slice(1)
}

function getVisBadgeClass(vis?: string): string {
  if (vis === 'public') {
    return 'bg-emerald-950/40 text-emerald-400 border-emerald-800/60'
  }
  if (vis === 'unlisted') {
    return 'bg-blue-950/40 text-blue-300 border-blue-800/60'
  }
  return 'bg-amber-950/40 text-amber-300 border-amber-800/60'
}

function formatSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

// Folder settings
async function setFolderVisibility(vis: string) {
  if (!currentFolder.value) return
  savingFolder.value = true
  try {
    const res = await api.updateFolderVisibility(currentFolder.value, vis)
    if (currentFolderMeta.value) {
      currentFolderMeta.value.visibility = res.visibility
      currentFolderMeta.value.is_public = res.is_public
    }
  } catch (err: any) {
    alert(err.message)
  } finally {
    savingFolder.value = false
  }
}

async function addFolderCollab() {
  if (!currentFolder.value || !newCollabUsername.value.trim()) return
  savingFolder.value = true
  try {
    const res = await api.addFolderCollaborator(currentFolder.value, newCollabUsername.value.trim(), newCollabRole.value)
    if (currentFolderMeta.value) {
      currentFolderMeta.value.collaborators = res.collaborators
    }
    newCollabUsername.value = ''
  } catch (err: any) {
    alert(err.message)
  } finally {
    savingFolder.value = false
  }
}

async function removeFolderCollab(uname: string) {
  if (!currentFolder.value) return
  savingFolder.value = true
  try {
    const res = await api.deleteFolderCollaborator(currentFolder.value, uname)
    if (currentFolderMeta.value) {
      currentFolderMeta.value.collaborators = res.collaborators
    }
  } catch (err: any) {
    alert(err.message)
  } finally {
    savingFolder.value = false
  }
}
</script>
