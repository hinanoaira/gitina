<template>
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
    <div v-if="loading" class="py-16 text-center">
      <div class="inline-block w-8 h-8 border-2 border-github-border border-t-github-blue rounded-full animate-spin"></div>
      <p class="text-xs text-github-muted mt-3">Loading repository...</p>
    </div>

    <div v-else-if="error" class="py-16 text-center">
      <div class="p-4 bg-red-950/40 border border-red-800 rounded-lg max-w-lg mx-auto text-xs text-red-300">
        {{ error }}
      </div>
      <div class="mt-4">
        <router-link to="/" class="text-xs text-github-blue hover:underline">&larr; Back to Dashboard</router-link>
      </div>
    </div>

    <div v-else-if="repoInfo">
      <!-- Repo Header -->
      <div class="border-b border-github-border pb-4">
        <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
          <!-- Title & Visibility -->
          <div class="flex items-center space-x-2.5 truncate">
            <FolderGit2 class="w-6 h-6 text-github-muted flex-shrink-0" />
            <h1 class="text-xl font-bold text-white font-mono truncate">
              {{ repoInfo.path }}
            </h1>
            <span
              class="text-xs px-2.5 py-0.5 rounded-full border font-mono font-medium flex-shrink-0"
              :class="getVisBadgeClass(repoInfo.visibility || (repoInfo.is_public ? 'public' : 'private'))"
            >
              {{ formatVis(repoInfo.visibility || (repoInfo.is_public ? 'public' : 'private')) }}
            </span>
          </div>

          <!-- Actions: PAT & Clone URL -->
          <div class="flex items-center space-x-2 flex-wrap gap-y-2">
            <!-- PAT Button (If logged in and has access) -->
            <button
              v-if="auth.user && repoInfo.can_read"
              @click="showTokenModal = true"
              class="px-3 py-1.5 bg-github-subtle hover:bg-github-border border border-github-border text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition-colors shadow-sm"
            >
              <Key class="w-3.5 h-3.5 text-github-blue" />
              <span>Generate PAT</span>
            </button>

            <!-- Clone Dropdown / Input -->
            <div class="relative">
              <button
                @click="showCloneMenu = !showCloneMenu"
                class="px-3 py-1.5 bg-github-green hover:bg-github-greenHover text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition-colors shadow-sm"
              >
                <Download class="w-3.5 h-3.5" />
                <span>Clone</span>
                <ChevronDown class="w-3 h-3" />
              </button>

              <div
                v-if="showCloneMenu"
                class="absolute right-0 mt-2 w-80 bg-github-subtle border border-github-border rounded-lg shadow-2xl p-4 z-50 text-xs"
              >
                <div class="font-bold text-white mb-1.5">Clone with HTTP</div>
                <div class="flex items-center space-x-2">
                  <input
                    type="text"
                    readonly
                    :value="repoInfo.clone_url"
                    class="w-full bg-github-darker border border-github-border px-2.5 py-1.5 rounded text-[11px] font-mono text-github-text select-all focus:outline-none"
                  />
                  <button
                    @click="copyCloneURL"
                    class="px-2.5 py-1.5 bg-github-dark hover:bg-github-border border border-github-border rounded text-white flex-shrink-0 transition-colors"
                  >
                    <Check v-if="cloneCopied" class="w-3.5 h-3.5 text-emerald-400" />
                    <Copy v-else class="w-3.5 h-3.5" />
                  </button>
                </div>
                <div class="mt-2 text-[10px] text-github-muted">
                  Use your GitHub credentials or a Gitina Personal Access Token for authentication.
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Navigation Tabs -->
        <div class="flex items-center space-x-1 mt-6 -mb-4 overflow-x-auto text-xs font-medium">
          <button
            @click="switchTab('code')"
            class="px-4 py-2 border-b-2 flex items-center space-x-2 transition-colors flex-shrink-0"
            :class="currentTab === 'code' ? 'border-github-blue text-white font-semibold' : 'border-transparent text-github-muted hover:text-white'"
          >
            <Code class="w-4 h-4" />
            <span>Code</span>
          </button>
          <button
            @click="switchTab('commits')"
            class="px-4 py-2 border-b-2 flex items-center space-x-2 transition-colors flex-shrink-0"
            :class="currentTab === 'commits' ? 'border-github-blue text-white font-semibold' : 'border-transparent text-github-muted hover:text-white'"
          >
            <GitCommit class="w-4 h-4" />
            <span>Commits</span>
          </button>
          <button
            v-if="auth.user && repoInfo.can_read"
            @click="switchTab('tokens')"
            class="px-4 py-2 border-b-2 flex items-center space-x-2 transition-colors flex-shrink-0"
            :class="currentTab === 'tokens' ? 'border-github-blue text-white font-semibold' : 'border-transparent text-github-muted hover:text-white'"
          >
            <Key class="w-4 h-4" />
            <span>Access Tokens</span>
          </button>
          <button
            v-if="repoInfo.is_admin"
            @click="switchTab('settings')"
            class="px-4 py-2 border-b-2 flex items-center space-x-2 transition-colors flex-shrink-0"
            :class="currentTab === 'settings' ? 'border-github-blue text-white font-semibold' : 'border-transparent text-github-muted hover:text-white'"
          >
            <Settings class="w-4 h-4" />
            <span>Settings</span>
          </button>
        </div>
      </div>

      <!-- Tab Content Area -->
      <div class="mt-6">
        <!-- CODE TAB -->
        <div v-if="currentTab === 'code'">
          <!-- Branch Selector & Top Bar -->
          <div class="flex items-center justify-between mb-4">
            <div class="flex items-center space-x-2">
              <div class="relative">
                <button
                  @click="showBranchMenu = !showBranchMenu"
                  class="px-3 py-1.5 bg-github-subtle hover:bg-github-border border border-github-border rounded text-xs font-semibold text-white flex items-center space-x-2 transition-colors shadow-sm"
                >
                  <GitBranch class="w-3.5 h-3.5 text-github-blue" />
                  <span class="font-mono">{{ currentRef }}</span>
                  <ChevronDown class="w-3 h-3 text-github-muted" />
                </button>

                <div
                  v-if="showBranchMenu"
                  class="absolute left-0 mt-2 w-56 bg-github-subtle border border-github-border rounded-lg shadow-xl p-2 z-40 text-xs"
                >
                  <div class="font-semibold text-github-muted px-2 py-1 uppercase text-[10px] tracking-wider">Switch branches</div>
                  <div class="max-h-56 overflow-y-auto divide-y divide-github-border/30">
                    <button
                      v-for="b in branchList"
                      :key="b"
                      @click="selectBranch(b)"
                      class="w-full text-left px-2 py-1.5 hover:bg-github-dark rounded flex items-center justify-between text-xs font-mono transition-colors"
                      :class="b === currentRef ? 'text-github-blue font-bold' : 'text-github-text'"
                    >
                      <span>{{ b }}</span>
                      <Check v-if="b === currentRef" class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              </div>

              <span class="text-xs text-github-muted">
                {{ branchList.length }} branch{{ branchList.length === 1 ? '' : 'es' }}
              </span>
            </div>
          </div>

          <!-- Tree or Blob View -->
          <div v-if="loadingContent" class="py-12 text-center text-xs text-github-muted">
            <div class="inline-block w-6 h-6 border-2 border-github-border border-t-github-blue rounded-full animate-spin"></div>
            <p class="mt-2">Loading content...</p>
          </div>

          <div v-else>
            <!-- Blob Viewer -->
            <BlobViewer
              v-if="currentBlob"
              :file-path="currentPath"
              :blob="currentBlob"
            />

            <!-- File Tree View -->
            <FileTree
              v-else-if="treeEntries"
              :repo-path="repoPath"
              :ref-name="currentRef"
              :current-path="currentPath"
              :entries="treeEntries"
              :latest-commit="treeLatestCommit"
            />

            <!-- README.md Viewer if available in tree view -->
            <ReadmeViewer
              v-if="!currentBlob && treeReadme"
              title="README.md"
              :content="treeReadme.content"
            />
          </div>
        </div>

        <!-- COMMITS TAB -->
        <div v-else-if="currentTab === 'commits'">
          <div v-if="loadingCommits" class="py-12 text-center text-xs text-github-muted">
            <div class="inline-block w-6 h-6 border-2 border-github-border border-t-github-blue rounded-full animate-spin"></div>
            <p class="mt-2">Loading commits...</p>
          </div>

          <div v-else-if="commits.length === 0" class="py-12 text-center text-xs text-github-muted">
            No commits found on branch {{ currentRef }}.
          </div>

          <div v-else class="border border-github-border rounded-md bg-github-dark divide-y divide-github-border/40">
            <div
              v-for="c in commits"
              :key="c.hash || (c as any).Hash"
              class="p-4 hover:bg-github-subtle/40 transition-colors flex flex-col md:flex-row md:items-center justify-between gap-2"
            >
              <div>
                <div class="text-sm font-semibold text-white leading-snug">
                  {{ c.message || (c as any).Message }}
                </div>
                <div class="text-xs text-github-muted mt-1 flex items-center space-x-2">
                  <span class="text-github-text font-medium">{{ c.author_name || (c as any).AuthorName }}</span>
                  <span>committed on {{ formatDate(c.author_date || (c as any).AuthorDate) }}</span>
                </div>
              </div>

              <div class="flex items-center space-x-2 flex-shrink-0">
                <span class="font-mono text-xs px-2.5 py-1 bg-github-subtle border border-github-border rounded text-github-blue select-all">
                  {{ c.short_hash || (c as any).ShortHash }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <!-- ACCESS TOKENS TAB -->
        <div v-else-if="currentTab === 'tokens'" class="space-y-6 max-w-5xl">
          <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between pb-4 border-b border-github-border gap-4">
            <div>
              <h2 class="text-base font-bold text-white flex items-center space-x-2">
                <Key class="w-5 h-5 text-github-blue" />
                <span>Personal Access Tokens</span>
              </h2>
              <p class="text-xs text-github-muted mt-1">
                Authenticate with Git over HTTP using Basic Authentication. Pass the <strong>Username (ID)</strong> and <strong>Password (Token)</strong>.
              </p>
            </div>
            <button
              @click="showTokenModal = true"
              class="px-3.5 py-1.5 bg-github-green hover:bg-github-greenHover text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition-colors shadow-sm self-start sm:self-auto"
            >
              <Key class="w-3.5 h-3.5" />
              <span>Generate New Token</span>
            </button>
          </div>

          <div v-if="loadingTokens" class="py-12 text-center text-xs text-github-muted">
            <div class="inline-block w-6 h-6 border-2 border-github-border border-t-github-blue rounded-full animate-spin"></div>
            <p class="mt-2">Loading access tokens...</p>
          </div>

          <div v-else-if="tokens.length === 0" class="py-12 text-center border border-dashed border-github-border rounded-lg bg-github-dark/40">
            <Key class="w-8 h-8 text-github-muted mx-auto mb-2 opacity-50" />
            <div class="text-sm font-semibold text-white">No personal access tokens</div>
            <p class="text-xs text-github-muted mt-1">You haven't generated any access tokens for this repository yet.</p>
          </div>

          <div v-else class="border border-github-border rounded-lg bg-github-dark overflow-hidden">
            <div class="overflow-x-auto">
              <table class="w-full text-left text-xs">
                <thead class="bg-github-subtle text-github-muted border-b border-github-border font-semibold uppercase tracking-wider text-[10px]">
                  <tr>
                    <th class="px-4 py-3">Token Name</th>
                    <th class="px-4 py-3">Basic Auth ID</th>
                    <th class="px-4 py-3">Scope</th>
                    <th class="px-4 py-3">Created</th>
                    <th class="px-4 py-3">Expires</th>
                    <th class="px-4 py-3 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-github-border/40 text-github-text">
                  <tr v-for="t in tokens" :key="t.id" class="hover:bg-github-subtle/30 transition-colors">
                    <td class="px-4 py-3 font-semibold text-white">
                      {{ t.name }}
                    </td>
                    <td class="px-4 py-3">
                      <div class="flex items-center space-x-1.5 font-mono text-[11px] text-github-blue">
                        <span>{{ t.id }}</span>
                        <button
                          @click="copyText(t.id)"
                          title="Copy ID"
                          class="p-1 text-github-muted hover:text-white transition-colors"
                        >
                          <Copy class="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </td>
                    <td class="px-4 py-3">
                      <span
                        class="text-[10px] uppercase font-mono px-2 py-0.5 rounded border"
                        :class="t.scope === 'rw' ? 'bg-purple-950/60 text-purple-300 border-purple-800' : 'bg-blue-950/60 text-blue-300 border-blue-800'"
                      >
                        {{ t.scope === 'rw' ? 'Read / Write' : 'Read Only' }}
                      </span>
                    </td>
                    <td class="px-4 py-3 text-github-muted">
                      {{ formatDate(t.created_at) }}
                    </td>
                    <td class="px-4 py-3">
                      <template v-if="t.expires_at">
                        <span v-if="isExpired(t.expires_at)" class="text-red-400 font-medium flex items-center space-x-1">
                          <span>Expired ({{ new Date(t.expires_at).toLocaleDateString() }})</span>
                        </span>
                        <span v-else class="text-github-muted">
                          {{ new Date(t.expires_at).toLocaleDateString() }}
                        </span>
                      </template>
                      <span v-else class="text-github-muted">Never</span>
                    </td>
                    <td class="px-4 py-3 text-right">
                      <button
                        @click="revoke(t.id, t.name)"
                        :disabled="revokingId === t.id"
                        class="px-2.5 py-1 text-xs text-red-400 hover:text-white bg-red-950/30 hover:bg-red-900 border border-red-900/60 rounded transition-colors"
                      >
                        {{ revokingId === t.id ? 'Revoking...' : 'Revoke' }}
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>

        <!-- SETTINGS TAB (Admin Only) -->
        <div v-else-if="currentTab === 'settings' && repoInfo.is_admin" class="space-y-8 max-w-4xl">
          <!-- Visibility Card -->
          <div class="border border-github-border rounded-lg bg-github-dark p-6">
            <h3 class="text-sm font-bold text-white mb-2">Repository Visibility</h3>
            <p class="text-xs text-github-muted mb-4">
              Choose the visibility level for this repository.
            </p>
            <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <button
                v-for="v in ['public', 'unlisted', 'private']"
                :key="v"
                type="button"
                @click="changeVisibility(v)"
                :disabled="savingSettings"
                class="p-3 border rounded-lg text-left transition-all"
                :class="(repoInfo.visibility || (repoInfo.is_public ? 'public' : 'private')) === v ? 'bg-github-subtle border-github-blue ring-1 ring-github-blue' : 'bg-github-darker border-github-border hover:border-github-border/80'"
              >
                <div class="flex items-center justify-between mb-1">
                  <span class="font-bold text-xs capitalize text-white">{{ v }}</span>
                  <Check v-if="(repoInfo.visibility || (repoInfo.is_public ? 'public' : 'private')) === v" class="w-4 h-4 text-github-blue" />
                </div>
                <div class="text-[11px] text-github-muted leading-tight">
                  <span v-if="v === 'public'">Visible in directory listings and viewable by everyone.</span>
                  <span v-else-if="v === 'unlisted'">Hidden from listings unless user has access, but viewable by anyone with the direct URL.</span>
                  <span v-else>Only users with explicit permissions can view.</span>
                </div>
              </button>
            </div>
          </div>

          <!-- Collaborators Card -->
          <div class="border border-github-border rounded-lg bg-github-dark p-6">
            <h3 class="text-sm font-bold text-white mb-2">Collaborator Access</h3>
            <p class="text-xs text-github-muted mb-4">
              Manage users who have Read or Write access to this repository.
            </p>

            <!-- Add Collaborator Form -->
            <form @submit.prevent="addCollab" class="flex flex-col sm:flex-row gap-2 mb-6">
              <input
                type="text"
                v-model="newCollabUsername"
                placeholder="GitHub Username"
                required
                class="bg-github-subtle border border-github-border px-3 py-1.5 rounded text-xs text-github-text placeholder-github-muted focus:outline-none focus:border-github-blue flex-grow"
              />
              <select
                v-model="newCollabRole"
                class="bg-github-subtle border border-github-border px-3 py-1.5 rounded text-xs text-github-text focus:outline-none focus:border-github-blue"
              >
                <option value="ro">Read-only (RO)</option>
                <option value="rw">Read & Write (RW)</option>
              </select>
              <button
                type="submit"
                :disabled="savingSettings"
                class="px-4 py-1.5 bg-github-green hover:bg-github-greenHover text-white text-xs font-semibold rounded transition-colors"
              >
                Add User
              </button>
            </form>

            <!-- Collaborators List -->
            <div class="border border-github-border/60 rounded divide-y divide-github-border/40 text-xs">
              <div
                v-for="(role, uname) in repoInfo.collaborators"
                :key="uname"
                class="p-3 flex items-center justify-between hover:bg-github-subtle/30"
              >
                <div class="flex items-center space-x-2">
                  <span class="font-semibold text-white font-mono">{{ uname }}</span>
                  <span
                    class="text-[10px] uppercase px-1.5 py-0.2 rounded font-mono"
                    :class="role === 'rw' ? 'bg-purple-950 text-purple-300 border border-purple-800' : 'bg-blue-950 text-blue-300 border border-blue-800'"
                  >
                    {{ role === 'rw' ? 'Write' : 'Read' }}
                  </span>
                </div>
                <button
                  @click="deleteCollab(String(uname))"
                  class="text-red-400 hover:text-red-300 transition-colors"
                >
                  Remove
                </button>
              </div>
              <div v-if="!repoInfo.collaborators || Object.keys(repoInfo.collaborators).length === 0" class="p-4 text-center text-github-muted text-xs">
                No specific collaborators assigned.
              </div>
            </div>
          </div>

          <!-- Danger Zone: Delete Repo -->
          <div class="border border-red-900/60 rounded-lg bg-red-950/10 p-6">
            <h3 class="text-sm font-bold text-red-400 mb-1">Danger Zone</h3>
            <p class="text-xs text-github-muted mb-4">
              Deleting this repository will move it to the server trash folder. This action cannot be easily undone.
            </p>
            <div class="space-y-3">
              <div>
                <label class="block text-xs text-github-muted mb-1">
                  Type <span class="font-mono text-white font-semibold">{{ repoInfo.path }}</span> to confirm:
                </label>
                <input
                  type="text"
                  v-model="deleteConfirmName"
                  placeholder="repository name"
                  class="w-full sm:w-80 bg-github-dark border border-github-border px-3 py-1.5 rounded text-xs text-github-text focus:outline-none focus:border-red-600"
                />
              </div>
              <button
                @click="deleteRepoAction"
                :disabled="deleteConfirmName !== repoInfo.path && deleteConfirmName !== repoInfo.path.replace('.git', '')"
                class="px-4 py-2 bg-red-700 hover:bg-red-600 disabled:opacity-40 text-white rounded text-xs font-semibold transition-colors"
              >
                Delete this repository
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Token Modal -->
    <TokenModal
      :is-open="showTokenModal"
      :repo-path="repoPath"
      :can-write="repoInfo?.can_write ?? false"
      @close="showTokenModal = false"
      @created="loadTokens"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  FolderGit2,
  Key,
  Download,
  ChevronDown,
  Code,
  GitCommit,
  Settings,
  GitBranch,
  Copy,
  Check,
} from '@lucide/vue'
import { useAuthStore } from '../stores/auth'
import { api } from '../api'
import type { RepoDetail, TreeEntry, BlobContent, CommitInfo, TokenItem } from '../types'
import FileTree from '../components/FileTree.vue'
import BlobViewer from '../components/BlobViewer.vue'
import ReadmeViewer from '../components/ReadmeViewer.vue'
import TokenModal from '../components/TokenModal.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

