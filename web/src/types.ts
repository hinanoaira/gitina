export interface UserInfo {
  user: string | null
  is_admin: boolean
  base_url: string
}

export interface RepoSummary {
  path: string
  disk_size: number
  updated_at: string
  is_cached: boolean
  storage: string
  is_public: boolean
  can_read: boolean
  can_write: boolean
}

export interface RepoDetail {
  path: string
  is_public: boolean
  can_read: boolean
  can_write: boolean
  is_admin: boolean
  default_branch: string
  branches: string[]
  clone_url: string
  collaborators?: Record<string, string>
}

export interface TreeEntry {
  mode: string
  type: string
  hash?: string
  sha?: string
  name: string
  path?: string
  size: number
}

export interface CommitInfo {
  hash: string
  short_hash: string
  author_name: string
  author_email: string
  author_date: string
  committer_name: string
  committer_email: string
  committer_date: string
  message: string
}

export interface BlobContent {
  size: number
  is_binary: boolean
  content: string
}

export interface TreeResponse {
  ref: string
  path: string
  entries: TreeEntry[]
  readme?: BlobContent | null
  latest_commit?: CommitInfo | null
}

export interface BlobResponse {
  ref: string
  blob: BlobContent
  latest_commit?: CommitInfo | null
}

export interface CommitsResponse {
  ref: string
  commits: CommitInfo[]
}

export interface TokenResponse {
  token: string
  scope: string
  expires_at: string | null
}
