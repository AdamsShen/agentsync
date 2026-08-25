#!/usr/bin/env bash
# agentsync 一键安装脚本：下载对应平台的预编译二进制到 ~/.local/bin
# 用法：curl -fsSL https://raw.githubusercontent.com/xmly/agentsync/main/install.sh | bash
set -euo pipefail

REPO="xmly/agentsync"
BIN="agentsync"
INSTALL_DIR="${AGENTSYNC_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${AGENTSYNC_VERSION:-latest}"   # 默认 latest，可指定 v0.1.0

# 1. 检测平台
case "$(uname -s)" in
  Darwin) goos="darwin" ;;
  Linux)  goos="linux" ;;
  *)      echo "不支持的系统: $(uname -s)（当前仅支持 macOS / Linux）" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64)  goarch="amd64" ;;
  arm64|aarch64) goarch="arm64" ;;
  *)             echo "不支持的架构: $(uname -m)" >&2; exit 1 ;;
esac

# 2. 下载
url="https://github.com/${REPO}/releases/${VERSION}/download/${BIN}_${goos}_${goarch}"
echo "下载 $url ..."
mkdir -p "$INSTALL_DIR"
tmp="$(mktemp)"
curl -fsSL "$url" -o "$tmp" || {
  rm -f "$tmp"
  echo "下载失败：可能尚未发布 ${VERSION} 的 ${goos}/${goarch} 二进制。" >&2
  echo "可先用 go install 安装：go install github.com/${REPO}@latest" >&2
  exit 1
}

# 3. 安装
install -m 755 "$tmp" "$INSTALL_DIR/$BIN"
rm -f "$tmp"
echo "✓ 已安装到 $INSTALL_DIR/$BIN"

# 4. 检查 PATH
if ! printf '%s\n' "$PATH" | tr ':' '\n' | grep -qxF "$INSTALL_DIR"; then
  echo "⚠ $INSTALL_DIR 不在 PATH 中，请将其加入 PATH："
  echo "    export PATH=\"$INSTALL_DIR:\$PATH\""
fi

# 5. 使用提示
echo
echo "使用方式："
echo "  agentsync daemon     # 启动守护进程（收敛 + 分发）"
echo "  agentsync install    # 注册开机自启服务"
echo "  agentsync status     # 查看检测到的 agent"
echo "  agentsync tui        # 交互式状态面板"
