import type { UserInfo, RepoDetail, TreeResponse, BlobResponse, CommitsResponse, TokenResponse, TokenItem, ReposResponse } from './types'

async function request<T>(url: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(url, {
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
    ...options,
  })

  if (!res.ok) {
    let msg = `HTTP ${res.status}: ${res.statusText}`
    try {
      const err = await res.json()
      if (err.error) msg = err.error
    } catch {
      // ignore
    }
    throw new Error(msg)
  }

  return res.json()
}

export const api = {
  getAuthMe(): Promise<UserInfo> {
    return request<UserInfo>('/api/v1/auth/me')
  },

  getRepos(folder: string = ''): Promise<ReposResponse> {
    const q = new URLSearchParams()
    if (folder) q.set('folder', folder)
    return request<ReposResponse>(`/api/v1/repos?${q.toString()}`)
  },

  updateRepoVisibility(repoPath: string, visibility: string): Promise<{ visibility: string; is_public: boolean }> {
    return request<{ visibility: string; is_public: boolean }>(`/api/v1/repos/${repoPath}/settings/visibility`, {
      method: 'POST',
      body: JSON.stringify({ visibility }),
    })
  },

  updateFolderVisibility(folderPath: string, visibility: string): Promise<{ visibility: string; is_public: boolean }> {
    return request<{ visibility: string; is_public: boolean }>(`/api/v1/folders/visibility`, {
      method: 'POST',
      body: JSON.stringify({ path: folderPath, visibility }),
    })
  },

  addFolderCollaborator(folderPath: string, username: string, role: string): Promise<{ collaborators: Record<string, string> }> {
    return request<{ collaborators: Record<string, string> }>(`/api/v1/folders/collaborators`, {
      method: 'POST',
      body: JSON.stringify({ path: folderPath, username, role }),
    })
  },

  deleteFolderCollaborator(folderPath: string, username: string): Promise<{ collaborators: Record<string, string> }> {
    return request<{ collaborators: Record<string, string> }>(`/api/v1/folders/collaborators`, {
      method: 'DELETE',
      body: JSON.stringify({ path: folderPath, username }),
    })
  },

  getRepoInfo(repoPath: string): Promise<RepoDetail> {
    return request<RepoDetail>(`/api/v1/repos/${repoPath}`)
  },

  getTree(repoPath: string, ref: string, path: string = ''): Promise<TreeResponse> {
    const q = new URLSearchParams()
    if (ref) q.set('ref', ref)
    if (path) q.set('path', path)
    return request<TreeResponse>(`/api/v1/repos/${repoPath}/tree?${q.toString()}`)
  },

  getBlob(repoPath: string, ref: string, path: string): Promise<BlobResponse> {
    const q = new URLSearchParams({ ref, path })
    return request<BlobResponse>(`/api/v1/repos/${repoPath}/blob?${q.toString()}`)
  },

  getCommits(repoPath: string, ref: string): Promise<CommitsResponse> {
    const q = new URLSearchParams()
    if (ref) q.set('ref', ref)
    return request<CommitsResponse>(`/api/v1/repos/${repoPath}/commits?${q.toString()}`)
  },

  getTokens(repoPath: string): Promise<{ tokens: TokenItem[] }> {
    return request<{ tokens: TokenItem[] }>(`/api/v1/repos/${repoPath}/tokens`)
  },

  generateToken(repoPath: string, name: string, days: string, scope: string): Promise<TokenResponse> {
    return request<TokenResponse>(`/api/v1/repos/${repoPath}/tokens`, {
      method: 'POST',
      body: JSON.stringify({ name, days, scope }),
    })
  },

  revokeToken(repoPath: string, id: string): Promise<{ status: string }> {
    return request<{ status: string }>(`/api/v1/repos/${repoPath}/tokens?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
  },

  updateVisibility(repoPath: string, isPublic: boolean): Promise<{ is_public: boolean }> {
    return request<{ is_public: boolean }>(`/api/v1/repos/${repoPath}/settings/visibility`, {
      method: 'POST',
      body: JSON.stringify({ is_public: isPublic }),
    })
  },

  addCollaborator(repoPath: string, username: string, role: string): Promise<{ collaborators: Record<string, string> }> {
    return request<{ collaborators: Record<string, string> }>(`/api/v1/repos/${repoPath}/settings/collaborators`, {
      method: 'POST',
      body: JSON.stringify({ username, role }),
    })
  },

  deleteCollaborator(repoPath: string, username: string): Promise<{ collaborators: Record<string, string> }> {
    return request<{ collaborators: Record<string, string> }>(`/api/v1/repos/${repoPath}/settings/collaborators`, {
      method: 'DELETE',
      body: JSON.stringify({ username }),
    })
  },

  deleteRepo(repoPath: string, confirmName: string, force: boolean): Promise<{ status: string }> {
    return request<{ status: string }>(`/api/v1/repos/${repoPath}`, {
      method: 'DELETE',
      body: JSON.stringify({ confirm_name: confirmName, force }),
    })
  },
}
