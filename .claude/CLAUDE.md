# sillyGirl Project Memory

## 项目概述

Go 语言 IM 机器人框架，支持 Telegram/微信/QQ 等平台。核心在 `core/` 目录，插件通过 proto3 编译后嵌入。

## 两种部署模式

### 模式 A：本地开发自测

```
make build → scp sillyplus → deploy-test.sh → pm2 restart
```

- 本地 `make build` 产物名：`sillyplus`
- scp 直接上传，无需重命名
- 快速迭代用

### 模式 B：正式发布（GitHub Workflow）

```
git tag → gh workflow run → wait CI → gh release download → scp → deploy-test.sh → pm2 restart → 清理 dev releases/tags
```

- CI 产物名：`sillyGirl_linux_amd64`（带平台后缀）
- 下载后需重命名为 `sillyplus` 再上传
- `release.js` 自动完成全流程，支持 `--dry-run`

## 触发约定

| 用户说 | 执行 |
|--------|------|
| `部署测试`、`自测一下`、`本地部署` | 模式 A |
| `发版`、`发布 vX.X.X`、`发布新版本` | 模式 B |
| `预演发布`、`dry-run` | 模式 B dry-run |

## 关键配置

| 项目 | 值 |
|------|-----|
| 本地二进制名 | `sillyplus` |
| CI 产物名 | `sillyGirl_linux_amd64` |
| PM2 进程名 | `sillyplus` |
| 测试服务器 | `pagermaid@192.168.1.12` |
| 部署目录 | `/home/pagermaid/docker/sillyplus` |
| 远程仓库 | `git@github.com:wjs876046992/sillyGirl.git` |
| 发布分支 | `v2.1` |
| CI 触发 | 仅 `workflow_dispatch`（手动） |
| 健康检查 | sleep 25（服务器有 20s 延迟启动机制，不要改） |
| PM2 路径 | `which pm2` 优先，fallback 硬编码 `/home/pagermaid/.nvm/versions/node/v24.13.0/bin/pm2` |

## 重要约定

1. **不要改 build.yml 的产物名**，只在本地构建和部署时用 `sillyplus`
2. **发版后自动清理**：删除上一稳定 tag 到新 tag 之间的所有 pre-release release + git tags
3. **PR 不触发 CI**：本地 `go build` 验证即可（代码无平台特定问题，交叉编译安全）
4. **版本规则**：feat/fix → patch+1，BREAKING CHANGE → major+1
5. **changelog 写入**：release.js 非 dry-run 模式会将 changelog 写入 `/tmp/release_notes_<版本号>.md`
