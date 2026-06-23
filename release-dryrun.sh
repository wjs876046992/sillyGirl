#!/bin/bash
# sillyGirl Release Dry Run
# Run from /Users/hermanwu/Work/herman/sillygirl/sillyGirl

set -euo pipefail

CWD="/Users/hermanwu/Work/herman/sillygirl/sillyGirl"
cd "$CWD"

# ─── Phase 0: Pre-flight ───────────────────────────────────────────────────

echo "🔍 预检"
echo "---"

echo "检查 gh CLI..."
if command -v gh >/dev/null 2>&1; then
  echo "✓ gh CLI 已安装: $(gh --version | head -1)"
  echo "检查 gh 登录状态..."
  gh auth status 2>&1 | head -5 || echo "⚠ gh 未登录"
else
  echo "✗ gh CLI 未安装"
fi

echo "检查远程仓库..."
REMOTE=$(git remote get-url origin)
echo "remote: $REMOTE"

echo "检查当前分支..."
BRANCH=$(git branch --show-current)
echo "分支: $BRANCH"

echo "检查工作区..."
STATUS=$(git status --porcelain | wc -l | tr -d '[:space:]')
if [ "$STATUS" = "0" ]; then
  echo "✓ 工作区: 干净"
else
  echo "⚠ 工作区有 $STATUS 个未跟踪/未提交文件"
  git status --porcelain | head -20
fi

echo "检查远程可达..."
if git ls-remote --exit-code origin HEAD >/dev/null 2>&1; then
  echo "✓ 远程可达"
else
  echo "✗ 远程不可达"
fi

# ─── Phase 1: 版本确定 ──────────────────────────────────────────────────────

echo ""
echo "🔢 版本确定"
echo "---"

LAST_TAG=$(git tag -l 'v2.1.*' --sort=-v:refname | awk 'index($0, "-dev") == 0' | head -1 | tr -d '[:space:]')
echo "上一版本: $LAST_TAG"

COMMIT_COUNT=$(git log "$LAST_TAG"..HEAD --oneline --no-merges | wc -l | tr -d '[:space:]')
echo "新提交数: $COMMIT_COUNT"

echo "提交列表:"
git log "$LAST_TAG"..HEAD --pretty=format:"  %h %s" --no-merges
echo ""

FEAT_COUNT=$(git log "$LAST_TAG"..HEAD --pretty=format:"%s" --no-merges | awk '/^feat/{c++} END{print c+0}')
FIX_COUNT=$(git log "$LAST_TAG"..HEAD --pretty=format:"%s" --no-merges | awk '/^fix/{c++} END{print c+0}')
FEAT_COUNT=${FEAT_COUNT:-0}
FIX_COUNT=${FIX_COUNT:-0}
echo "feat: $FEAT_COUNT, fix: $FIX_COUNT"

# Calculate version
LTM=$(echo "$LAST_TAG" | cut -d. -f1 | tr -d v)
LTN=$(echo "$LAST_TAG" | cut -d. -f2)
LTP=$(echo "$LAST_TAG" | cut -d. -f3)
NEW_VERSION="v${LTM}.${LTN}.$((LTP + 1))"
BUMP_SOURCE="auto-patch"

echo "→ 新版本: $NEW_VERSION (来源: $BUMP_SOURCE)"

# ─── Phase 2: Changelog ────────────────────────────────────────────────────

echo ""
echo "📝 生成 Release Notes"
echo "---"

BODY_FILE="/tmp/release_notes_${NEW_VERSION}.md"

# Build changelog
{
  echo "## 版本更新"
  echo ""

  echo "### ✨ 新增功能"
  echo ""
  git log "$LAST_TAG"..HEAD --pretty=format:"%h|%s" --no-merges | awk -F'|' '$2 ~ /^feat/ {print}' | while IFS='|' read -r hash msg; do
    short=$(echo "$hash" | cut -c1-7)
    echo "- $msg ($short)"
  done
  echo ""

  echo "### 🐛 缺陷修复"
  echo ""
  git log "$LAST_TAG"..HEAD --pretty=format:"%h|%s" --no-merges | awk -F'|' '$2 ~ /^fix/ {print}' | while IFS='|' read -r hash msg; do
    short=$(echo "$hash" | cut -c1-7)
    echo "- $msg ($short)"
  done
  echo ""

  echo "### 🔧 维护更新"
  echo ""
  git log "$LAST_TAG"..HEAD --pretty=format:"%h|%s" --no-merges | awk -F'|' '$2 ~ /^chore/ {print}' | while IFS='|' read -r hash msg; do
    short=$(echo "$hash" | cut -c1-7)
    echo "- $msg ($short)"
  done
  echo ""

  echo "### 📝 其他更新"
  echo ""
  git log "$LAST_TAG"..HEAD --pretty=format:"%h|%s" --no-merges | awk -F'|' '$2 !~ /^feat/ && $2 !~ /^fix/ && $2 !~ /^chore/ && $1 != "" {print}' | while IFS='|' read -r hash msg; do
    short=$(echo "$hash" | cut -c1-7)
    echo "- $msg ($short)"
  done
  echo ""

  echo "### 🐳 Docker 镜像"
  echo "- \`ntwck/sillygirl:latest\`"
  echo "- \`ntwck/sillygirl:${NEW_VERSION}\`"
  echo ""
  echo "### 📦 支持平台"
  echo "- **Linux**: amd64 / arm64"
  echo "- **macOS**: amd64 / arm64 (Intel + Apple Silicon)"
  echo "- **Windows**: amd64"
  echo ""
  echo "---"
  echo ""
  echo "### 📊 变更清单"
  echo ""
  git diff --no-merges --shortstat "$LAST_TAG"..HEAD
} > "$BODY_FILE"

cat "$BODY_FILE"

# ─── Phase 3: Dry-run summary ──────────────────────────────────────────────

echo ""
echo "========== DRY RUN 完整结果 =========="
echo ""
echo "📋 版本: $NEW_VERSION"
echo "📋 Bump 来源: $BUMP_SOURCE"
echo ""
echo "📝 预检: 已完成"
echo "🔢 版本确定: $NEW_VERSION"
echo "📝 Changelog: 已生成 ($BODY_FILE)"
echo ""
echo "🏷️ [DRY RUN] git tag -a $NEW_VERSION -m \"Release $NEW_VERSION\""
echo "🏷️ [DRY RUN] git push origin v2.1"
echo "🏷️ [DRY RUN] git push origin $NEW_VERSION"
echo ""
echo "🚀 [DRY RUN] gh workflow run build.yml --ref $NEW_VERSION --field release_tag=$NEW_VERSION"
echo ""
echo "⏳ [DRY RUN] 等待 CI 完成（最多 30 分钟）"
echo ""
echo "📋 [DRY RUN] gh release edit $NEW_VERSION --notes-file $BODY_FILE"
echo ""
echo "🖥️ [DRY RUN] gh release download $NEW_VERSION --pattern 'sillyGirl_linux_amd64' -O /tmp/sillyGirl_linux_amd64"
echo "🖥️ [DRY RUN] scp /tmp/sillyGirl_linux_amd64 pagermaid@192.168.1.12:/tmp/sillyGirl_linux_amd64.$NEW_VERSION"
echo "🖥️ [DRY RUN] ssh pagermaid@192.168.1.12 \"bash -s\" < deploy-test.sh /home/pagermaid/docker/sillyplus $NEW_VERSION"
echo ""
echo "========== 没有实际执行任何操作 =========="
