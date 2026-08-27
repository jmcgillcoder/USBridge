# 参与贡献

感谢参与 USBridge 开发。

## 开发流程

1. Fork 仓库并从 `main` 创建分支。
2. 保持改动聚焦，新增或修改行为时同步补充测试。
3. 提交前运行 Android 单元测试、Go 测试与静态检查、前端构建。
4. 发起 Pull Request，说明问题、实现方式、验证结果及涉及的设备或 Windows 版本。

```powershell
.\gradlew.bat test
Set-Location desktop
go test ./...
go vet ./...
Set-Location frontend
npm ci
npm run build
```

不要提交签名密钥、密码、令牌、个人设备信息、构建产物或 IDE 本地配置。安全漏洞请遵循 [SECURITY.md](SECURITY.md) 私密报告。

提交代码即表示你同意按项目的 Apache License 2.0 许可证提供贡献。
