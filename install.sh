#!/usr/bin/env bash
# ==============================================================================
# gitina - Ubuntu Installation Script
# ==============================================================================
set -euo pipefail

# 1. Root privilege check
if [ "${EUID:-$(id -u)}" -ne 0 ]; then
    echo "エラー: このスクリプトは root 権限 (sudo) で実行してください。" >&2
    echo "使用例: sudo $0" >&2
    exit 1
fi

echo "=== gitina インストーラー (Ubuntu向け) を開始します ==="

# 2. OS verification
if [ -f /etc/os-release ]; then
    . /etc/os-release
    if [ "${ID:-}" != "ubuntu" ] && [ "${ID_LIKE:-}" != *"ubuntu"* ] && [ "${ID_LIKE:-}" != *"debian"* ]; then
        echo "警告: Ubuntu または Debian 以外のディストリビューションが検出されました (${NAME:-Unknown})。"
        echo "続行しますが、パッケージ管理コマンドが異なる場合があります。"
    fi
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_PATH="/usr/local/bin/gitina"
CONF_DIR="/etc/gitina"
CONF_FILE="${CONF_DIR}/config.yaml"
DATA_DIR="/var/lib/gitina"
SERVICE_FILE="/etc/systemd/system/gitina.service"

# 3. Dependencies check & installation
echo "[1/6] 依存パッケージの確認中..."
export DEBIAN_FRONTEND=noninteractive
MISSING_PKGS=()
for pkg in git curl openssl; do
    if ! command -v "$pkg" >/dev/null 2>&1; then
        MISSING_PKGS+=("$pkg")
    fi
done

if [ ${#MISSING_PKGS[@]} -gt 0 ]; then
    echo "必要なパッケージをインストールします: ${MISSING_PKGS[*]}"
    apt-get update -y
    apt-get install -y "${MISSING_PKGS[@]}"
fi

# 4. Build or locate binary
echo "[2/6] バイナリの準備中..."
if [ -f "${SCRIPT_DIR}/gitina" ]; then
    echo "カレントディレクトリの既存バイナリを使用します: ${SCRIPT_DIR}/gitina"
    cp "${SCRIPT_DIR}/gitina" "${BIN_PATH}"
elif command -v go >/dev/null 2>&1; then
    echo "Goコンパイラを検出しました。ソースからビルドします..."
    (cd "${SCRIPT_DIR}" && go build -o "${BIN_PATH}" ./cmd/gitina)
else
    echo "エラー: 'gitina' バイナリが見つからず、'go' コマンドもインストールされていません。" >&2
    echo "以下のいずれかを実行してください:" >&2
    echo "  1) あらかじめ手元で 'go build -o gitina ./cmd/gitina' を実行してスクリプトと同じ場所に配置する" >&2
    echo "  2) 'sudo apt-get install -y golang' で Go をインストールしてから再実行する" >&2
    exit 1
fi

chmod 755 "${BIN_PATH}"
echo "バイナリを配置しました: ${BIN_PATH}"

# 5. Service user & groups
echo "[3/6] システムユーザー (git) の設定中..."
if ! getent group git >/dev/null 2>&1; then
    groupadd --system git
    echo "システムグループ 'git' を作成しました。"
fi

if ! id -u git >/dev/null 2>&1; then
    useradd --system --gid git --shell /usr/sbin/nologin --home-dir "${DATA_DIR}" --no-create-home git
    echo "システムユーザー 'git' を作成しました。"
fi

# 6. Directories setup
echo "[4/6] データディレクトリの作成と権限設定中..."
mkdir -p "${DATA_DIR}/data/repositories"
mkdir -p "${DATA_DIR}/data/lfs"
mkdir -p "${DATA_DIR}/.trash"
chown -R git:git "${DATA_DIR}"
chmod 750 "${DATA_DIR}"

# 7. Configuration setup
echo "[5/6] 設定ファイルの準備中..."
mkdir -p "${CONF_DIR}"
chown root:git "${CONF_DIR}"
chmod 750 "${CONF_DIR}"

if [ -f "${CONF_FILE}" ]; then
    echo "既存の設定ファイルが検出されたため保持します: ${CONF_FILE}"
else
    if [ -f "${SCRIPT_DIR}/config.example.yaml" ]; then
        cp "${SCRIPT_DIR}/config.example.yaml" "${CONF_FILE}"
    else
        cat << 'EOF' > "${CONF_FILE}"
server:
  listen: ":8080"
  base_url: "https://git.example.com"
  repo_root: "/var/lib/gitina/data/repositories"
  jwt_secret: "CHANGEME"

repository_storage:
  type: "local"

lfs:
  storage: "local"
  local:
    path: "/var/lib/gitina/data/lfs"

github:
  client_id: "YOUR_GITHUB_CLIENT_ID"
  client_secret: "YOUR_GITHUB_CLIENT_SECRET"

admins:
  - "your_github_username"

repositories: []
EOF
    fi

    # Generate random JWT secret if placeholder exists
    RAND_SECRET=$(openssl rand -hex 32 2>/dev/null || tr -dc 'a-zA-Z0-9' </dev/urandom | head -c 64)
    sed -i "s/replace-with-a-very-long-random-secret-key/${RAND_SECRET}/" "${CONF_FILE}" || true
    sed -i "s/CHANGEME/${RAND_SECRET}/" "${CONF_FILE}" || true

    chown root:git "${CONF_FILE}"
    chmod 640 "${CONF_FILE}"
    echo "新しい設定ファイルを生成しました (ランダムJWTシークレット自動設定済み): ${CONF_FILE}"
fi

# 8. systemd service setup
echo "[6/6] systemd サービスの設定中..."
if [ -f "${SCRIPT_DIR}/gitina.service" ]; then
    cp "${SCRIPT_DIR}/gitina.service" "${SERVICE_FILE}"
else
    cat << EOF > "${SERVICE_FILE}"
[Unit]
Description=gitina Lightweight Git Server
Documentation=https://github.com/hinanoaira/gitina
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=git
Group=git
WorkingDirectory=/var/lib/gitina
ExecStart=${BIN_PATH} serve -c ${CONF_FILE}
Restart=always
RestartSec=5s
LimitNOFILE=65535
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF
fi

chmod 644 "${SERVICE_FILE}"
systemctl daemon-reload
systemctl enable gitina >/dev/null 2>&1

echo ""
echo "=============================================================================="
echo " gitina のインストールが正常に完了しました！"
echo "=============================================================================="
echo ""
echo "【次のステップ】"
echo "1. 設定ファイルを編集してください:"
echo "   sudo nano ${CONF_FILE}"
echo "   - base_url (例: https://git.hns.jp)"
echo "   - github.client_id & github.client_secret"
echo "   - admins (GitHubユーザー名)"
echo "   - 必要に応じて repository_storage や lfs の S3 設定"
echo ""
echo "2. サービスを起動してください:"
echo "   sudo systemctl start gitina"
echo ""
echo "3. 動作確認 & ログの確認:"
echo "   sudo systemctl status gitina"
echo "   journalctl -u gitina -f"
echo ""
echo "4. リポジトリ管理コマンドの確認:"
echo "   gitina repo ls"
echo "=============================================================================="
