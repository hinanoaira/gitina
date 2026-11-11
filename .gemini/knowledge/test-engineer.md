---
agent: test-engineer
scope: project
updated: 2026-11-11
max_lines: 150
---

# Knowledge: @test-engineer

## 1. Constraints (絶対遵守・禁止事項)
- **[C-01]** 単体テスト・結合テストは外部環境（ホストのgitconfigやネットワーク）に依存せず自己完結で実行可能にすること。 (2026-11-11)

## 2. Decisions (設計・技術選定の決定事項)
- **[D-01] テスト内Gitリポジトリの分離実行** (2026-11-11)
  - **決定**: テストケースごとに `os.MkdirTemp` で一時ディレクトリを確保し、`git init --bare` と一時作業ツリーでリポジトリを生成・破棄する。
  - **理由**: テスト間の副作用を排除し、並列テスト実行時の競合を防ぐため。

## 3. Pitfalls (踏んだ地雷・既知の不具合と回避策)
- **[P-01] テスト環境における Git GPG 署名エラー** (2026-11-11)
  - **事象**: テスト内で `git commit` を実行した際、ホストのグローバル gitconfig の GPG 署名設定（`gpg.exe` 等）が呼び出されて exit status 128 で失敗する。
  - **対策**: テスト用コマンド実行環境変数に `GIT_CONFIG_GLOBAL=/dev/null` および `GIT_CONFIG_SYSTEM=/dev/null` を指定してホスト設定を遮断する。

## 4. Patterns (確立された実装パターン)
- **[PT-01] ベアリポジトリと LFS オブジェクトのモック生成パターン** (2026-11-11)
  - `git init --bare` で一時ベアリポジトリを作成し、一時 workdir からクローン・コミット・プッシュしてテストフィクスチャを構築する。
