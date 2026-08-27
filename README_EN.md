# USBridge

[简体中文](README.md) | **English**

USBridge lets Windows applications use an Android phone's mobile connection through USB tethering and exposes that connection as local HTTP and SOCKS5 proxies.

The Android and Windows apps are designed to run together. The current release supports **rooted Android 11+** phones and **Windows x64**. There is no account, cloud service, or remote control component.

## Screenshots

### Windows

![USBridge Windows connection screen](docs/images/desktop-connection.png)

### Android

<img src="docs/images/android-automation.png" alt="USBridge Android USB automation screen" width="360">

## Download

Download matching Android and Windows builds from [Releases](https://github.com/jmcgillcoder/USBridge/releases/latest).

- Install the APK on the phone and grant root access on first launch.
- Run the Windows EXE directly; no installer is required.
- The Windows build is not yet signed with a commercial code-signing certificate, so SmartScreen may warn on first launch.

## Quick Start

1. Connect the phone to the PC with a USB data cable.
2. Open USBridge on Android and enable USB tethering. Automatic tethering can be enabled in the app settings.
3. Open USBridge on Windows and wait for it to identify the phone's USB network adapter.
4. Configure the application that should use the phone connection with one of these local proxies:

| Protocol | Address | Credentials |
| --- | --- | --- |
| HTTP / SOCKS5 | `127.0.0.1:18080` | None |
| HTTP / SOCKS5 | `127.0.0.1:18081` | `usbridge` / `usbridge_pw` |

Both ports accept HTTP, HTTPS CONNECT, and SOCKS5 on the same port. If the phone connection becomes unavailable, the proxies fail closed instead of falling back to another Windows network.

## Reconnecting and IP Changes

The reconnect action cycles the phone's mobile connection and compares public IPv4 and IPv6 addresses before and after the operation. A successful reconnect does not guarantee that the carrier will assign a new address, so USBridge reports network recovery and IP changes separately.

The available connection modes are Auto, IPv4, and IPv6. They select the address family used by proxy connections; they do not provide IPv4 service when the carrier only offers IPv6.

## Exclusive Mode

Without exclusive mode, Windows applications that do not use a proxy may still connect directly through the phone's USB adapter.

Exclusive mode requests administrator permission once and installs dynamic Windows Filtering Platform rules for the selected adapter. Only USBridge can connect directly through that adapter; other applications must use port `18080` or `18081`. Other Wi-Fi, Ethernet, and VPN interfaces are not affected.

## Local Control API

The Windows app exposes an unauthenticated local API at `http://127.0.0.1:18082`. It only listens on the loopback interface. Read the live endpoint list with:

```powershell
curl.exe http://127.0.0.1:18082/v1/help
```

Reconnect the mobile network with:

```powershell
curl.exe -X POST -H "Content-Type: application/json" -d "{}" http://127.0.0.1:18082/v1/mobile/reconnect
```

## Root Compatibility

The Android app requests root through [libsu](https://github.com/topjohnwu/libsu) and is not tied to one root manager. Magisk, KernelSU, APatch, and other compatible implementations can be used. Mobile-data and tethering commands may still vary between vendor ROMs.

Traffic history stays on the phone and PC. The phone control service only accepts requests from the USB tethering interface, while the Windows proxies and control API only listen on `127.0.0.1`.

## Building from Source

The build requires JDK 21, Android SDK 36.1, Go 1.25, Node.js 22, and npm.

```powershell
# Android
.\gradlew.bat test assembleDebug

# Windows backend
Set-Location desktop
go test ./...
go vet ./...

# Windows frontend
Set-Location frontend
npm ci
npm run build
```

See [docs/RELEASING.md](docs/RELEASING.md) for Wails release builds and signing. Use [Issues](https://github.com/jmcgillcoder/USBridge/issues) for regular bug reports and follow [SECURITY.md](SECURITY.md) for private security reports.

## License

[Apache License 2.0](LICENSE)
