# USBridge 发布签名

开源项目仍然需要为“官方二进制”签名。签名证明 APK/EXE 来自同一个发布者，也让后续升级可以验证发布者身份；它不限制别人阅读源码或自行构建。

仓库只保存签名流程。JKS、PFX、私钥和密码不得提交到 Git，也不要通过聊天、Issue 或构建日志传递。

## Android 官方签名

首次正式发布前，在离线或可信电脑上生成一次长期密钥：

```powershell
keytool -genkeypair -v -storetype JKS -keystore usbridge-release.jks -alias usbridge-release -keyalg RSA -keysize 4096 -validity 10000
```

至少保存两份加密备份，并把密码放入密码管理器。Android 应用发布后必须一直使用同一把密钥；丢失密钥将无法为现有安装提供可直接升级的新 APK。

构建前设置以下进程环境变量：

```powershell
$env:USBRIDGE_ANDROID_KEYSTORE = 'D:\private\usbridge-release.jks'
$env:USBRIDGE_ANDROID_STORE_PASSWORD = '<store password>'
$env:USBRIDGE_ANDROID_KEY_ALIAS = 'usbridge-release'
$env:USBRIDGE_ANDROID_KEY_PASSWORD = '<key password>'
$env:USBRIDGE_ANDROID_STORE_TYPE = 'JKS' # 可省略，默认 JKS
```

四个必填变量必须同时存在。缺少全部变量时可以构建明确标记为 `unsigned` 的测试产物；只配置一部分时构建会直接失败，避免误发包。

## Windows 官方签名

公开分发时应使用受 Windows 信任的代码签名证书，例如可信 CA 的 OV/EV 证书、Microsoft Trusted Signing，或符合条件时申请面向开源项目的 SignPath Foundation。自签名证书只适合内部测试，不能正常消除其他电脑上的“未知发布者”提示。

发布脚本支持两种方式，二选一：

```powershell
# PFX 文件
$env:USBRIDGE_WINDOWS_CERTIFICATE = 'D:\private\usbridge-code-signing.pfx'
$env:USBRIDGE_WINDOWS_CERT_PASSWORD = '<pfx password>'

# 或使用已安装在 Windows 证书存储中的证书
$env:USBRIDGE_WINDOWS_CERT_THUMBPRINT = '<SHA1 thumbprint>'
```

脚本会使用 SHA-256 和 RFC 3161 时间戳，并在完成后调用 `signtool verify`。签名证书到期后，带有效时间戳的旧版本仍可验证。普通 OV 证书签名后 SmartScreen 声誉仍可能需要逐步积累，这不等于签名失败。

## 构建正式产物

在项目根目录执行：

```powershell
.\scripts\build-release.ps1 -Version 0.3.0 -AndroidVersionCode 4
```

输出位于 `dist`，文件名会明确包含 `signed` 或 `unsigned`，并生成 `SHA256SUMS.txt`。只有通过签名验证的 `signed` 产物才应作为官方版本发布。

每次升级版本时同步修改：

- `app/build.gradle.kts` 中的默认 `versionName` 与 `versionCode`；
- `desktop/wails.json` 中的 `productVersion`；
- 桌面前端 `package.json` 和界面显示版本。

如果以后接入 GitHub Actions 或其他 CI，把同名环境变量放入加密 Secrets，并直接调用同一个发布脚本。生产私钥不应存放在公开仓库或普通 CI 构建缓存中。
