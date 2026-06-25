#!/bin/bash
# sillyGirl Test Deployment Script
# Usage: bash deploy-test.sh <版本号>
#
# 示例: bash deploy-test.sh 1782365773
#
# 部署流程:
#   1. scp sillyGirl_linux_amd64 pagermaid@192.168.1.12:/tmp/sillyplus.<版本号>
#   2. ssh pagermaid@192.168.1.12 "bash /tmp/deploy-test.sh <版本号>"

set -euo pipefail

TEST_DIR="/home/pagermaid/docker/sillyplus"
PM2="/home/pagermaid/.nvm/versions/node/v24.13.0/bin/pm2"
BINARY_NAME="sillyplus"
VERSION="${1:-}"
REMOTE_BINARY="${TEST_DIR}/${BINARY_NAME}"
TEMP_BINARY="/tmp/${BINARY_NAME}.${VERSION}"
BACKUP_BINARY="${REMOTE_BINARY}.backup.$(date +%s)"

if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version>"
  exit 1
fi

echo "==> 部署 sillyGirl ${VERSION} -> ${REMOTE_BINARY}"

# Verify binary
if [ ! -f "$TEMP_BINARY" ]; then
  echo "ERROR: Binary not found at $TEMP_BINARY"
  echo "Make sure you've uploaded it first:"
  echo "  scp sillyGirl_linux_amd64 pagermaid@192.168.1.12:/tmp/sillyplus.${VERSION}"
  exit 1
fi

if ! file "$TEMP_BINARY" | grep -q "ELF"; then
  echo "ERROR: Not a valid ELF binary: $TEMP_BINARY"
  exit 1
fi

# Backup current binary
if [ -f "$REMOTE_BINARY" ]; then
  cp "$REMOTE_BINARY" "$BACKUP_BINARY"
  echo "==> 已备份: $BACKUP_BINARY"
else
  echo "==> 无现有二进制（首次部署）"
fi

# Graceful stop
$PM2 stop sillyplus 2>/dev/null || true
sleep 2

# Replace binary
chmod +x "$TEMP_BINARY"
mv "$TEMP_BINARY" "$REMOTE_BINARY"
echo "==> 二进制已替换"

# Start
$PM2 startOrRestart sillyplus
sleep 25

# Health check
STATUS=$($PM2 describe sillyplus --no-style 2>/dev/null | grep -c "online" || echo "0")
if [ "$STATUS" -eq 0 ]; then
  echo "ERROR: 进程不健康！回滚中..."
  if [ -f "$BACKUP_BINARY" ]; then
    mv "$BACKUP_BINARY" "$REMOTE_BINARY"
    $PM2 restart sillyplus
    echo "==> 回滚完成"
  fi
  exit 1
fi

echo "==> 部署完成！"
$PM2 status sillyplus 2>/dev/null || true

# Cleanup backup on success
rm -f "$BACKUP_BINARY" 2>/dev/null || true
echo "==> 备份已清理"
