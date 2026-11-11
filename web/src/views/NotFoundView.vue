<template>
  <div class="max-w-md mx-auto text-center py-20 px-4">
    <h1 class="text-6xl font-extrabold text-github-blue mb-4">404</h1>
    <h2 class="text-lg font-semibold text-white mb-2">Page Not Found</h2>
    <p class="text-xs text-github-muted mb-6">The repository or page you are looking for does not exist.</p>
    <router-link
      to="/"
      class="inline-block px-4 py-2 bg-github-subtle hover:bg-github-border border border-github-border text-white rounded text-xs font-semibold transition-colors"
    >
      &larr; Back to Dashboard
    </router-link>
  </div>
</template>
