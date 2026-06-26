---
name: sillygirl-release
description: 发布 sillyGirl 新版本的标准化流程。适用于 wjs876046992/sillyGirl 仓库的 v2.x.x 版本发布。触发条件：用户要求创建/发布新版本、打 tag、发 release。
---

# sillyGirl Release Workflow

## 分支管理规范

- **主开发分支**：`v2.1`
- **新功能开发**：从 `v2.1` 创建分支（如 `feature/xxx`），在分支上开发
- **合并发布**：开发完成后合并回 `v2.1`，随 `v2.1` 发布新版本
- **清理分支**：发布新版本后，删除已合并的开发分支（`git branch -d feature/xxx`）

> ⚠️ 开发分支生命周期：`v2.1` → `feature/xxx` → 开发 → 合并 `v2.1` → 发版 → 删除分支

## 前置条件

- 工作目录：项目根目录（sillyGirl 仓库）
- 远程仓库：`git@github.com:wjs876046992/sillyGirl.git`
- 发布分支：`v2.1`
- 测试服务器：`pagermaid@192.168.1.12`，部署目录 `/home/pagermaid/docker/sillyplus`
- 已配置 `gh` CLI 并登录
- SSH 密钥配置，可免密连接测试机

## 触发约定

| 用户说 | 执行模式 |
|--------|---------|
| `部署测试`、`自测一下`、`本地部署` | **模式 A**：本地 make build → scp → deploy-test.sh |
| `发版`、`发布 v2.1.6`、`发布新版本` | **模式 B**：tag → CI → download → scp → deploy-test.sh → 清理 dev releases/tags |
| `预演发布`、`dry-run`、`看看发布流程` | **模式 B dry-run**：只展示流程不执行 |

---

## 模式 A：本地开发自测

快速迭代用，本地 `go build` → 直接 scp 到测试服务器。

```
make build → scp sillyplus → deploy-test.sh → pm2 restart → 看日志
```

### 步骤

```bash
# 1. 本地构建（产物名：sillyplus）
make build        # 或 make run 先本地跑一下

# 2. 上传到测试服务器
scp sillyplus pagermaid@192.168.1.12:/tmp/sillyplus

# 3. 远程部署（使用 deploy-test.sh）
ssh pagermaid@192.168.1.12 "bash /tmp/deploy-test.sh <版本号>"
# 或者不用脚本，手动部署：
ssh pagermaid@192.168.1.12 '
  cd /home/pagermaid/docker/sillyplus && \
  cp /tmp/sillyplus sillyplus && \
  pm2 restart sillyplus && \
  sleep 5 && pm2 status sillyplus
'

# 4. 看日志
ssh pagermaid@192.168.1.12 "pm2 logs sillyplus --lines 30"
```

> **注意**：本地构建产物直接叫 `sillyplus`，无需重命名。

---

## 模式 B：正式发布（GitHub Workflow）

通过 CI 构建 → GitHub Release → 下载 → 部署到测试服务器。适用于正式版本发布。

```
git tag → push tag → gh workflow run → 等 CI 完成 → gh release download → scp → deploy-test.sh → pm2 restart
```

### 推荐方式：release.js 自动化

```bash
cd <项目根目录>

# 自动检测版本，完整发布 + 测试部署
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
cd <项目根目录>
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

#### Step 3: 创建 Tag 并推送

```bash
git tag -a v2.1.6 -m "Release v2.1.6"
git push origin v2.1
git push origin v2.1.6
```

#### Step 4: 触发 CI 构建

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
gh release edit v2.1.6 --notes-file /tmp/release_notes_v2.1.6.md
```

#### Step 7: 下载并部署到测试服务器

```bash
# 7a. 从 GitHub Release 下载二进制（CI 产物名带平台后缀，需重命名）
gh release download v2.1.6 --pattern 'sillyGirl_linux_amd64' -O /tmp/sillyplus

# 7b. 上传到测试服务器
scp /tmp/sillyplus pagermaid@192.168.1.12:/tmp/sillyplus.v2.1.6

# 7c. 远程部署
ssh pagermaid@192.168.1.12 '
  cd /home/pagermaid/docker/sillyplus && \
  cp /tmp/sillyplus.v2.1.6 sillyplus && \
  pm2 restart sillyplus && \
  sleep 5 && pm2 status sillyplus
'

# 或者使用 deploy-test.sh：
scp deploy-test.sh pagermaid@192.168.1.12:/tmp/
ssh pagermaid@192.168.1.12 "bash /tmp/deploy-test.sh v2.1.6"
```

> **注意**：CI 产物名是 `sillyGirl_linux_amd64`，下载后需重命名为 `sillyplus` 再上传。

#### Step 8: CI 自动清理

正式版发布后，build.yml 会自动删除所有 pre-release 的 dev 构建 + git tags。同时 release.js 也会在本地做同样的清理（双重保险）。

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

服务器端自动化部署脚本，支持备份 + 回滚 + 健康检查。详细逻辑见 `deploy-test.sh` 源码。

### 用法

```bash
# 1. 先上传二进制文件
scp sillyplus pagermaid@192.168.1.12:/tmp/sillyplus.<版本号>

# 2. 上传部署脚本
scp deploy-test.sh pagermaid@192.168.1.12:/tmp/

# 3. 执行部署
ssh pagermaid@192.168.1.12 "bash /tmp/deploy-test.sh <版本号>"
```

### 流程说明

1. 校验二进制文件（ELF 格式检查）
2. 备份当前运行的二进制
3. 停服 → 替换 → 重启（PM2 管理）
4. 等待 25 秒（服务器有 20 秒延迟启动机制）
5. 健康检查：失败则自动回滚

## Troubleshooting

| 问题 | 解决 |
|------|------|
| gh auth expired | `gh auth login` |
| 远程仓库不对 | `git remote set-url origin git@github.com:wjs876046992/sillyGirl.git` |
| CI 未触发 | 确认使用 `gh workflow run build.yml --field release_tag=v2.1.x` 手动触发 |
| 测试 SSH 失败 | `ssh -T pagermaid@192.168.1.12` 检查密钥 |
| pre-release 未清理 | `gh release list --json isPrerelease,tagName | jq '.[] | select(.isPrerelease) | .tagName'` 查看后手动删除 |

## CI Build Workflow 要点

`.github/workflows/build.yml` 仅支持手动触发：

| 触发方式 | 行为 | Release 类型 |
|---------|------|-------------|
| `workflow_dispatch` + 留空 | 自动 dev build | pre-release |
| `workflow_dispatch` + release_tag | 正式 release | release |

正式 release（`mode=update`）触发后自动清理所有 pre-release dev 构建 + git tags。
