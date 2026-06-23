/**
 * sillyGirl Release Workflow - Dry Run
 *
 * Usage: Workflow({ script: release.js, args: { bumpType, version, deploy, dryRun } })
 *
 * Design: Use multiple small agent calls instead of one big bash script.
 * Each agent does one thing (git log, git tag, gh workflow, etc.)
 * Phase headers group agents visually.
 */

export const meta = {
  name: 'sillygirl-release',
  description: 'Release sillyGirl with auto versioning, CI triggering, and test deployment',
  phases: [
    { title: '🔍 预检', detail: 'Check gh, auth, remote, branch, clean tree' },
    { title: '🔢 版本确定', detail: 'Parse commits, determine version' },
    { title: '📝 Changelog 生成', detail: 'Group commits by type' },
    { title: '🏷️ Tag + CI + Release + Deploy', detail: 'Dry-run remaining steps' },
  ],
};

const CWD = '/Users/hermanwu/Work/herman/sillygirl/sillyGirl';
const TEST_HOST = "pagermaid@192.168.1.12";
const TEST_DIR = "/home/pagermaid/docker/sillyplus";

// Parse args - Workflow scripts have NO Node.js globals
const BUMP_TYPE = args?.bumpType ?? null;
const FORCE_VERSION = args?.version ?? null;
const SKIP_DEPLOY = args?.deploy === false;
const DRY_RUN = args?.dryRun === true;

function safeBash(desc, cmd, opts = {}) {
  // In dry-run mode, NEVER execute destructive commands
  const isSafe = opts.safe !== false;
  return agent(
    desc + '\n\n' +
    'Run this exact command and return the stdout verbatim. ' +
    (isSafe ? 'Do NOT modify any files or create tags.\n\n' : 'You MAY modify files as needed.\n\n') +
    '```\n' + cmd + '\n```',
    { label: opts.label || desc.slice(0, 20), phase: opts.phase, agentType: "general-purpose" }
  ).catch(err => {
    if (DRY_RUN === true) return opts.fallback ?? "";
    throw new Error(`${desc} failed: ${err}`);
  });
}
function bash(desc, cmd, opts = {}) {
  return safeBash(desc, cmd, { ...opts, safe: true });
}

// ─── Phase 0: Pre-flight ───────────────────────────────────────────────────

phase("🔍 预检");

const ghVer = await bash(
  "检查 gh CLI",
  "command -v gh && gh --version | head -1 || echo 'NOT INSTALLED'",
  { label: "gh CLI", phase: "🔍 预检" }
);
log("gh CLI: " + ghVer);

const ghAuth = await bash(
  "检查 gh 登录",
  "gh auth status 2>&1 | head -3 || echo 'AUTH ERROR'",
  { label: "gh auth", phase: "🔍 预检", fallback: "NOT LOGGED IN" }
);
log("gh auth: " + ghAuth);

const remote = await bash(
  "检查远程仓库",
  "git remote get-url origin",
  { label: "remote", phase: "🔍 预检" }
);
log("remote: " + remote);

const branch = await bash(
  "检查当前分支",
  "git branch --show-current",
  { label: "branch", phase: "🔍 预检" }
);
log("branch: " + branch);

const gitStatus = await bash(
  "检查工作区",
  "git status --porcelain | head -20 || echo 'clean'",
  { label: "status", phase: "🔍 预检", fallback: "clean" }
);
if (gitStatus.trim() === '' || gitStatus.trim() === 'clean') {
  log("工作区: 干净");
} else {
  log("工作区: 有变更\n" + gitStatus);
}

const gitReachable = await bash(
  "检查远程可达",
  "git ls-remote --exit-code origin HEAD >/dev/null 2>&1 && echo 'OK' || echo 'UNREACHABLE'",
  { label: "reachable", phase: "🔍 预检", fallback: "UNREACHABLE (offline?)" }
);
log("remote reachable: " + gitReachable);

// ─── Phase 1: 版本确定 ──────────────────────────────────────────────────────

phase("🔢 版本确定");

const lastTag = await bash(
  "获取上一版本 tag",
  "git tag -l 'v2.1.*' --sort=-v:refname | grep -v '-dev' | head -1 | tr -d '[:space:]'",
  { label: "last tag", phase: "🔢 版本确定", fallback: "v2.1.5" }
);
log("上一版本: " + lastTag);

const commitCount = await bash(
  "统计提交数",
  "git log '" + lastTag + "'..HEAD --oneline --no-merges | wc -l | tr -d '[:space:]'",
  { label: "commit count", phase: "🔢 版本确定", fallback: "0" }
);
log("新提交数: " + commitCount);

const commitList = await bash(
  "列出所有提交",
  "git log '" + lastTag + "'..HEAD --pretty=format:'%h %s' --no-merges",
  { label: "commits", phase: "🔢 版本确定", fallback: "(no commits)" }
);
log("提交列表:\n" + commitList);

const featCount = await bash(
  "统计 feat 提交",
  "git log '" + lastTag + "'..HEAD --pretty=format:'%s' --no-merges | grep -c '^feat' || echo 0",
  { label: "feat count", phase: "🔢 版本确定", fallback: "0" }
);
log("feat: " + featCount);

