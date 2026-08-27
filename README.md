# USBridge

让 Windows 通过 Android 手机的 USB 共享网络上网，并把这条网络作为 HTTP / SOCKS5 代理提供给本机软件。

手机端和电脑端配套使用。目前支持 **Android 11+（需要 Root）** 和 **Windows x64**，没有账号、云端服务或远程控制。

## 下载

从 [Releases](https://github.com/jmcgillcoder/USBridge/releases/latest) 下载同一版本的 Android APK 和 Windows EXE。

- 手机安装 APK，首次启动时授予 Root 权限
- 电脑运行 EXE，不需要安装
- Windows 文件尚未使用商业代码签名证书，首次运行可能出现 SmartScreen 提示

## 怎么用

1. 用数据线连接手机和电脑。
2. 打开手机端 USBridge，开启 USB 网络共享；也可以在设置里启用“连接 USB 后自动开启”。
3. 打开 Windows 端，等它识别出手机的 USB 网卡。
4. 在需要走手机网络的软件里填写代理地址。

| 用途 | 地址 | 账号 |
| --- | --- | --- |
| HTTP / SOCKS5 | `127.0.0.1:18080` | 无 |
| HTTP / SOCKS5 | `127.0.0.1:18081` | `usbridge` / `usbridge_pw` |

两个端口都同时支持 HTTP、HTTPS CONNECT 和 SOCKS5，不会在手机网络断开时偷偷改走电脑原来的 Wi-Fi 或网线。

## 换 IP

Windows 端的“重新联网”会让手机重新连接移动网络，然后比较操作前后的公网 IPv4 / IPv6。运营商是否分配新地址由运营商决定，所以软件会分别显示“网络已恢复”和“公网 IP 是否变化”，不会把重连成功当成换 IP 成功。

出口模式有“自动、IPv4、IPv6”三种。这里控制的是代理连接使用哪种地址族，不会凭空给只提供 IPv6 的电话卡生成 IPv4。

## 独占模式

默认情况下，Windows 上没有设置代理的软件仍可能直接使用手机的 USB 网卡。

开启“独占模式”后，USBridge 会请求一次管理员权限，并通过 Windows Filtering Platform 限制这张 USB 网卡：只有 USBridge 自己可以直接连接，其他软件必须走 `18080` 或 `18081`。电脑本身的 Wi-Fi、以太网和 VPN 不受影响。

## 本机接口

Windows 端在 `http://127.0.0.1:18082` 提供控制接口，只监听本机且不需要鉴权。接口清单可直接读取：

```powershell
curl.exe http://127.0.0.1:18082/v1/help
```

例如重新连接移动网络：

```powershell
curl.exe -X POST -H "Content-Type: application/json" -d "{}" http://127.0.0.1:18082/v1/mobile/reconnect
```

## 兼容性

手机端通过 [libsu](https://github.com/topjohnwu/libsu) 请求 Root，不绑定某一个 Root 管理器。Magisk、KernelSU、APatch 以及其他兼容实现都可以尝试；具体的移动数据和 USB 共享命令仍会受手机厂商 ROM 影响。

流量统计保存在手机和电脑本地。手机控制服务只接受来自 USB 共享接口的请求，电脑端代理和控制接口只监听 `127.0.0.1`。

## 从源码构建

需要 JDK 21、Android SDK 36.1、Go 1.25、Node.js 22 和 npm。

```powershell
# Android
.\gradlew.bat test assembleDebug

# Windows 后端
Set-Location desktop
go test ./...
go vet ./...

# Windows 前端
Set-Location frontend
npm ci
npm run build
```

Wails 正式构建和发布签名见 [docs/RELEASING.md](docs/RELEASING.md)。问题反馈请使用 [Issues](https://github.com/jmcgillcoder/USBridge/issues)，安全问题请按 [SECURITY.md](SECURITY.md) 私密报告。

## License

[Apache License 2.0](LICENSE)
