# AGENTS.md

本文件是本仓库的代理工作约定。修改功能前先阅读相关代码与本文件；完成任务时按 [.agents/skills/git-delivery/SKILL.md](.agents/skills/git-delivery/SKILL.md) 提交。

## 项目概览

NiriDesk 是一个面向 niri/Wayland 的浏览器远程桌面：

- `cmd/niridesk`：进程入口、健康检查、优雅关闭与 wayvnc 监管。
- `internal/auth`：无状态签名会话、登录限流。
- `internal/config`：环境变量解析与安全校验。
- `internal/wayvnc`：wayvnc 进程、日志、控制 socket、输出切换。
- `internal/web`：HTTP API、静态资源、鉴权、WebSocket ↔ RFB 代理。
- `web/`：SolidJS + Vite + TypeScript 前端，pnpm 管理依赖。noVNC 1.7.0（MPL-2.0）由 pnpm 安装并打包，不进入 Git。`web/dist` 仅在构建时生成。
- 部署：单容器运行 Go 网关与 Debian 的 wayvnc；Go 二进制内嵌前端。

数据流：浏览器 WSS → Go 网关鉴权 → `127.0.0.1:5900` RFB → wayvnc → niri Wayland socket。客户端永远不能直接连 VNC。

## 常用命令

```bash
make init          # 生成 .env（随机密码/密钥、检测 UID 与 Wayland display）
make web-deps      # 使用 pnpm 安装锁定的前端依赖
make web-build     # TypeScript 检查并用 Vite 生成 web/dist
make dev-web       # Vite 开发服务器（代理到 :8080）
make check         # gofmt、go vet、go test、tsc 类型检查
make build         # 构建前端并生成内嵌资源的 bin/niridesk
make docker-build  # 构建 Docker 镜像
make up            # docker compose up -d --build
make down
make logs
```

本地依赖 Go 1.24+、Docker、Node 20.19+ 与 pnpm 11。前端由 Vite 构建，Go 二进制通过 `go:embed` 内嵌 `web/dist`。第三方源码、`node_modules`、`web/dist`、Go `vendor/` 都不入库。

## 关键不变量

- **VNC 只监听回环**：`internal/wayvnc` 强制使用 `127.0.0.1`；`docker-compose.yml` 不发布 5900。任何改动都不得让 VNC 直接暴露。
- **所有客户端请求先鉴权**：`/api/status`、`/api/output`、`/websocket` 必须要求有效签名 Cookie；WebSocket 必须同时通过 Origin 校验。
- **Cookie**：`HttpOnly`、`SameSite=Strict`；HTTPS 或受信任代理下使用 `Secure`。不要改为前端可读 token。
- **不记录秘密**：日志不得包含密码、Cookie、Authorization、剪贴板内容或键盘数据。
- **Wayland 权限**：容器 UID 必须匹配宿主 Wayland 用户；`$XDG_RUNTIME_DIR` 只读挂载。不要把 socket 目录改成可写或暴露 `/dev`。
- **单容器边界**：wayvnc 使用 `/tmp` 控制 socket；根文件系统保持只读，容器保持非 root、`cap_drop: ALL`、`no-new-privileges`。
- **第三方源码与产物不入库**：不要提交 `/vendor/`、`web/node_modules/`、`web/dist/`（仅保留 `web/dist/.gitkeep`）或下载的第三方源码。Go 依赖由 `go mod` 获取；前端依赖只能通过 `web/package.json` 与 `web/pnpm-lock.yaml` 由 pnpm 安装。升级依赖时修改 manifest 并更新锁文件，不要复制源码进仓库。
- **前端自包含**：不要用 CDN 或外部字体；Vite 打包 noVNC，构建脚本把随包许可证复制到 `dist/licenses/`，部署时必须保留。
- **pnpm 为唯一前端包管理器**：不要在 `web/` 下使用 npm 或 yarn，也不要手写锁文件；验证使用 `pnpm install --frozen-lockfile`。
- **移动端**：底部工具栏、安全区、粘滞修饰键与隐藏输入框是核心交互，修改布局后至少检查 390×844 与横屏全屏。
- **输出切换**：通过 `wayvncctl`；API 输入必须再次校验输出名，不能拼接 shell。切换影响所有观看者。
- **错误可诊断**：配置错误应在启动时明确失败；Wayvnc 崩溃应自动退避重启并在 `/api/status` 暴露 `last_error`。

## 测试与验收

修改后端后运行：

```bash
make check
```

首次构建前运行 `make web-build`（会自动执行 `pnpm install --frozen-lockfile`）。Docker 构建会在镜像内完成 pnpm 安装与 Vite 构建；运行时若报告 `embedded frontend assets are missing`，说明编译前没有生成 `web/dist`。

涉及容器、Wayland 或前端连接时额外验证：

```bash
make web-build
make check
docker compose build
docker compose up -d
curl -fsS http://127.0.0.1:8080/api/health
docker compose logs --tail=80
```

有真实 niri 会话时，用浏览器验证登录、画布连接、桌面/手机布局、设置面板和至少一次输出切换。无法运行的检查必须在最终回复中明确说明原因，不能声称已通过。

## 提交与长任务

- 每次完成任务后提交 Git，使用 Conventional Commits（英文主题），例如 `feat(web): add mobile modifier bar`。
- 长任务按可验证的独立里程碑拆分提交；每个提交只包含本次逻辑改动，不混入无关文件或用户未提交修改。
- 绝对不要提交 `.env`、`.cache/`、`bin/`、日志、证书私钥或浏览器存储。
- 不主动改写历史、不强推、不推送远程；除非用户明确要求。
- 完成任务时检查 `AGENTS.md` 是否仍准确；架构、命令、环境要求或验证方式变化时同步更新。
- 最终报告提交哈希、提交信息、改动范围、已执行验证与未验证项。

## 已知限制

- 单共享密码、无多用户权限模型。
- 单输出捕获，切换不持久化。
- 无音频、文件传输、录制。
- 剪贴板仅文本；移动端复杂 IME 行为取决于远端应用。
- 已在 niri 26.04、wayvnc 0.9.1、Debian trixie 验证；其他合成器未验证。
