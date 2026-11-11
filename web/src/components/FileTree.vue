<template>
  <div class="border border-github-border rounded-md overflow-hidden bg-github-dark">
    <!-- Breadcrumb Header -->
    <div class="bg-github-subtle px-4 py-2.5 border-b border-github-border flex items-center justify-between text-sm">
      <div class="flex items-center space-x-1.5 font-mono text-xs overflow-x-auto py-0.5">
        <router-link
          :to="{ path: `/${repoPath}`, query: { ref, tab: 'code' } }"
          class="text-github-blue hover:underline font-semibold"
        >
          {{ repoPath }}
        </router-link>
        <template v-for="(seg, idx) in pathSegments" :key="idx">
          <span class="text-github-muted">/</span>
          <router-link
            v-if="idx < pathSegments.length - 1"
            :to="{ path: `/${repoPath}`, query: { ref, tab: 'code', path: getSubPath(idx) } }"
            class="text-github-blue hover:underline"
          >
            {{ seg }}
          </router-link>
          <span v-else class="text-github-text font-semibold">{{ seg }}</span>
        </template>
      </div>

      <div v-if="latestCommit" class="hidden md:flex items-center space-x-2 text-xs text-github-muted truncate max-w-sm">
        <span class="truncate">{{ latestCommit.message }}</span>
        <span>•</span>
        <span class="font-mono text-github-blue">{{ latestCommit.short_hash }}</span>
      </div>
    </div>

    <!-- Tree Entries List -->
    <div class="divide-y divide-github-border/40 text-sm">
      <!-- Parent directory link if nested -->
      <div
        v-if="currentPath"
        @click="goUp"
        class="px-4 py-2 hover:bg-github-subtle/50 flex items-center space-x-3 cursor-pointer text-github-blue text-xs transition-colors"
      >
        <FolderUp class="w-4 h-4 text-github-muted" />
        <span class="font-semibold">..</span>
      </div>

      <div
        v-for="entry in sortedEntries"
        :key="entry.name"
        @click="onEntryClick(entry)"
        class="px-4 py-2 hover:bg-github-subtle/70 flex items-center justify-between cursor-pointer transition-colors group"
      >
        <div class="flex items-center space-x-2.5 truncate">
          <Folder v-if="entry.type === 'tree'" class="w-4 h-4 text-github-blue flex-shrink-0" />
          <FileCode v-else-if="isCodeFile(entry.name)" class="w-4 h-4 text-emerald-400 flex-shrink-0" />
          <FileText v-else class="w-4 h-4 text-github-muted flex-shrink-0" />

          <span
            class="truncate group-hover:text-github-blue transition-colors text-xs font-mono"
            :class="{ 'font-semibold text-white': entry.type === 'tree' }"
          >
            {{ entry.name }}
          </span>
        </div>

        <div class="text-xs text-github-muted font-mono flex-shrink-0 pl-4">
          <span v-if="entry.type !== 'tree'">{{ formatSize(entry.size) }}</span>
        </div>
      </div>

      <div v-if="entries.length === 0" class="px-4 py-8 text-center text-xs text-github-muted">
        No files in this directory.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { Folder, FolderUp, FileText, FileCode } from '@lucide/vue'
import type { TreeEntry, CommitInfo } from '../types'

const props = defineProps<{
  repoPath: string
  refName: string
  currentPath: string
  entries: TreeEntry[]
  latestCommit?: CommitInfo | null
}>()

const router = useRouter()

const ref = computed(() => props.refName)

const pathSegments = computed(() => {
  return props.currentPath ? props.currentPath.split('/').filter(Boolean) : []
})

function getSubPath(idx: number): string {
  return pathSegments.value.slice(0, idx + 1).join('/')
}

const sortedEntries = computed(() => {
  const trees = props.entries.filter(e => e.type === 'tree').sort((a, b) => a.name.localeCompare(b.name))
  const blobs = props.entries.filter(e => e.type !== 'tree').sort((a, b) => a.name.localeCompare(b.name))
  return [...trees, ...blobs]
})

function onEntryClick(entry: TreeEntry) {
  const fullPath = props.currentPath ? `${props.currentPath}/${entry.name}` : entry.name
  if (entry.type === 'tree') {
    router.push({
      path: `/${props.repoPath}`,
      query: { ref: props.refName, tab: 'code', path: fullPath },
    })
  } else {
    router.push({
      path: `/${props.repoPath}`,
      query: { ref: props.refName, tab: 'code', path: fullPath, view: 'blob' },
    })
  }
}

function goUp() {
  const segs = [...pathSegments.value]
  segs.pop()
  const parent = segs.join('/')
  router.push({
    path: `/${props.repoPath}`,
    query: { ref: props.refName, tab: 'code', path: parent || undefined },
  })
}

function isCodeFile(name: string): boolean {
  return /\.(go|ts|js|vue|py|rs|c|cpp|h|java|php|rb|html|css|json|yaml|yml|md|sh)$/i.test(name)
}

function formatSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}
</script>
