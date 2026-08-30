import type { AppBridge, AuthenticatedProxyAccess, Snapshot } from './types'

const developmentAuthenticatedProxy: AuthenticatedProxyAccess = {
	listen: '127.0.0.1:18081',
	username: 'usbridge',
	password: 'usbridge_pw',
	httpUrl: 'http://usbridge:usbridge_pw@127.0.0.1:18081',
	socks5Url: 'socks5://usbridge:usbridge_pw@127.0.0.1:18081',
}

const developmentSnapshot: Snapshot = {
  adapters: [],
  ipMode: 'auto',
  androidReady: false,
  proxyListen: '127.0.0.1:18080',
  proxyRunning: false,
	authenticatedProxyListen: '127.0.0.1:18081',
	authenticatedProxyRunning: false,
	exclusiveModeSupported: true,
	exclusiveModeEnabled: false,
	exclusiveModeActive: false,
	systemProxyActive: false,
	controlListen: '127.0.0.1:18082',
  controlRunning: false,
  networkChanging: false,
  traffic: {
    startedAtMillis: Date.now(),
    activeConnections: 0,
    totalConnections: 0,
    uploadBytes: 0,
    downloadBytes: 0,
    uploadBytesPerSecond: 0,
    downloadBytesPerSecond: 0,
    http: { connections: 0, uploadBytes: 0, downloadBytes: 0 },
    socks5: { connections: 0, uploadBytes: 0, downloadBytes: 0 },
  },
}

const developmentBridge: AppBridge = {
  async GetSnapshot() { return developmentSnapshot },
	async GetAuthenticatedProxyAccess() { return developmentAuthenticatedProxy },
  async CheckForUpdates() { return { currentVersion: '0.3.1', latestVersion: '0.3.1', available: false } },
  async InstallUpdate() {},
  async OpenProjectPage() {},
  async RefreshAdapters() {},
  async SelectAdapter() {},
  async SetExclusiveMode(enabled) {
	developmentSnapshot.exclusiveModeEnabled = enabled
	developmentSnapshot.exclusiveModeActive = enabled && Boolean(developmentSnapshot.selectedAdapter)
	developmentSnapshot.systemProxyActive = developmentSnapshot.exclusiveModeActive
  },
  async SetIPMode(mode) { developmentSnapshot.ipMode = mode as Snapshot['ipMode'] },
  async ResetTraffic() {},
  async ReconnectMobile() { return { ok: false, message: '开发预览未连接手机' } },
  async RefreshPublicIP() { return { ok: false, message: '开发预览未连接手机' } },
  async StartTethering() { return { ok: false, message: '开发预览未连接手机' } },
  async StopTethering() { return { ok: false, message: '开发预览未连接手机' } },
}

function bindingUnavailable(): Promise<never> {
  return Promise.reject(new Error('桌面后台绑定不可用，请重启应用'))
}

const unavailableBridge: AppBridge = {
  GetSnapshot: bindingUnavailable,
	GetAuthenticatedProxyAccess: bindingUnavailable,
  CheckForUpdates: bindingUnavailable,
  InstallUpdate: bindingUnavailable,
  OpenProjectPage: bindingUnavailable,
  RefreshAdapters: bindingUnavailable,
  SelectAdapter: bindingUnavailable,
  SetExclusiveMode: bindingUnavailable,
  SetIPMode: bindingUnavailable,
  ResetTraffic: bindingUnavailable,
  ReconnectMobile: bindingUnavailable,
  RefreshPublicIP: bindingUnavailable,
  StartTethering: bindingUnavailable,
  StopTethering: bindingUnavailable,
}

export function getBridge(): AppBridge {
  const bound = window.go?.main?.App
  if (bound) return bound
  return import.meta.env.DEV ? developmentBridge : unavailableBridge
}
