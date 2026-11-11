---
agent: frontend-developer
scope: project
updated: 2026-11-11
max_lines: 150
---

# Knowledge: @frontend-developer

## 1. Constraints (絶対遵守・禁止事項)
- **[C-01] ビルド成果物の Git 追跡禁止** (2026-11-11)
  - `web/dist/` および `internal/web/dist/` は `.gitignore` 対象であり、絶対にGitコミットに含めない。

## 2. Decisions (設計・技術選定の決定事項)
- **[D-01] Node.js / fnm 環境と Go への静的アセット埋め込み** (2026-11-11)
  - **決定**: フロントエンドは `fnm`（Node v26系）環境で `npm run build` を実行し、生成された静的ファイルを `internal/web/dist/` に出力して Go の `embed.FS` で単一バイナリに同梱する。
  - **理由**: デプロイ時の外部依存を排除し、単一バイナリによる自己完結な運用を実現するため。
- **[D-02] リポジトリストレージ情報の表示撤廃** (2026-11-11)
  - **決定**: リポジトリ一覧カード（`HomeView.vue`）から `Storage`（`repo.storage`）および `Cached` バッジの表示を削除。
  - **理由**: リポジトリストレージがローカルに一本化され、ストレージ種別やS3キャッシュ状態の表示が不要となったため。

## 3. Pitfalls (踏んだ地雷・既知の不具合と回避策)
- **[P-01] branches API レスポンスの型不整合** (2026-11-11)
  - **事象**: バックエンドが `BranchInfo` オブジェクト配列を返却する場合に、Vue 側で文字列として扱うと画面に JSON がそのままレンダリングされる。
  - **対策**: `branchList` computed で `typeof b === 'string' ? b : b.name` によりブランチ名を安全に抽出する。

## 4. Patterns (確立された実装パターン)
- **[PT-01] Vue Router を利用した動的タイトル制御** (2026-11-11)
  - `router.afterEach` グローバルフックで、リポジトリ詳細時は `${repoPath} - Gitina`、一覧画面は `Gitina` に更新する。
