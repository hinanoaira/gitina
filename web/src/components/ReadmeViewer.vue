<template>
  <div class="border border-github-border rounded-md overflow-hidden bg-github-dark mt-6">
    <div class="bg-github-subtle px-4 py-2.5 border-b border-github-border flex items-center space-x-2 text-xs font-mono font-semibold text-white">
      <BookOpen class="w-4 h-4 text-github-blue" />
      <span>{{ title }}</span>
    </div>
    <div class="p-6 text-sm text-github-text markdown-body" v-html="renderedHtml"></div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { marked } from 'marked'
import { BookOpen } from '@lucide/vue'

const props = defineProps<{
  title: string
  content: string
}>()

const renderedHtml = computed(() => {
  const c = props.content || (props as any).Content || ''
  if (!c) return ''
  return marked.parse(c, { gfm: true, breaks: true })
})
</script>

<style>
.markdown-body h1 {
  font-size: 1.75rem;
  font-weight: 700;
  border-bottom: 1px solid #30363d;
  padding-bottom: 0.5rem;
  margin-top: 1.5rem;
  margin-bottom: 1rem;
  color: #ffffff;
}
.markdown-body h2 {
  font-size: 1.4rem;
  font-weight: 600;
  border-bottom: 1px solid #30363d;
  padding-bottom: 0.3rem;
  margin-top: 1.25rem;
  margin-bottom: 0.75rem;
  color: #ffffff;
}
.markdown-body h3 {
  font-size: 1.15rem;
  font-weight: 600;
  margin-top: 1rem;
  margin-bottom: 0.5rem;
  color: #ffffff;
}
.markdown-body p {
  margin-bottom: 1rem;
  line-height: 1.6;
}
.markdown-body code {
  background-color: #161b22;
  border: 1px solid #30363d;
  padding: 0.2rem 0.4rem;
  border-radius: 4px;
  font-size: 0.85em;
  font-family: monospace;
}
.markdown-body pre {
  background-color: #010409;
  border: 1px solid #30363d;
  padding: 1rem;
  border-radius: 6px;
  overflow-x: auto;
  margin-bottom: 1rem;
}
.markdown-body pre code {
  background: transparent;
  border: none;
  padding: 0;
}
.markdown-body ul, .markdown-body ol {
  padding-left: 2rem;
  margin-bottom: 1rem;
}
.markdown-body ul {
  list-style-type: disc;
}
.markdown-body ol {
  list-style-type: decimal;
}
.markdown-body blockquote {
  border-left: 4px solid #30363d;
  padding-left: 1rem;
  color: #8b949e;
  margin-bottom: 1rem;
}
.markdown-body hr {
  border: none;
  border-top: 1px solid #30363d;
  margin: 1.5rem 0;
}
.markdown-body table {
  width: 100%;
  border-collapse: collapse;
  margin-bottom: 1rem;
}
.markdown-body th, .markdown-body td {
  border: 1px solid #30363d;
  padding: 0.5rem 0.75rem;
}
.markdown-body th {
  background-color: #161b22;
}
</style>
