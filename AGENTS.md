# AGENTS.md (Gitina AI 開発体制規約)

本プロジェクト（Gitina）におけるAIエージェントおよびサブエージェント運用のための基本方針・規約です。
Antigravity IDEで作業するエージェントは、本ドキュメントおよび `.agents/agents/` 配下のサブエージェント定義に従ってください。

---

## 1. サブエージェント体制と役割分担

タスクの特性に応じて、以下のサブエージェントを適切に起動・連携させて自立的かつ効率的に作業を進めてください。

### 実装チーム（Implementation）
- **`code-architect`** (`.agents/agents/code-architect.md`):
  - 要件分析、全体アーキテクチャ設計、インターフェース仕様定義、詳細タスク分解を担当。コード実装前に設計を固める。
- **`feature-developer`** (`.agents/agents/feature-developer.md`):
  - Go言語によるバックエンド/コアロジック（`cmd/`, `internal/`）やAPIの実装・リファクタリングを担当。
- **`frontend-developer`** (`.agents/agents/frontend-developer.md`):
  - Web UI（`web/` 配下のテンプレート、HTML/CSS/JS等）の実装・スタイリングを担当。
- **`test-engineer`** (`.agents/agents/test-engineer.md`):
  - 単体テスト・結合テストの作成・拡充、エッジケース網羅を担当。

### デバッグ・品質チーム（Debugging & QA）
- **`debug-investigator`** (`.agents/agents/debug-investigator.md`):
  - エラーログやテスト失敗の原因調査・トリアージを担当。**コードは改変せず**、根本原因と再現条件、修正方針の特定に専念する。
- **`patch-developer`** (`.agents/agents/patch-developer.md`):
  - 調査結果に基づき、余計なリファクタリングを排した**最小限かつ安全なピンポイント修正**を担当。
- **`qa-verifier`** (`.agents/agents/qa-verifier.md`):
  - 修正・実装後のビルド確認、全テスト実行（`go test ./...`）を行い、リグレッションの有無を客観的に判定（PASS/FAIL）する。

---

## 2. 連携ワークフロー

### 機能開発サイクル
1. `code-architect` で設計とタスク一覧を策定
2. `feature-developer` および `frontend-developer` でタスクごとに実装
3. `test-engineer` でテストコードを作成しパスを確認
4. `qa-verifier` で全体ビルドおよび既存テストを含む全テストの通過を確認

### 不具合修正サイクル
1. `debug-investigator` でログ・テスト失敗から根本原因（ファイル・行番号）を特定
2. `patch-developer` で副作用を起こさない最小限の修正パッチを適用
3. `qa-verifier` で検証。FAILの場合は理由を添えて `patch-developer` に再修正を要求

---

## 3. Git コミット規約（必須遵守）

Gitコミットを行う際は、以下のルールを厳守すること。

1. **コミットの粒度**:
   - 変更履歴を追いやすくするため、関連する変更単位ごとになるべく細かく分割してコミットすること。
   - 複数の無関係な修正・リファクタリングを1つのコミットにまとめないこと。
2. **コミットメッセージの言語**:
   - すべて日本語で記述すること。
3. **コミットメッセージの構成**:
   - 1行目に要約（プレフィックス付き: `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `style:` 等）を記述すること。
   - 1行空けた後の本文（詳細）は、箇条書きで記述すること。
4. **記述スタイル**:
   - 詳細の箇条書きは体言止めを用い、冗長な表現を避けて簡潔にまとめること。

---

## 4. プロジェクト基本コマンド

- **ビルド**: `go build -v ./...` または `make build`
- **テスト**: `go test -v ./...` または `make test`