// State
const repoInfo = ref<RepoDetail | null>(null)
const loading = ref(true)
const error = ref('')

const currentRef = ref('')
const currentPath = ref('')
const currentBlob = ref<BlobContent | null>(null)
const treeEntries = ref<TreeEntry[] | null>(null)
const treeReadme = ref<BlobContent | null>(null)
const treeLatestCommit = ref<CommitInfo | null>(null)
const loadingContent = ref(false)

const commits = ref<CommitInfo[]>([])
const loadingCommits = ref(false)

const tokens = ref<TokenItem[]>([])
const loadingTokens = ref(false)
const revokingId = ref('')

const showCloneMenu = ref(false)
const cloneCopied = ref(false)
const showBranchMenu = ref(false)
const showTokenModal = ref(false)

const savingSettings = ref(false)
const newCollabUsername = ref('')
const newCollabRole = ref('ro')
const deleteConfirmName = ref('')

// Compute repoPath from router params
const repoPath = computed(() => {
  const p = route.params.repoPath
  if (Array.isArray(p)) return p.join('/')
  return p || ''
})

const currentTab = computed(() => {
  return (route.query.tab as string) || 'code'
})

const branchList = computed<string[]>(() => {
  if (!repoInfo.value?.branches) return []
  return repoInfo.value.branches.map((b) => (typeof b === 'string' ? b : b.name))
})

