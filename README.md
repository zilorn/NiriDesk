# NiriDesk

在浏览器中安全地访问 niri 桌面。服务端用 wayvnc 通过 Wayland screencopy 抓取画面并注入键鼠，Go 网关负责密码登录、签名会话与 WebSocket 代理，前端是 SolidJS + Vite 构建的移动优先 PWA，noVNC 由 pnpm 管理并打包进静态资源。

```
手机 / 平板 / 桌面浏览器
        │  HTTPS + WSS（密码登录、签名 Cookie）
        ├── RFB 画面与控制 ──┐
        └── 时间戳 PCM 音频 ─┤
                             ▼
                    NiriDesk Go 网关（鉴权、限流、代理）
                     │                      │
                     │ 仅回环 127.0.0.1:5900│ PulseAudio/PipeWire
                     ▼                      ▼
                   wayvnc ──── niri      parec（系统声音）
                     │
                     └── Wayland gamma control（防窥时物理输出全黑）
```

## 功能

- **面向 niri**：基于 wayvnc 的 wlr-screencopy 与 virtual-keyboard/pointer 支持，不需要 Xwayland。
- **手机可用**：画面默认铺满手机屏幕（裁切边缘、拖动平移），菜单以浮层覆盖且可隐藏；支持适应窗口/1:1 拖动、横屏全屏、粘滞 `Ctrl`/`Alt`/`Super`/`Shift` 与常用按键、移动端虚拟键盘。
- **多显示器**：在设置中列出 niri 输出并实时切换捕获目标，无需重启容器。
- **音频同步**：通过独立鉴权的 WebSocket 传输带时间戳的 48 kHz 立体声 PCM，浏览器用 Web Audio 调度播放；可调节音画延迟以补偿 VNC 画面延迟。
- **防窥模式**：通过 Wayland gamma control 把物理输出压成全黑，wayvnc screencopy 仍取得正常画面；远程键鼠不会唤醒显示，关闭模式或断开最后一个画面会话后自动恢复。
- **会话安全**：口令登录、HttpOnly/SameSite 签名 Cookie、登录限流、WebSocket Origin 校验；wayvnc 永远只监听容器回环地址。
- **可安装 PWA**：在 HTTPS 或 localhost 下可添加到主屏幕，支持 Wake Lock 防止手机息屏。
- **可调画质**：noVNC 画质/压缩实时调整，支持只看不控、剪贴板桥接和多名观众。
- **单容器部署**：多阶段构建、非 root、只读根文件系统、丢弃全部 Linux capabilities。

## 环境要求

- Linux 上的 niri 会话（已在 niri 26.04 验证）。
- Docker Engine 与 Docker Compose v2。
- 运行容器的用户 UID 必须与 Wayland 会话用户一致，默认 `1000`。
- 宿主机可访问 `$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY` socket。
- 不需要 `/dev/dri`；wayvnc 使用 screencopy 而不是 DRM 接管。
- 音频需要宿主机提供 PulseAudio 或 PipeWire 的 Pulse 兼容 socket；默认使用当前用户的 sink monitor。镜像内已包含 `parec`。
- 防窥模式需要合成器支持 `zwlr_gamma_control_manager_v1`（niri 26.04 已验证）。
- 构建镜像需要访问 Go module proxy 与 npm registry。前端依赖由 `web/pnpm-lock.yaml` 锁定，第三方源码、`node_modules` 和构建产物都不进入本仓库。

> NiriDesk 面向单用户、可信网络的远程访问。暴露到公网前请使用 HTTPS 反向代理或 VPN。

## 快速开始

```bash
make init
docker compose up -d --build
```

`make init` 会生成权限为 `0600` 的 `.env`，自动检测 UID、`XDG_RUNTIME_DIR` 与 `WAYLAND_DISPLAY`，并生成访问密码和会话密钥。密码会打印一次，也可随时查看：

```bash
grep '^REMOTE_PASSWORD=' .env
```

启动后访问：

```text
http://<这台机器的局域网 IP>:8080
```

验证服务：

```bash
curl -fsS http://127.0.0.1:8080/api/health
docker compose ps
docker compose logs --tail=80
```

停止：

```bash
docker compose down
```

如果不想使用 `make init`，复制 `.env.example` 为 `.env` 并至少填写 `REMOTE_PASSWORD`。未设置 `SESSION_SECRET` 时服务仍会启动，但每次重启都会让登录会话失效。

## 手机使用

