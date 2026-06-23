---
name: sillygirl-release
description: 发布 sillyGirl 新版本的标准化流程。适用于 wjs876046992/sillyGirl 仓库的 v2.x.x 版本发布。触发条件：用户要求创建/发布新版本、打 tag、发 release。
---

# sillyGirl Release Workflow

## 前置条件

- 工作目录：`/Users/hermanwu/Work/herman/sillygirl/sillyGirl`
- 远程仓库：`git@github.com:wjs876046992/sillyGirl.git`
- 发布分支：`v2.1`
- 已配置 `gh` CLI 并登录
- SSH 密钥配置，可免密连接测试机

## 工作模式

```
代码修改流程（反复）                 发布流程（偶尔）
┌───────────────┐                ┌────────────────┐
│ 1. 建分支      │                │ 1. release.js  │
│ 2. 改代码提交  │  ←→  dev迭代   │ 2. 等 CI       │
│ 3. 自测部署    │                │ 3. 更新 release│
└───────────────┘                │ 4. 部署测试机  │
                                 └────────────────┘
```

## 代码修改流程

### 1. 创建分支

从 `v2.1` 建分支，命名规则：`fix/xxx`、`feat/xxx`、`chore/xxx`

```bash
git checkout v2.1
git checkout -b <类型>/<简短描述>
# 改代码...
git add <改动的文件>
git commit -m "<类型>: <简短描述>"
git push origin <分支名>
```

### 2. 自测

```bash
# 本地构建
make run

# 上传到测试机
scp sillyGirl pagermaid@192.168.1.12:/tmp/sillyGirl

# 部署
ssh pagermaid@192.168.1.12 '
  cd /home/pagermaid/docker/sillyplus && \
  cp /tmp/sillyGirl sillyGirl_linux_amd64 && \
  pm2 restart sillygirl && \
  pm2 logs sillygirl --lines 20
'
```

## 发布流程（使用 release.js）

### 推荐方式：Workflow 脚本

```bash
# 自动检测版本，完整发布 + 测试部署
cd /Users/hermanwu/Work/herman/sillygirl/sillyGirl
node release.js

# 强制 minor bump
node release.js --bump minor

# 指定版本
node release.js --version v2.2.0

# 跳过测试部署
node release.js --no-deploy

# 预演（不实际执行）
node release.js --dry-run
```

### 手动流程

#### Step 1: 预检

```bash
cd /Users/hermanwu/Work/herman/sillygirl/sillyGirl
gh auth status                # 确认登录
git remote get-url origin     # 确认远程仓库
git branch --show-current     # 确认在 v2.* 分支
git status --porcelain        # 确认工作区干净
git fetch --tags origin       # 拉取最新标签
```

#### Step 2: 确定版本

```bash
# 查看自上一版本以来的提交
LAST_TAG=$(git tag -l "v2.1.*" --sort=-v:refname | grep -v "-dev" | head -1)
git log "$LAST_TAG"..HEAD --oneline --no-merges

# 版本规则
# feat/fix 存在 → patch+1
# BREAKING CHANGE → major+1
# --bump minor → minor+1, patch=0
```

#### Step 3: 创建 Tag

```bash
git tag -a v2.1.6 -m "Release v2.1.6"
git push origin v2.1
git push origin v2.1.6
```

#### Step 4: 触发 CI

```bash
gh workflow run build.yml --ref v2.1.6 --field release_tag=v2.1.6
```

#### Step 5: 等待 CI 完成

```bash
# 轮询（最多 30 分钟）
RUN_ID=$(gh run list --workflow=build.yml --limit 1 --json databaseId --jq '.[0].databaseId')
while true; do
  STATUS=$(gh run view "$RUN_ID" --json status --jq '.status')
  [ "$STATUS" = "completed" ] && break
  sleep 15
done
CONCLUSION=$(gh run view "$RUN_ID" --json conclusion --jq '.conclusion')
echo "$CONCLUSION"  # success / failure / cancelled
```

#### Step 6: 更新 Release Notes

```bash
# 自动生成 changelog
gh release edit v2.1.6 --notes-file /tmp/release_notes_v2.1.6.md
```