onMounted(async () => {
  await loadRepoInfo()
})

watch(
  () => [route.params.repoPath, route.query.ref, route.query.tab, route.query.path, route.query.view],
  async () => {
    if (!repoInfo.value || repoInfo.value.path !== repoPath.value) {
      await loadRepoInfo()
    } else {
      await handleRouteQueryChange()
    }
  }
)

async function loadRepoInfo() {
  loading.value = true
  error.value = ''
  try {
    const info = await api.getRepoInfo(repoPath.value)
    repoInfo.value = info
    currentRef.value = (route.query.ref as string) || info.default_branch || 'main'
    await handleRouteQueryChange()
  } catch (err: any) {
    error.value = err.message || 'Failed to load repository'
  } finally {
    loading.value = false
  }
}

async function handleRouteQueryChange() {
  const tab = currentTab.value
  const qRef = (route.query.ref as string) || repoInfo.value?.default_branch || 'main'
  currentRef.value = qRef
  currentPath.value = (route.query.path as string) || ''

  if (tab === 'code') {
    await loadCodeTabContent()
  } else if (tab === 'commits') {
    await loadCommits()
  } else if (tab === 'tokens') {
    await loadTokens()
  }
}

async function loadCodeTabContent() {
  loadingContent.value = true
  currentBlob.value = null
  treeEntries.value = null
  treeReadme.value = null

  try {
    const isBlobView = route.query.view === 'blob'
    if (isBlobView && currentPath.value) {
      const res = await api.getBlob(repoPath.value, currentRef.value, currentPath.value)
      currentBlob.value = res.blob
    } else {
      const res = await api.getTree(repoPath.value, currentRef.value, currentPath.value)
      treeEntries.value = res.entries || []
      treeReadme.value = res.readme || null
      treeLatestCommit.value = res.latest_commit || null
    }
  } catch (err: any) {
    // If blob failed, try listing tree
    try {
      const res = await api.getTree(repoPath.value, currentRef.value, currentPath.value)
      treeEntries.value = res.entries || []
      treeReadme.value = res.readme || null
      treeLatestCommit.value = res.latest_commit || null
    } catch {
      // ignore
    }
  } finally {
    loadingContent.value = false
  }
}

