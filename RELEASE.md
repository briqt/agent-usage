# Release Guide

本文档描述 agent-usage 的发版流程和规范。

This document describes the release process and conventions for agent-usage.

## Version Scheme

遵循 [Semantic Versioning 2.0.0](https://semver.org/)：`vMAJOR.MINOR.PATCH`

| 变更类型 | 版本位 | 示例 | 触发场景 |
|---------|--------|------|---------|
| Breaking | MAJOR | v1.0.0 → v2.0.0 | 配置格式变更、数据库 schema 不兼容迁移、API 响应结构变更 |
| Feature | MINOR | v0.1.0 → v0.2.0 | 新数据源、新仪表板面板、新 API 端点、新配置项 |
| Fix | PATCH | v0.1.0 → v0.1.1 | Bug 修复、性能优化、依赖更新、文档修正 |

### Pre-release

开发阶段可使用预发布标签：

```
v0.2.0-alpha.1    # 早期测试
v0.2.0-beta.1     # 功能完整，测试中
v0.2.0-rc.1       # 发布候选
```

## Release Checklist

发版前逐项确认：

```
[ ] 所有目标功能已合并到 main
[ ] go build 编译通过（无 warning）
[ ] go vet ./... 无问题
[ ] go test ./... 全部通过
[ ] 本地运行测试，数据解析正常
[ ] CHANGELOG.md 已更新（将 [Unreleased] 内容移至新版本号下）
[ ] README 已更新（如有用户可见变更）
[ ] config.yaml 示例已更新（如有新配置项）
```

## How to Release

### Step 1: Update CHANGELOG

将 `[Unreleased]` 部分重命名为新版本号并添加日期：

```markdown
## [0.1.0] - 2026-04-03

### Added
- Claude Code session parser
- ...

## [Unreleased]
```

提交：

```bash
git add CHANGELOG.md
git commit -m "chore: prepare release v0.1.0"
git push origin main
```

### Step 2: Create Tag

```bash
git tag -a v0.1.0 -m "Release v0.1.0: initial release with Claude Code and Codex support"
git push origin v0.1.0
```

Tag message 格式：`Release v<VERSION>: <one-line summary>`

### Step 3: Verify

1. 前往 [GitHub Actions](https://github.com/briqt/agent-usage/actions) 确认 Release workflow 成功
2. 检查 [Releases](https://github.com/briqt/agent-usage/releases) 页面：
   - Changelog 自动生成且正确
   - 6 个平台二进制已上传（linux/darwin/windows × amd64/arm64）
   - checksums.txt 存在

### Alternative: GitHub UI

也可以通过 GitHub Actions 页面手动触发：

1. 进入 Actions → Release → Run workflow
2. 输入版本号（如 `v0.1.0`）
3. 点击 Run

## Build Matrix

GoReleaser 自动交叉编译以下平台：

| OS | Arch | 产物 |
|----|------|------|
| Linux | amd64 | `agent-usage_<ver>_linux_amd64.tar.gz` |
| Linux | arm64 | `agent-usage_<ver>_linux_arm64.tar.gz` |
| macOS | amd64 | `agent-usage_<ver>_darwin_amd64.tar.gz` |
| macOS | arm64 (Apple Silicon) | `agent-usage_<ver>_darwin_arm64.tar.gz` |
| Windows | amd64 | `agent-usage_<ver>_windows_amd64.zip` |
| Windows | arm64 | `agent-usage_<ver>_windows_arm64.zip` |

所有二进制使用 `-s -w` ldflags 去除调试信息，CGO_ENABLED=0 确保静态链接。

## Hotfix Process

紧急修复流程：

```bash
# 1. 基于最新 tag 创建修复
git checkout -b hotfix/v0.1.1 v0.1.0

# 2. 修复并提交
git commit -m "fix: critical bug description"

# 3. 合并回 main
git checkout main
git merge hotfix/v0.1.1
git push origin master:main

# 4. 打 patch tag
git tag -a v0.1.1 -m "Release v0.1.1: fix critical bug"
git push origin v0.1.1

# 5. 清理
git branch -d hotfix/v0.1.1
```

## Post-Release

发版后：

1. 确认 GitHub Release 页面正常
2. 在本地测试下载的二进制能否正常运行
3. 如有必要，更新相关文档或公告