#### Step 7: 部署到测试机

```bash
# 下载二进制
gh release download v2.1.6 --pattern 'sillyGirl_linux_amd64' -O /tmp/sillyGirl_linux_amd64

# 上传并部署
scp /tmp/sillyGirl_linux_amd64 pagermaid@192.168.1.12:/tmp/sillyGirl_linux_amd64.v2.1.6

ssh pagermaid@192.168.1.12 '
  cd /home/pagermaid/docker/sillyplus && \
  cp /tmp/sillyGirl_linux_amd64.v2.1.6 sillyGirl_linux_amd64 && \
  pm2 restart sillygirl && \
  sleep 5 && pm2 status sillygirl
'
```

#### Step 8: CI 自动清理

正式版发布后，build.yml 会自动删除所有 pre-release 的 dev 构建。

## Release Notes 模板

```
## 版本更新

### ✨ 新增功能
- 描述 (abc1234)

### 🐛 缺陷修复
- 描述 (def5678)

### 🔧 维护更新
- 描述 (9ab0123)

### 🐳 Docker 镜像
- `ntwck/sillygirl:latest`
- `ntwck/sillygirl:v2.1.6`

### 📦 支持平台
- Linux: amd64 / arm64
- macOS: amd64 / arm64
- Windows: amd64

---
### 📊 变更清单
<N> files changed, +xxx/-xxx insertions(+), -xxx deletions(-)
```

## deploy-test.sh

可选的自动化部署脚本，放在项目根目录：

```bash
#!/bin/bash
set -euo pipefail
TEST_DIR="${1:-/home/pagermaid/docker/sillyplus}"
TAG="${2:-v2.1.6}"
BINARY_NAME="sillyGirl_linux_amd64"
REMOTE_BINARY="${TEST_DIR}/${BINARY_NAME}"
TEMP_BINARY="/tmp/${BINARY_NAME}.${TAG}"
BACKUP_BINARY="${REMOTE_BINARY}.backup.$(date +%s)"

[ -f "$TEMP_BINARY" ] || { echo "ERROR: binary not found"; exit 1; }
[ -f "$REMOTE_BINARY" ] && cp "$REMOTE_BINARY" "$BACKUP_BINARY"
pm2 stop sillygirl 2>/dev/null || true; sleep 2
chmod +x "$TEMP_BINARY"
mv "$TEMP_BINARY" "$REMOTE_BINARY"
pm2 startOrRestart sillygirl
sleep 5
pm2 describe sillygirl --no-style | grep -q "online" || {
  [ -f "$BACKUP_BINARY" ] && mv "$BACKUP_BINARY" "$REMOTE_BINARY"; pm2 restart sillygirl
  exit 1
}
rm -f "$BACKUP_BINARY" 2>/dev/null || true
pm2 status sillygirl 2>/dev/null || true
```

```bash
chmod +x deploy-test.sh
# 用法：scp deploy-test.sh pagermaid@192.168.1.12:/tmp/ && ssh pagermaid@192.168.1.12 "bash /tmp/deploy-test.sh /home/pagermaid/docker/sillyplus v2.1.6"
```

## Troubleshooting

| 问题 | 解决 |
|------|------|
| gh auth expired | `gh auth login` |
| 远程仓库不对 | `git remote set-url origin git@github.com:wjs876046992/sillyGirl.git` |
| CI 未触发 | 确认 tag 推送到 v2.* 分支 |
| 测试 SSH 失败 | `ssh -T pagermaid@192.168.1.12` 检查密钥 |
| pre-release 未清理 | 手动 `gh release list --json isPrerelease,tagName | jq` |

## CI Build Workflow 要点

`.github/workflows/build.yml` 支持三种触发模式：

| 触发方式 | 行为 | Release 类型 |
|---------|------|-------------|
| push 到 v2.* | 自动 dev build | pre-release |
| workflow_dispatch + release_tag | 正式 release | release |
| pull_request | 仅构建 | 无 release |

正式 release（`mode=update`）触发后自动清理所有 pre-release dev 构建。
