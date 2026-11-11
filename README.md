# gitina

超軽量・高性能なプライベートGitサーバー＆リポジトリビューア。

- **超軽量＆ゼロ外部依存**: Go言語単一バイナリ設計。Node.jsや重厚なDBは一切不要。
- **Web リポジトリビューア＆管理UI内包**: ブラウザでリポジトリURLを開くだけで、ファイルツリー、ソースコード、コミット履歴の閲覧、パーソナルアクセストークン（PAT）の発行、権限管理が可能。
- **S3 Tiered Storage（階層化ストレージ）**: S3/Cloudflare R2をマスターストレージとしてリポジトリを容量無制限にホスト。ローカルSSD（20GB等）をオンデマンドなLRUキャッシュとして活用し、小容量VPSでも巨大・大量リポジトリを安定運用。
- **変幻自在なパス階層**: `/fuga.git`、`/path/to/directory.git`、`/path/to/directory/hoge.git` など任意の深さに対応。
- **初回アクセス時の自動ベアリポジトリ作成**: 初回 `git clone` や `git push` 時に自動で `git init --bare` して即座に利用可能。
- **Git Credential Manager (GCM) ブラウザ認証**: Gitea互換OAuth2フローを内包し、Git操作時にブラウザが自動起動してGitHub認証を実行。
- **GitHub OAuth & きめ細やかな認可 (ACL)**: リポジトリ単位で `public: true/false` やユーザーごとの `rw` / `ro` をYAML設定＆Web画面から動的変更可能。
- **Git LFS (Large File Storage) 完全対応**: ローカルディスク保存およびS3互換ストレージ（AWS S3 / Cloudflare R2 / MinIO 等）での Presigned URL 直接転送に対応。
- **サーバー管理CLI**: `gitina repo ls`、`gitina repo rm`、`gitina repo clean` による安全なリポジトリ管理（ゴミ箱機能つき）。

---

## 1. セットアップ

### Ubuntu / Debian (推奨: ワンクリック自動インストール)
Ubuntu 環境では付属のインストールスクリプトで、ユーザー作成・ディレクトリ構築・systemdサービス登録・設定ファイル配置を一括で自動セットアップできます：

```bash
# リポジトリ直下で実行
sudo ./install.sh
```

---

### 手動セットアップ

#### 必要要件
- Go 1.22+
- Git 2.10+ (サーバー上にインストールされていること)

#### ビルド
```bash
# Vue 3 フロントエンドと Go バイナリの一括ビルド
make build

# または Go のみビルド (事前ビルド済みアセットを使用)
make build-go
```

#### フロントエンド開発 (Dev Mode)
```bash
# Vite 開発サーバー (HMR & バックエンドAPIプロキシ)
make dev-web

# Go サーバーの起動
make dev-go
```

#### 設定ファイルの準備
```bash
cp config.example.yaml config.yaml
```

`config.yaml` を開き、以下の項目を設定します：
- `server.base_url`: 例 `https://git.hns.jp`
- `server.jwt_secret`: 署名用の十分長いランダム文字列
- `github.client_id` & `github.client_secret`: 後述のGitHub OAuth App情報
- `admins`: 全リポジトリにアクセスできる管理者のGitHubユーザー名
- `repositories`: リポジトリ単位の公開設定 (`public`) とコラボレーター設定

---

## 2. GitHub OAuth App の登録

GitHubにてOAuthアプリケーションを1つ作成します。

1. GitHubの **Settings** $\rightarrow$ **Developer settings** $\rightarrow$ **OAuth Apps** $\rightarrow$ **New OAuth App**
2. 以下の通り設定：
   - **Application name**: `gitina`（任意）
   - **Homepage URL**: `https://git.hns.jp`
   - **Authorization callback URL**: `https://git.hns.jp/login/oauth/github/callback`
3. 発行された **Client ID** と **Client Secret** を `config.yaml` に転記します。

---

