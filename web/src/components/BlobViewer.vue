<template>
  <div class="border border-github-border rounded-md overflow-hidden bg-github-dark">
    <!-- Header -->
    <div class="bg-github-subtle px-4 py-2.5 border-b border-github-border flex items-center justify-between">
      <div class="flex items-center space-x-2 text-xs font-mono">
        <FileText class="w-4 h-4 text-github-muted" />
        <span class="font-semibold text-white">{{ fileName }}</span>
        <span class="text-github-muted">({{ formatSize(blob.size) }}<span v-if="!blob.is_binary">, {{ lineCount }} lines</span>)</span>
      </div>

      <div class="flex items-center space-x-2 text-xs">
        <button
          v-if="!blob.is_binary"
          @click="copyContent"
          class="px-2.5 py-1 bg-github-dark hover:bg-github-border border border-github-border rounded text-github-text hover:text-white transition-colors flex items-center space-x-1"
        >
          <Check v-if="copied" class="w-3.5 h-3.5 text-emerald-400" />
          <Copy v-else class="w-3.5 h-3.5" />
          <span>{{ copied ? 'Copied!' : 'Copy' }}</span>
        </button>
      </div>
    </div>

    <!-- Content -->
    <div v-if="isBinary" class="p-8 text-center text-xs text-github-muted">
      Binary file cannot be displayed in browser.
    </div>
    <div v-else class="overflow-x-auto text-xs font-mono leading-relaxed bg-github-darker flex">
      <!-- Line Numbers -->
      <div class="select-none py-2 text-right text-github-muted bg-github-subtle/20 border-r border-github-border/30 px-3 flex-shrink-0">
        <div v-for="n in lineCount" :key="n">{{ n }}</div>
      </div>
      <!-- Code lines -->
      <pre class="py-2 px-4 text-github-text overflow-visible flex-grow"><code>{{ rawContent }}</code></pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { FileText, Copy, Check } from '@lucide/vue'
import type { BlobContent } from '../types'

const props = defineProps<{
  filePath: string
  blob: BlobContent
}>()

const copied = ref(false)

const isBinary = computed(() => {
  return props.blob?.is_binary ?? (props.blob as any)?.IsBinary ?? false
})

const rawContent = computed(() => {
  return props.blob?.content ?? (props.blob as any)?.Content ?? ''
})

const fileName = computed(() => {
  const parts = props.filePath.split('/')
  return parts[parts.length - 1]
})

const lineCount = computed(() => {
  if (!rawContent.value) return 1
  return rawContent.value.split('\n').length
})

function formatSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

async function copyContent() {
  if (!rawContent.value) return
  await navigator.clipboard.writeText(rawContent.value)
  copied.value = true
  setTimeout(() => {
    copied.value = false
  }, 2000)
}
</script>