async function loadCommits() {
  loadingCommits.value = true
  try {
    const res = await api.getCommits(repoPath.value, currentRef.value)
    commits.value = res.commits || []
  } catch {
    commits.value = []
  } finally {
    loadingCommits.value = false
  }
}

function switchTab(tab: string) {
  router.push({
    path: `/${repoPath.value}`,
    query: { ...route.query, tab },
  })
}

function selectBranch(branch: string) {
  showBranchMenu.value = false
  router.push({
    path: `/${repoPath.value}`,
    query: { ...route.query, ref: branch },
  })
}

async function copyCloneURL() {
  if (!repoInfo.value) return
  await navigator.clipboard.writeText(repoInfo.value.clone_url)
  cloneCopied.value = true
  setTimeout(() => {
    cloneCopied.value = false
  }, 2000)
}

function formatDate(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

// Settings handlers

async function addCollab() {
  if (!repoInfo.value || !newCollabUsername.value) return
  savingSettings.value = true
  try {
    const res = await api.addCollaborator(repoPath.value, newCollabUsername.value, newCollabRole.value)
    repoInfo.value.collaborators = res.collaborators
    newCollabUsername.value = ''
  } catch (err: any) {
    alert(err.message)
  } finally {
    savingSettings.value = false
  }
}

async function deleteCollab(uname: string) {
  if (!repoInfo.value) return
  savingSettings.value = true
  try {
    const res = await api.deleteCollaborator(repoPath.value, uname)
    repoInfo.value.collaborators = res.collaborators
  } catch (err: any) {
    alert(err.message)
  } finally {
    savingSettings.value = false
  }
}

async function deleteRepoAction() {
  if (!repoInfo.value) return
  try {
    await api.deleteRepo(repoPath.value, deleteConfirmName.value, true)
    router.push('/')
  } catch (err: any) {
    alert(err.message)
  }
}

// Token handlers
async function loadTokens() {
  loadingTokens.value = true
  try {
    const res = await api.getTokens(repoPath.value)
    tokens.value = res.tokens || []
  } catch {
    tokens.value = []
  } finally {
    loadingTokens.value = false
  }
}

async function revoke(id: string, name: string) {
  if (!confirm(`Are you sure you want to revoke token "${name}"? Any Git clients using this token will immediately lose access.`)) {
    return
  }
  revokingId.value = id
  try {
    await api.revokeToken(repoPath.value, id)
    await loadTokens()
  } catch (err: any) {
    alert(err.message || 'Failed to revoke token')
  } finally {
    revokingId.value = ''
  }
}

async function copyText(text: string) {
  await navigator.clipboard.writeText(text)
  alert('Copied to clipboard: ' + text)
}

function formatVis(vis?: string): string {
  if (!vis) return 'Private'
  return vis.charAt(0).toUpperCase() + vis.slice(1)
}

function getVisBadgeClass(vis?: string): string {
  if (vis === 'public') return 'bg-emerald-950/40 text-emerald-400 border-emerald-800/60'
  if (vis === 'unlisted') return 'bg-blue-950/40 text-blue-300 border-blue-800/60'
  return 'bg-amber-950/40 text-amber-300 border-amber-800/60'
}

async function changeVisibility(vis: string) {
  if (!repoInfo.value) return
  savingSettings.value = true
  try {
    const res = await api.updateRepoVisibility(repoPath.value, vis)
    repoInfo.value.visibility = res.visibility
    repoInfo.value.is_public = res.is_public
  } catch (err: any) {
    alert(err.message)
  } finally {
    savingSettings.value = false
  }
}

function isExpired(dateStr?: string | null): boolean {
  if (!dateStr) return false
  return new Date().getTime() > new Date(dateStr).getTime()
}
</script>