1. 在同一局域网或 VPN 中打开服务地址并登录。
2. 手机上的远程画面默认「铺满」整个屏幕，画面边缘会按屏幕比例裁切；单指拖动可平移。缩放按钮在「适应 / 铺满 / 1:1」之间循环，可在设置中直接选择。
3. 手机首次进入会默认隐藏上下菜单并铺满屏幕。点屏幕右侧的悬浮球可显示或收起上下栏，进入浏览器全屏时也会自动隐藏；设置中的「隐藏控制菜单」可保持该状态。
4. 底部工具栏提供键盘、修饰键、缩放、声音、剪贴板和设置；右上角可全屏。
5. 使用「修饰键」时，先点亮 `Ctrl`/`Alt`/`Super`/`Shift`，再按字母或常用键；`释放` 可清除所有按下状态。
6. 点「声音」播放远端系统音频。浏览器要求用户点按后才能播放；若口型不同步，在设置的「音画同步延迟」中增大数值。
7. 在设置中打开「防窥模式」后，物理显示器会全黑，但远程画面与声音继续工作。关闭开关、断开最后一个画面会话或退出服务时自动恢复。
8. 手机浏览器通常不允许 HTTP 页面访问剪贴板或安装 PWA。通过 HTTPS 访问后功能完整；建议使用 Caddy、Tailscale Serve 或可信内网证书。
9. 观看端不操作时可打开「只看不控」；它会阻止前端发送所有键鼠事件。

## 配置

所有配置都通过环境变量提供。Compose 从 `.env` 读取。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PUID` / `PGID` | `1000` | 容器运行 UID/GID，必须匹配 Wayland 会话用户。 |
| `XDG_RUNTIME_DIR` | `/run/user/1000` | 宿主机 Wayland socket 目录，会以只读方式挂载。 |
| `WAYLAND_DISPLAY` | `wayland-1` | niri 的 Wayland display 名称。 |
| `REMOTE_PASSWORD` | 必填 | 网页登录密码，至少 8 个字符。也可用 `REMOTE_PASSWORD_FILE` 指定文件。 |
| `SESSION_SECRET` | 随机 | 会话签名密钥，建议设置为 `openssl rand -hex 32`；不设置则仅保存在内存中。 |
| `SESSION_TTL` | `12h` | 登录有效期，范围 5 分钟到 30 天。 |
| `BIND_ADDR` / `PORT` | `0.0.0.0` / `8080` | 发布到宿主机的地址和端口。前置反向代理时建议改为 `127.0.0.1`。 |
| `MAX_CLIENTS` | `4` | 同时连接的浏览器会话上限，1–32。 |
| `TRUST_PROXY` | `false` | 仅在受信任反向代理后开启，用于 `X-Forwarded-For` 与 `X-Forwarded-Proto`。 |
| `COOKIE_SECURE` | `auto` | `auto`、`true`、`false`。`auto` 在 HTTPS 请求或受信任代理转发 HTTPS 时设置 Secure。 |
| `WAYVNC_OUTPUT` | 首个输出 | 启动时捕获的输出名，例如 `DP-1`；网页中的切换只在当前进程内生效。 |
| `WAYVNC_MAX_FPS` | `30` | 捕获帧率上限，1–120。 |
| `WAYVNC_SHOW_CURSOR` | `true` | 是否在画面中绘制远端鼠标指针。 |
| `WAYVNC_DISABLE_INPUT` | `false` | 设为 `true` 时彻底禁用远程键鼠，只保留观看。 |
| `AUDIO_ENABLED` | `true` | 是否捕获并传输系统音频；缺少 PulseAudio/PipeWire 时可关闭。 |
| `AUDIO_DEVICE` | `@DEFAULT_MONITOR@` | PulseAudio 源；默认捕获当前默认输出的 monitor。 |
| `AUDIO_BINARY` | `parec` | 音频捕获程序路径，通常无需修改。 |
| `LOG_LEVEL` | `info` | `debug`、`info`、`warn`、`error`。 |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | 空 | 直接让 NiriDesk 提供 HTTPS 时使用，需同时设置并将证书挂载进容器。 |

其他仅用于本地开发或外部 VNC 的变量：`VNC_ADDR`、`WAYVNC_MANAGED=false`、`WAYVNC_BIN`、`WAYVNCCTL_BIN`、`WAYVNC_CONTROL_SOCKET`。

### 多显示器

登录后打开「设置 → 显示器输出」，选择任意 niri 输出即可切换。所有观看者会同时看到新的输出；设置只在本次 wayvnc 进程存活期间有效。要固定默认输出，设置 `.env` 中的 `WAYVNC_OUTPUT` 并重建容器。防窥模式会压黑所有物理输出，而不只是当前捕获的输出。

## HTTPS 与安全

默认的明文 HTTP 只适合完全可信的局域网。网页密码会随 HTTP 明文传输，因此对外或跨不可信网络时必须使用 TLS。

### Caddy 反向代理

将 [deploy/Caddyfile.example](deploy/Caddyfile.example) 作为起点，并在 `.env` 中设置：

```dotenv
BIND_ADDR=127.0.0.1
TRUST_PROXY=true
COOKIE_SECURE=auto
```

Caddy 默认保留原始 `Host`，因此 WebSocket Origin 校验可以正常工作。如果其他代理会改写 Host，需要同时转发原始 Host。

### 原生 TLS

在直接运行二进制或自定义容器时，挂载证书并设置 `TLS_CERT_FILE`、`TLS_KEY_FILE`。Compose 中没有预置证书挂载，推荐反向代理。

### 建议

- 使用 `openssl rand -hex 32` 生成随机的 `SESSION_SECRET`。
- 不要把 8080 暴露到公网；优先使用 Tailscale/WireGuard 等私有网络。
- 永远不要发布 5900 端口。wayvnc 只绑定容器回环，所有访问都必须经过网关鉴权。
- 登录失败 5 次后，同一来源会被封禁 5 分钟。容器重启会清空该状态。
- 远程桌面等同于控制你的用户会话，请像保护 SSH 私钥一样保护密码和 `.env`。
- 容器以非 root 运行、丢弃全部 capabilities、根文件系统只读，只挂载 Wayland socket 和临时目录。

## Docker 说明

Compose 的挂载方式：

```yaml
volumes:
  - "${XDG_RUNTIME_DIR}:/run/user/1000:ro"
