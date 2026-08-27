# USBridge Desktop

Windows 端核心使用 Go。当前阶段提供：

- 自动扫描并评分 USB/RNDIS/NCM 网络适配器；
- HTTP（含 HTTPS `CONNECT`）与 SOCKS5 共用 TCP 端口并自动分流；
- Wails 桌面程序同时提供 `18080` 免密代理和 `18081` 账密代理；
- SOCKS5 支持域名、IPv4 和 IPv6；
- 分协议统计实时速度、累计流量和连接数；
- `auto`、`ipv4`、`ipv6` 三种出口模式；
- 所有代理连接绑定到选中的 USB 网卡源地址；
- 没有识别到 USB 网卡时失败关闭，不回退到电脑原有网络；
- 可选的 Windows 独占模式，阻止其他程序直接使用所选手机 USB 网卡；
- 安卓控制接口客户端及 JSON 契约。
- 仅监听本机的 Windows 控制 API，供其他程序触发换 IP 和共享控制；
- Wails 2 + Vue 3 桌面界面，共用安卓端的 Material 3 色彩层级。

## 开发命令

当前机器的 Go 位于：

```powershell
& 'D:\Program Files\DevRuntimes\Go\bin\go.exe' test ./...
& 'D:\Program Files\DevRuntimes\Go\bin\go.exe' build ./cmd/usbridge
```

构建桌面界面：

```powershell
$env:PATH = 'D:\Program Files\DevRuntimes\Go\bin;D:\Program Files\DevRuntimes\Node.js;' + $env:PATH
& 'D:\Program Files\DevRuntimes\Go\bin\go.exe' run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 build -clean
```

Wails GUI 不能使用普通 `go build` 打包；生产构建必须由 `wails build` 注入正确的 Windows/WebView2 构建标签。

列出网卡：

```powershell
.\usbridge.exe --list-adapters
```

启动代理：

```powershell
.\usbridge.exe --listen 127.0.0.1:18080 --ip-mode auto
```

默认只监听本机回环地址。安卓 API 默认使用所选 USB 网卡的默认网关和 `17890` 端口。

## 目录

```text
cmd/usbridge/              命令行入口与后台核心
internal/adapter/          Windows 网卡发现、自动评分和手动选择
internal/routing/          USB 源地址绑定与 IP 模式策略
internal/proxy/httpproxy/  HTTP/HTTPS CONNECT 代理
internal/proxy/socks5/     SOCKS5 代理
internal/proxy/multiplex/  单端口协议识别与分流
internal/traffic/          实时、累计及分协议流量统计
internal/androidclient/    安卓控制 API 客户端与契约
internal/controlapi/       Windows 本机 HTTP 控制 API
internal/exclusivenet/     提权辅助进程与动态 WFP 独占策略
internal/service/          Wails UI 绑定的控制层
frontend/                  Vue 3 + TypeScript 桌面界面
```

Wails 界面已绑定 `internal/service.Controller`，代理核心仍不依赖 UI。桌面端会持续检查安卓控制服务，只有 USB 网卡、手机 API 和 Root 三项均可用时才启用手机控制按钮。

安卓控制服务监听手机 USB 共享接口的 `17890` 端口，并拒绝从 Wi-Fi、蜂窝或其他非 USB 接口进入的请求。当前契约包括：

- `GET /v1/status`
- `POST /v1/mobile/reconnect`
- `POST /v1/public-ip/refresh`
- `POST /v1/upstream/cellular`
- `POST /v1/tether/start`
- `POST /v1/tether/stop`
- `PUT /v1/ip-mode`
- `GET /v1/traffic`

移动网络重连结果会分别报告 Root 命令、蜂窝网络断开、蜂窝网络恢复和公网 IP 变化，公网 IP 未变化时不会伪装成“换 IP 成功”。

USB 共享启用时，安卓端会暂时关闭 Wi-Fi 并守护蜂窝默认网络；USB 断开后，仅当 Wi-Fi 是由 USBridge 关闭时才自动恢复。桌面代理也会检查 `cellularUpstream`，手机共享上游不是蜂窝网络时直接失败关闭。

## Windows 本机代理

Wails 桌面程序固定提供两个仅监听回环地址的统一代理：

- `127.0.0.1:18080`：免密，支持 HTTP、HTTPS 和 SOCKS5；
- `127.0.0.1:18081`：用户名密码认证，支持 HTTP、HTTPS 和 SOCKS5。

账密代理固定使用用户名 `usbridge` 和密码 `usbridge_pw`，并保存到 `%AppData%\USBridge\config.json`。连接页会显示完整代理 URI、用户名和密码，并提供复制按钮。HTTP 使用 Basic Proxy Authentication，SOCKS5 使用 RFC 1929 用户名密码认证。

两个端口共享同一套 USB 网卡绑定、IP 模式和流量统计。手机网络不可用时，两种代理都不会回退到电脑的其他网络。

## Windows 独占模式

设置页可以开启“独占模式”。开启后，只有 `USBridge.exe` 能直接通过当前选中的手机 USB 网卡建立出站连接；其他 Windows 程序必须连接 `127.0.0.1:18080` 或 `127.0.0.1:18081`，再由 USBridge 转发。电脑的 Wi-Fi、以太网和 VPN 不受影响。

首次开启会显示 Windows 管理员授权窗口。主界面仍以普通用户权限运行，授权后的同一程序会进入隐藏的辅助模式，并使用 Windows Filtering Platform 在 IPv4、IPv6 出站连接层安装动态规则。规则同时保留 DHCP 和必要的 IPv6 邻居发现流量，覆盖 TCP、UDP/QUIC、ICMP 及其他 IP 协议。

辅助进程只接受带随机会话密钥的本机命令，也不接受外部程序路径。关闭独占模式、切换 USB 网卡或退出 USBridge 时会清理或替换规则；主程序或辅助进程异常结束时，Windows 会自动回收动态 WFP 会话。独占模式设置保存在 `%AppData%\USBridge\config.json`，新安装默认关闭。

## Windows 本机控制 API

桌面程序启动后固定监听 `http://127.0.0.1:18082`。接口不需要账号或 Token，也不会监听局域网地址。所有写操作必须使用 `Content-Type: application/json`，服务不开放 CORS。

```powershell
curl.exe -X POST -H "Content-Type: application/json" http://127.0.0.1:18082/v1/mobile/reconnect
```

可用端点：

- `GET /v1/status`：Windows、代理、USB 网卡和手机完整状态；
- `POST /v1/mobile/reconnect`：重新连接移动网络并比较公网 IP；
- `POST /v1/public-ip/refresh`：不重连网络，仅重新读取手机公网 IP；
- `POST /v1/upstream/cellular`：强制 USB 共享使用蜂窝上游；
- `POST /v1/tether/start`、`POST /v1/tether/stop`：控制 USB 共享；
- `PUT /v1/ip-mode`：请求体为 `{"mode":"auto"}`、`ipv4` 或 `ipv6`；
- `GET /v1/traffic`：读取 Windows 代理流量；
- `GET /v1/help`：读取机器可解析的接口清单。

换 IP 响应会返回 `before`、`after`、`ipChanged`、`commandSucceeded`、`networkDisconnected` 和 `networkRecovered`，调用方应根据这些字段判断是否真的换到了新公网 IP。