## 3. サーバー起動

```bash
# デフォルトで config.yaml を読み込んで起動
./gitina

# 設定ファイルを指定して起動
./gitina serve -c /path/to/config.yaml
```

---

## 4. クライアント側（開発者PC）の操作

### Git Credential Manager (GCM) での利用
Git Credential Manager (GCM) をインストール済みの環境であれば、初回アクセス時に自動でブラウザが開いてGitHubログイン画面に遷移します。

より確実にGCMに認識させるには、手元のPCで以下の設定を1回実行してください：
```bash
git config --global credential.https://git.hns.jp.provider gitea
```

### クローン & プッシュ (任意階層・自動初期化)
サーバー上にリポジトリが存在しなくても、権限があれば初回アクセス時に自動でベアリポジトリが作られます。

```bash
# 浅いパス
git clone https://git.hns.jp/fuga.git

# 深い階層のパス
git clone https://git.hns.jp/path/to/directory.git
git clone https://git.hns.jp/path/to/directory/hoge.git

# 新規プロジェクトを直接プッシュ (初回pushで自動作成)
cd my-project
git init
git remote add origin https://git.hns.jp/my/awesome/app.git
git add .
git commit -m "Initial commit"
git push -u origin main
```

### Git LFS の利用
特別な設定は不要です。通常通り `git lfs track` してコミット＆プッシュするだけで、大容量バイナリはgitinaのLFSバックエンド（ローカルまたはS3互換ストレージ）へ自動転送されます。

```bash
git lfs install
git lfs track "*.zip" "*.mp4" "*.tar.gz"
git add .gitattributes
git add bigfile.zip
git commit -m "Add large file"
git push origin main
```

---

## 5. S3 Tiered Storage（大容量・大量リポジトリ運用）

WebARENA Indigo などの 20GB SSD マシンでも、S3 または Cloudflare R2 をマスターストレージとすることで、数百GB〜数TBのリポジトリを容量無制限でホストできます。

### 仕組み
- リポジトリ本体はすべて S3 上に `repos/<path>.bundle`（単一アーカイブ）として保存されます。
- クライアントがアクセス（clone/fetch/push）した時のみ、S3から自動ダウンロードされてローカルSSDに一時展開されます。
- `git push` が成功すると、最新の bundle が S3 に自動で同期されます。
- ローカルSSDの使用量が `cache.max_size_gb` を超えると、最近使われていないリポジトリのローカルキャッシュが安全に自動蒸発（LRUパージ）します（S3に実体があるためデータ消失ゼロ）。

---

## 6. サーバー管理コマンド (CLI)

SSHログインしたサーバー上からリポジトリの一覧や安全な削除が可能です。
設定ファイルは自動検出されるため（`./config.yaml` $\rightarrow$ `/etc/gitina/config.yaml`）、どのカレントディレクトリからでも直接コマンドを実行できます。

```bash
# ホストされているリポジトリ一覧の表示
gitina repo ls

# リポジトリの安全な削除 (デフォルトは .trash/ へ退避)
gitina repo rm path/to/directory.git

# 物理削除 (強制)
gitina repo rm -f path/to/directory.git

# ゴミ箱 (.trash/) 内の30日以上経過したリポジトリを完全消去
gitina repo clean --days 30
```

> [!TIP]
> systemd で `git` ユーザー権限で動作させている環境では、パーミッションを揃えるため `sudo -u git gitina repo ...` または `sudo gitina repo ...` で実行できます。ゴミ箱の自動作成時もリポジトリ所有者情報が保持されます。

---

## 6. システム常駐 (systemd サービス例)

`/etc/systemd/system/gitina.service`:
```ini
[Unit]
Description=gitina Lightweight Git Server
After=network.target

[Service]
Type=simple
User=git
Group=git
WorkingDirectory=/var/gitina
ExecStart=/usr/local/bin/gitina serve -c /var/gitina/config.yaml
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```