```

Wayland socket 的父子目录权限通常为 `0700`，因此容器的 UID 必须与宿主机用户一致；不匹配时会看到 `Permission denied` 或 wayvnc 无法连接。使用 rootless Docker 且用户命名空间映射不同时，需要额外配置映射或改用与映射后 UID 匹配的 `PUID`/`PGID`。

Wayvnc 不写入 Wayland socket 目录，因此只读挂载足够。它需要写入控制 socket，使用 `/tmp` 的 tmpfs。音频通过同一挂载中的 PulseAudio socket 采集；防窥通过 Wayland socket 设置 gamma，不需要额外设备权限。

## 本地开发

```bash
make web-deps     # 使用 pnpm 安装锁定版本的前端依赖
make web-build    # TypeScript 检查并生成 web/dist
make dev-web      # Vite 开发服务器，代理 /api 与 /websocket 到 :8080
make check        # gofmt、go vet、Go 测试、前端类型检查
make build        # 构建前端并生成内嵌静态资源的 bin/niridesk
make docker-build # 构建容器镜像（前端在镜像内由 pnpm 构建）
```

前端位于 `web/`，使用 SolidJS、Vite、TypeScript 与 pnpm。`package.json` 和 `pnpm-lock.yaml` 进入 Git；`node_modules/`、`web/dist/` 由 `make` 或 Docker 生成并被忽略。Go 只嵌入构建后的 `web/dist`，因此不要直接提交前端构建产物或第三方源码。直接执行 `go build` 而未生成 `web/dist` 时，二进制会在启动阶段明确报错并提示运行 `make web-build`。

Vite 开发服务器默认在 `5173` 端口，并把 `/api` 与 `/websocket` 代理到本机 `8080`。先运行网关（例如 `make run`），再在另一个终端运行 `make dev-web`。

直接运行二进制并连接一个已有的 wayvnc：

```bash
REMOTE_PASSWORD='local-development-password' \
SESSION_SECRET='0123456789abcdef0123456789abcdef' \
LISTEN_ADDR=127.0.0.1:8080 \
VNC_ADDR=127.0.0.1:5900 \
WAYVNC_MANAGED=false \
bin/niridesk
```

前端使用 SolidJS + Vite，noVNC 1.7.0 与 pako 由 pnpm 解析并打包进按哈希命名的静态资源；构建时会把 noVNC 及其依赖的许可证复制到 `dist/licenses/`，随镜像一起分发。图标由 `scripts/generate-icons.py` 生成到 `web/public/icons/`。

### 测试

- Go 单元测试覆盖登录会话、限流、配置校验、wayvnc 控制接口、音频帧协议、防窥 wire 协议和 WebSocket 代理。
- 开发中已用真实 niri + wayvnc + headless Chrome 驱动验证登录、画布连接、桌面端/手机端布局、设置面板、输出切换与 wayvnc 崩溃重启；此浏览器脚本依赖本机 Chrome/CDP，未作为公开测试入口提交。
- 音频已在容器内用真实 PulseAudio/PipeWire 验证采集、WebSocket 帧时间戳和非静音内容；防窥已通过 DRM `GAMMA_LUT` 读回验证全零、远程输入后保持全零、关闭后恢复。
- 当前验证环境：niri 26.04、wayvnc 0.9.1、Debian trixie、Chrome 156。

## 故障排查

**启动时报 `embedded frontend assets are missing`**

本地编译前没有生成前端构建产物。运行 `make web-build` 后重新构建；Docker/Compose 构建会自动执行 pnpm 安装与 Vite 构建。`go.mod` 中的 Go 依赖由 `go mod download` 获取，不提交 vendor 目录。

**页面能打开，但一直显示「正在连接桌面」**

```bash
docker compose logs --tail=100
docker exec niridesk niridesk version
docker exec niridesk wayvncctl -S /tmp/niridesk-wayvnc.sock -j output-list
```

确认容器里能看到 `$WAYLAND_DISPLAY`，并且 UID 与宿主机 Wayland 用户一致。Niri 的 socket 通常位于 `/run/user/<uid>/wayland-1`。

**wayvnc 报 Permission denied**

`PUID`/`PGID` 与 `XDG_RUNTIME_DIR` 的实际所有者不匹配。运行 `id -u`、`id -g`、`ls -ld /run/user/$(id -u)`，更新 `.env` 后重启。

**没有可切换的输出**

检查 niri 是否启用了 screencopy 支持，并运行上面的 `wayvncctl output-list`。在 niri 26.04 与 Debian 的 wayvnc 0.9.1 组合上已验证。

**切换输出后画面异常**

切换会让所有观看者一起变更。如果浏览器保留旧尺寸，点一次「重新连接」即可重新协商。

**重启容器后需要重新登录**

没有设置 `SESSION_SECRET`。设置固定值后重建容器。

**手机无法安装 PWA 或使用剪贴板**

浏览器要求安全上下文。使用 HTTPS（或 localhost）。纯 HTTP 局域网页面只能使用基础远程桌面。

**声音按钮不可用或没有声音**

```bash
docker exec niridesk parec --raw --format=s16le --rate=48000 --channels=2 --device=@DEFAULT_MONITOR@ --latency-msec=20 | wc -c
```

先确认宿主机 PulseAudio/PipeWire 正常、容器 UID 能访问 `$XDG_RUNTIME_DIR/pulse/native`。浏览器还要求点按后才能播放。自定义 PulseAudio 服务地址时，需要自行给容器设置 `PULSE_SERVER`。

**音画不同步**

音频与 VNC 画面走不同通道，VNC 编码与网络延迟通常更大。打开设置，提高「音画同步延迟」直到声音与画面口型一致。该值会保存在浏览器本地。

**防窥模式不可用**

防窥依赖 Wayland gamma control。运行 `wayland-info | grep gamma` 确认存在 `zwlr_gamma_control_manager_v1`。其他合成器或远程/无头环境可能不提供该协议。开启后应在物理显示器上看到全黑，但远程画面保持正常。

**输入法输入中文无效**

手机端隐藏输入框会把 IME 完成的字符以 Unicode keysym 发送；部分远处的应用/输入法组合可能不接受这种注入。可改用物理键盘、在远端启用输入法，或通过剪贴板传递文本。

**iOS 上无法全屏**

iOS Safari 对元素全屏支持有限；添加到主屏幕后以独立窗口运行。布局和触控不依赖浏览器全屏。

## 限制

- 同一时间只捕获一个输出，切换会影响所有观看者；防窥会压黑所有物理输出。
- 音频使用未压缩 48 kHz 立体声 PCM，约 1.5 Mbps/观看者，尚未使用 Opus 等压缩编码。
- 音画同步需要手动调节延迟；VNC 没有可用的逐帧时间戳，无法自动做口型对齐。
- 防窥依赖合成器的 `wlr-gamma-control`；不支持该协议的合成器无法启用。远程输入不会清除 gamma，但进程崩溃时由合成器在连接断开后自动恢复。
- 无文件传输、多用户账号或录制。
- 画面经 VNC/RFB 传输，不是为高帧率游戏设计的协议；局域网办公体验良好，公网高延迟下建议降低画质与帧率。
- 依赖 niri 提供的 screencopy、virtual pointer/keyboard 与 wayvnc 控制接口；其他 Wayland 合成器可能可用但未验证。
- 剪贴板只处理文本。
- 仅支持单个共享访问密码，没有每用户权限模型。

## 许可证

NiriDesk 使用 MIT License。第三方依赖不存储在源码仓库中：Go 依赖由 Go modules 获取，前端依赖由 pnpm 按锁文件安装。noVNC（MPL-2.0）、pako 与 noVNC 内置组件的许可证会在构建时复制到 `dist/licenses/`，并随容器镜像分发。完整依赖清单见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