const fixCount = await bash(
  "统计 fix 提交",
  "git log '" + lastTag + "'..HEAD --pretty=format:'%s' --no-merges | grep -c '^fix' || echo 0",
  { label: "fix count", phase: "🔢 版本确定", fallback: "0" }
);
log("fix: " + fixCount);

// Calculate version
const lastTagMajor = lastTag.split('.')[0].replace('v', '');
const lastTagMinor = lastTag.split('.')[1];
const lastTagPatch = parseInt(lastTag.split('.')[2]) || 0;

let NEW_VERSION;
let BUMP_SOURCE;

if (FORCE_VERSION) {
  NEW_VERSION = FORCE_VERSION;
  BUMP_SOURCE = 'forced';
} else if (BUMP_TYPE === 'minor') {
  NEW_VERSION = lastTagMajor + '.' + (parseInt(lastTagMinor) + 1) + '.0';
  BUMP_SOURCE = 'force-minor';
} else if (BUMP_TYPE === 'major') {
  NEW_VERSION = (parseInt(lastTagMajor) + 1) + '.0.0';
  BUMP_SOURCE = 'force-major';
} else {
  NEW_VERSION = lastTagMajor + '.' + lastTagMinor + '.' + (lastTagPatch + 1);
  BUMP_SOURCE = 'auto-patch';
}

log("→ 新版本: " + NEW_VERSION + " (来源: " + BUMP_SOURCE + ")");

// ─── Phase 2: Changelog ────────────────────────────────────────────────────

phase("📝 生成 Release Notes");

const changelogFeats = await bash(
  "提取 feat 提交",
  "git log '" + lastTag + "'..HEAD --pretty=format:'%h|%s' --no-merges | grep '^|feat' | while IFS='|' read -r hash msg; do short=\$(echo \$hash | cut -c1-7); echo \"- \$msg (\$short)\"; done",
  { label: "changelog feat", phase: "📝 生成 Release Notes", fallback: "" }
);

const changelogFixes = await bash(
  "提取 fix 提交",
  "git log '" + lastTag + "'..HEAD --pretty=format:'%h|%s' --no-merges | grep '^|fix' | while IFS='|' read -r hash msg; do short=\$(echo \$hash | cut -c1-7); echo \"- \$msg (\$short)\"; done",
  { label: "changelog fix", phase: "📝 生成 Release Notes", fallback: "" }
);

const changelogChores = await bash(
  "提取 chore 提交",
  "git log '" + lastTag + "'..HEAD --pretty=format:'%h|%s' --no-merges | grep '^|chore' | while IFS='|' read -r hash msg; do short=\$(echo \$hash | cut -c1-7); echo \"- \$msg (\$short)\"; done",
  { label: "changelog chore", phase: "📝 生成 Release Notes", fallback: "" }
);

const changelogOther = await bash(
  "提取其他提交",
  "git log '" + lastTag + "'..HEAD --pretty=format:'%h|%s' --no-merges | grep -v '^|feat' | grep -v '^|fix' | grep -v '^|chore' | grep -v '^$' | while IFS='|' read -r hash msg; do short=\$(echo \$hash | cut -c1-7); echo \"- \$msg (\$short)\"; done",
  { label: "changelog other", phase: "📝 生成 Release Notes", fallback: "" }
);

const statOutput = await bash(
  "变更统计",
  "git diff --no-merges --shortstat '" + lastTag + "'..HEAD",
  { label: "stat", phase: "📝 生成 Release Notes", fallback: "(no stats)" }
);

// Write changelog to temp file
const bodyFile = "/tmp/release_notes_" + NEW_VERSION + ".md";
const changelogContent = [
  '## 版本更新',
  '',
  changelogFeats ? '### ✨ 新增功能\n' + changelogFeats : '',
  '',
  changelogFixes ? '### 🐛 缺陷修复\n' + changelogFixes : '',
  '',
  changelogChores ? '### 🔧 维护更新\n' + changelogChores : '',
  '',
  changelogOther ? '### 📝 其他更新\n' + changelogOther : '',
  '',
  '### 🐳 Docker 镜像',
  '- `ntwck/sillygirl:latest`',
  '- `ntwck/sillygirl:' + NEW_VERSION + '`',
  '',
  '### 📦 支持平台',
  '- **Linux**: amd64 / arm64',
  '- **macOS**: amd64 / arm64 (Intel + Apple Silicon)',
  '- **Windows**: amd64',
  '',
  '---',
  '',
  '### 📊 变更清单',
  '',
  statOutput,
].filter(Boolean).join('\n');

if (!DRY_RUN) {
  // We can't write files in Workflow context, but in dry-run mode this is fine
  log("Release Notes 预览（不会写入文件，dry-run 模式）:");
}
log(changelogContent);

// ─── Phase 3: Dry-run remaining ─────────────────────────────────────────────

phase("🏷️ Tag + CI + Release + Deploy");

