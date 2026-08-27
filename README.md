# USBridge

USBridge 是一套 Android 与 Windows 配合使用的 USB 网络共享工具。Android 端负责开启 USB 网络共享、切换移动数据连接、展示公网 IPv4/IPv6 并记录共享流量；Windows 端自动识别手机网络适配器，并提供本机 HTTP 与 SOCKS5 代理。

## 功能

- Android 11 及以上，采用 Material 3 界面
- 通过 Root 控制 USB 网络共享与移动网络重连
- 兼容通过 libsu 提供 Root 权限的 KernelSU、Magisk、APatch 等方案
- USB 接入后可自动开启网络共享
- 自动、IPv4、IPv6 三种网络模式
- 手机端公网地址展示与 USB 流量统计
- Windows 自动识别手机 USB 网卡
- 本机 HTTP/SOCKS5 代理：`127.0.0.1:18080`
- 带用户名密码的 HTTP/SOCKS5 代理：`127.0.0.1:18081`
- Windows 本机控制 API：`127.0.0.1:18082`
- Windows 独占模式：经 UAC 授权后使用 WFP 限制其他程序直接访问手机网络，只允许通过 USBridge 代理使用

## 架构

- `app/`：Kotlin、Jetpack Compose 与 Material 3 构建的 Android 应用
- `desktop/`：Go、Wails 与 Vue 构建的 Windows 应用，包含代理、控制 API、网卡路由和 WFP 独占策略
- `scripts/`：Android/Windows 发布构建与签名脚本

手机端控制服务仅面向 USB 共享链路。Windows 控制 API 与代理默认只监听回环地址，不应暴露到局域网或公网。

## 构建要求

- JDK 21
- Android SDK 36.1 与 Build Tools 36.0.0
- Go 1.25 或更高版本
- Node.js 22 与 npm
- Windows 上安装 Wails CLI（构建桌面程序时需要）

Android 单元测试与调试构建：

```powershell
.\gradlew.bat test assembleDebug
```

Windows 后端与前端：

```powershell
Set-Location desktop
go test ./...
go vet ./...
Set-Location frontend
npm ci
npm run build
```

Windows 正式构建：

```powershell
Set-Location desktop
wails build
```

同时构建发布产物：

```powershell
.\scripts\build-release.ps1
```

## 使用说明

1. 在 Android 设备上安装应用并授予 Root 权限。
2. 使用 USB 数据线连接 Windows 电脑，在手机端开启或允许自动开启 USB 网络共享。
3. 启动 Windows 客户端，等待其识别手机网卡。
4. 将需要使用手机网络的软件配置到界面所示的 HTTP 或 SOCKS5 代理。
5. 需要阻止其他程序绕过代理时，启用独占模式并接受 UAC 授权。

Root、移动数据和网络共享实现受设备厂商及系统版本影响。首次使用前请确认设备具备可恢复的 Root 环境。

## 发布与安全

官方 APK 应始终使用同一 Android 密钥签名，Windows 安装包或 EXE 应使用可信代码签名证书。私钥、证书密码和本地配置不得提交到仓库。完整发布流程见 [docs/RELEASING.md](docs/RELEASING.md)，安全问题报告方式见 [SECURITY.md](SECURITY.md)。

## 许可证

项目使用 [Apache License 2.0](LICENSE) 发布。Material Symbols 由 Google 提供，同样采用 Apache License 2.0。
