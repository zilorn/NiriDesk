---
name: git-delivery
description: 在完成代码、文档或配置任务后自动整理并提交 Git 变更，使用 Conventional Commits；长任务按可验证里程碑分段提交，并维护项目 AGENTS.md。适用于“完成任务后提交”“长任务分段提交”“生成 AGENTS.md”以及任何以 Git 交付的工作。
---

# Git 交付

任务完成不等于代码写完。只有变更已整理、验证并提交，且项目约定同步完成，才算交付。

## 红线

- 只提交本次任务相关文件。用户未提交的修改、`.env`、凭据、缓存、构建产物和日志绝不混入。
- 不主动 `push`、`rebase`、`reset`、`commit --amend` 或强推。需要改写历史或发布时，先得到用户明确授权。
- 不把失败说成通过。验证无法运行时，在最终回复中列出原因与未验证范围。
- 不为了清空工作区而提交半成品。任务被阻塞时保留未提交改动并说明。

## 流程

1. **确认范围**
   - 运行 `git status --short`，查看 `git diff --stat` 与相关 diff。
   - 区分本次任务文件与用户既有修改；只暂存前者。

2. **先验证，再提交**
   - 以项目 `AGENTS.md`、`Makefile`、`package.json` 或 CI 配置为准。
   - 至少运行与改动直接相关的测试、类型检查、格式检查和构建。
   - 提交前确保暂存内容可以独立通过对应验证。

3. **划分提交**
   - 小任务：一个完整提交。
   - 长任务：按“功能/修复、构建与部署、文档与约定”等可独立验证的里程碑提交。每个提交都应是逻辑完整的 Conventional Commit，不使用 `wip`。
   - 提交顺序应让仓库在每一步都可构建；后一个提交不要依赖未提交文件。
   - 同一文件包含多个逻辑改动且难以拆分时，优先进度合理的单个提交，而不是伪造原子性。

4. **提交格式**
   - 主题格式：`<type>(<scope>): <imperative summary>`，scope 可省略。
   - 常用 type：`feat`、`fix`、`docs`、`refactor`、`test`、`build`、`ci`、`perf`、`chore`。
   - 主题使用英文、祈使句、简洁描述结果，不超过 72 字符，不以句号结尾。
   - 必要时加短 body，解释动机、取舍或验证；不要写成长篇流水账。
   - 示例：
     - `feat(web): add authenticated mobile remote desktop`
     - `build(docker): package wayvnc with the web gateway`
     - `docs: document deployment and security boundaries`
     - `fix(auth): reject expired session cookies`

5. **暂存与提交**
   - 使用显式路径：`git add -- path/to/file ...`。
   - 不要用 `git add -A` 或 `git add .` 代替范围审查。
   - 提交后运行 `git status --short`，确认没有遗漏或误提交。
   - 若发现提交包含无关文件：在未推送且用户允许改写历史时修正；否则报告，不要擅自强推。

6. **维护 AGENTS.md**
   - 仓库没有 `AGENTS.md`，或任务改变了架构、命令、环境要求、测试方式、代码边界时，创建或更新它。
   - 长任务至少在每个里程碑后检查一次，最终提交前再检查一次。
   - 内容保持可执行、精炼：项目概览、技术栈、常用命令、关键不变量、验证步骤、提交约定、已知限制。
   - 不写秘密、个人路径、临时环境状态或空泛口号。
   - 如果项目已有专用技能目录（如 `.agents/skills/`），在 `AGENTS.md` 中链接相关技能，并同步更新接口或流程变化。

7. **汇报**
   - 给出每个提交的短哈希与标题。
   - 用表格或简短列表列出改动范围。
   - 列出已运行的验证及结果；无法运行的项说明原因。
   - 明确仍未完成或未验证的内容。

## 里程碑切分示例

长任务“新增远程桌面产品的 Docker 部署”可以拆为：

1. `feat(server): add authenticated remote desktop gateway`
2. `feat(web): add mobile remote desktop client`
3. `build(docker): add single-container wayvnc deployment`
4. `docs: document setup, security, and operations`
5. `docs(agents): add project workflow and git delivery skill`

每个提交单独通过对应检查。若一次改动无法独立构建，就合并相邻步骤，直到提交自洽。

## 完成检查

- [ ] 只包含本次任务文件
- [ ] 提交信息符合 Conventional Commits
- [ ] 相关测试/构建已运行并通过
- [ ] 长任务已分段提交
- [ ] `AGENTS.md` 已创建或仍准确
- [ ] 未推送或改写历史（除非已获授权）
- [ ] 已向用户报告提交与验证结果
