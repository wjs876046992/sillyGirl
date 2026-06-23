#!/bin/bash
# sillyGirl Test Deployment Script
# Usage: scp deploy-test.sh pagermaid@192.168.1.12:/tmp/ && ssh pagermaid@192.168.1.12 "bash /tmp/deploy-test.sh /home/pagermaid/docker/sillyplus v2.1.6"
#
# Or use inline:
# ssh pagermaid@192.168.1.12 "bash -s" < deploy-test.sh /home/pagermaid/docker/sillyplus v2.1.6

set -euo pipefail

TEST_DIR="${1:-/home/pagermaid/docker/sillyplus}"
TAG="${2:-}"
BINARY_NAME="sillyGirl_linux_amd64"
REMOTE_BINARY="${TEST_DIR}/${BINARY_NAME}"
TEMP_BINARY="/tmp/${BINARY_NAME}.${TAG}"
BACKUP_BINARY="${REMOTE_BINARY}.backup.$(date +%s)"

echo "==> 部署 sillyGirl ${TAG} -> ${REMOTE_BINARY}"

# Verify binary
if [ ! -f "$TEMP_BINARY" ]; then
  echo "ERROR: Binary not found at $TEMP_BINARY"
  echo "Make sure you've uploaded it first:"
  echo "  scp /path/to/sillyGirl_linux_amd64 pagermaid@192.168.1.12:/tmp/${BINARY_NAME}.${TAG}"
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
pm2 stop sillygirl 2>/dev/null || true
sleep 2

# Replace binary
chmod +x "$TEMP_BINARY"
mv "$TEMP_BINARY" "$REMOTE_BINARY"
echo "==> 二进制已替换"

# Start
pm2 startOrRestart sillygirl
sleep 5

# Health check
STATUS=$(pm2 describe sillygirl --no-style 2>/dev/null | grep -c "online" || echo "0")
if [ "$STATUS" -eq 0 ]; then
  echo "ERROR: 进程不健康！回滚中..."
  if [ -f "$BACKUP_BINARY" ]; then
    mv "$BACKUP_BINARY" "$REMOTE_BINARY"
    pm2 restart sillygirl
    echo "==> 回滚完成"
  fi
  exit 1
fi

echo "==> 部署完成！"
pm2 status sillygirl 2>/dev/null || true

# Cleanup backup on success
rm -f "$BACKUP_BINARY" 2>/dev/null || true
echo "==> 备份已清理"
