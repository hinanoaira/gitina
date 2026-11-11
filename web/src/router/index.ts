import { createRouter, createWebHistory } from 'vue-router'
import HomeView from '../views/HomeView.vue'
import RepoDetailView from '../views/RepoDetailView.vue'
import NotFoundView from '../views/NotFoundView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'home',
      component: HomeView,
    },
    {
      path: '/:repoPath(.+)',
      name: 'repo',
      component: RepoDetailView,
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: NotFoundView,
    },
  ],
})

router.afterEach((to) => {
  if (to.name === 'repo' && to.params.repoPath) {
    const p = Array.isArray(to.params.repoPath) ? to.params.repoPath.join('/') : to.params.repoPath
    document.title = `${p} - Gitina`
  } else {
    document.title = 'Gitina'
  }
})

export default router