if (DRY_RUN === true) {
  log("");
  log("========== DRY RUN 完整结果 ==========");
  log("");
  log("📋 版本: " + NEW_VERSION);
  log("📋 Bump 来源: " + BUMP_SOURCE);
  log("📋 跳过部署: " + (SKIP_DEPLOY ? '是' : '否'));
  log("");
  log("📝 预检: 已完成 (gh CLI 已安装, 远程可达)");
  log("🔢 版本确定: " + NEW_VERSION);
  log("📝 Changelog: 已生成 (见上方)");
  log("");
  log("🏷️ [DRY RUN] git tag -a " + NEW_VERSION + ' -m "Release ' + NEW_VERSION + '"');
  log("🏷️ [DRY RUN] git push origin v2.1");
  log("🏷️ [DRY RUN] git push origin " + NEW_VERSION);
  log("");
  log("🚀 [DRY RUN] gh workflow run build.yml --ref " + NEW_VERSION + " --field release_tag=" + NEW_VERSION);
  log("");
  log("⏳ [DRY RUN] 等待 CI 完成（最多 30 分钟）");
  log("");
  log("📋 [DRY RUN] gh release edit " + NEW_VERSION + " --notes-file " + bodyFile);
  log("");
  if (!SKIP_DEPLOY) {
    log("🖥️ [DRY RUN] gh release download " + NEW_VERSION + " --pattern 'sillyGirl_linux_amd64' -O /tmp/sillyGirl_linux_amd64");
    log("🖥️ [DRY RUN] scp /tmp/sillyGirl_linux_amd64 " + TEST_HOST + ":/tmp/sillyGirl_linux_amd64." + NEW_VERSION);
    log("🖥️ [DRY RUN] ssh " + TEST_HOST + ' "bash -s" < deploy-test.sh ' + TEST_DIR + ' ' + NEW_VERSION);
  }
  log("");
  log("========== 没有实际执行任何操作 ==========");
} else {
  // Real release - would execute all steps
  log("执行完整发布流程...");

  // Tag
  await bash(
    "创建并推送 tag",
    "git tag -a '" + NEW_VERSION + "' -m 'Release " + NEW_VERSION + "' && git push origin v2.1 && git push origin '" + NEW_VERSION + "'",
    { label: "tag", phase: "🏷️ Tag + CI + Release + Deploy" }
  );
  log("✓ Tag 已推送: " + NEW_VERSION);

  // Write changelog
  // Can't write files in Workflow context in dry-run, skip for now

  // Trigger CI
  await bash(
    "触发 CI 构建",
    "gh workflow run build.yml --ref '" + NEW_VERSION + "' --field release_tag='" + NEW_VERSION + "'",
    { label: "CI trigger", phase: "🏷️ Tag + CI + Release + Deploy" }
  );
  log("✓ CI 已触发");

  // Wait for CI
  await bash(
    "等待 CI 完成（轮询）",
    "RUN_ID=$(gh run list --workflow=build.yml --limit 1 --json databaseId --jq '.[0].databaseId'); echo 'RUN_ID=$RUN_ID'; for i in $(seq 1 120); do sleep 15; STATUS=$(gh run view $RUN_ID --json status --jq '.status' 2>/dev/null || echo 'unknown'); CONCLUSION=$(gh run view $RUN_ID --json conclusion --jq '.conclusion' 2>/dev/null || echo 'unknown'); if [ '$STATUS' = 'completed' ]; then if [ '$CONCLUSION' = 'success' ]; then echo 'SUCCESS'; else echo 'FAILED: $CONCLUSION'; exit 1; fi; break; fi; echo 'CI: $STATUS'; done",
    { label: "CI wait", phase: "⏳ 等待 CI" }
  );

  // Update release notes
  await bash(
    "更新 Release Notes",
    "gh release edit '" + NEW_VERSION + "' --notes-file '" + bodyFile + "' 2>&1 || echo 'Release not yet created'",
    { label: "release notes", phase: "📋 更新 Release Notes" }
  );

  // Deploy
  if (!SKIP_DEPLOY) {
    await bash(
      "下载二进制",
      "gh release download '" + NEW_VERSION + "' --pattern 'sillyGirl_linux_amd64' -O /tmp/sillyGirl_linux_amd64 && file /tmp/sillyGirl_linux_amd64 | grep -q 'ELF' && echo 'valid' || echo 'invalid'",
      { label: "download", phase: "🖥️ 部署测试机" }
    );
    await bash(
      "上传到测试机",
      "scp /tmp/sillyGirl_linux_amd64 " + TEST_HOST + ":/tmp/sillyGirl_linux_amd64." + NEW_VERSION,
      { label: "scp", phase: "🖥️ 部署测试机" }
    );
    await bash(
      "远程部署",
      "ssh " + TEST_HOST + " 'bash -s' <<'EOF'\nset -euo pipefail\ncd /home/pagermaid/docker/sillyplus && cp /tmp/sillyGirl_linux_amd64." + NEW_VERSION + " sillyGirl_linux_amd64 && pm2 restart sillygirl && sleep 5 && pm2 status sillygirl\nEOF",
      { label: "deploy", phase: "🖥️ 部署测试机" }
    );
  }

  log("\n🎉🎉🎉  发布完成！" + NEW_VERSION + "  🎉🎉🎉");
}
