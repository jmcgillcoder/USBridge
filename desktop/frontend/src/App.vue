<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { getBridge } from './bridge'
import MaterialIcon from './components/MaterialIcon.vue'
import type { MaterialSymbolName } from './materialSymbols'
import type { Adapter, AuthenticatedProxyAccess, OperationResponse, Snapshot, TrafficSnapshot } from './types'

type Page = 'connection' | 'traffic' | 'settings'
type ControlExample = 'windows' | 'python' | 'go'
type NotificationKind = 'success' | 'error' | 'info'

interface AppNotification {
  id: number
  message: string
  kind: NotificationKind
}

const bridge = getBridge()
const currentPage = ref<Page>('connection')
const snapshot = ref<Snapshot>({
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
	controlListen: '127.0.0.1:18082',
  controlRunning: false,
  networkChanging: false,
  traffic: emptyTraffic(),
})
const contentElement = ref<HTMLElement | null>(null)
const busy = ref('')
const appNotification = ref<AppNotification | null>(null)
const lastReconnect = ref<OperationResponse | null>(null)
const copiedControlExample = ref(false)
const copiedProxyItem = ref('')
const authenticatedProxy = ref<AuthenticatedProxyAccess | null>(null)
const authenticatedProxyAccessError = ref('')
const authenticatedProxyAccessLoading = ref(false)
const activeControlExample = ref<ControlExample>('windows')
const failedPolls = ref(0)
const now = ref(Date.now())
const confirmingTrafficReset = ref(false)
let pollingHandle: number | undefined
let copyNoticeHandle: number | undefined
let proxyCopyNoticeHandle: number | undefined
let notificationHandle: number | undefined
let trafficResetConfirmHandle: number | undefined
let notificationId = 0
let snapshotSequence = 0
let appliedSnapshotSequence = 0

const controlEndpoints = [
  { method: 'GET', path: '/v1/status', label: '完整运行状态' },
  { method: 'POST', path: '/v1/mobile/reconnect', label: '重新联网 / 换 IP' },
  { method: 'POST', path: '/v1/public-ip/refresh', label: '刷新公网 IP' },
  { method: 'POST', path: '/v1/upstream/cellular', label: '强制使用蜂窝上游' },
  { method: 'POST', path: '/v1/tether/start', label: '开启 USB 共享' },
  { method: 'POST', path: '/v1/tether/stop', label: '关闭 USB 共享' },
  { method: 'PUT', path: '/v1/ip-mode', label: '设置 auto / ipv4 / ipv6' },
  { method: 'GET', path: '/v1/traffic', label: 'Windows 代理流量' },
  { method: 'GET', path: '/v1/help', label: '机器可读接口说明' },
] as const

const navigation: Array<{ id: Page; label: string; icon: MaterialSymbolName }> = [
  { id: 'connection', label: '连接', icon: 'cable' },
  { id: 'traffic', label: '流量', icon: 'monitoring' },
  { id: 'settings', label: '设置', icon: 'settings' },
]

const pageMetadata = computed(() => {
  if (currentPage.value === 'traffic') {
    return { eyebrow: '流量统计', title: '代理流量', subtitle: '仅统计通过 USBridge 本机代理的流量' }
  }
  if (currentPage.value === 'settings') {
    return { eyebrow: '应用设置', title: '设置', subtitle: '代理、网络协议和手机 USB 设备' }
  }
  return { eyebrow: '移动网络', title: '手机网络桥接', subtitle: '让电脑应用只通过手机移动网络访问互联网' }
})

const selected = computed(() => snapshot.value.selectedAdapter)
const connected = computed(() => Boolean(selected.value && selected.value.status.toLowerCase() === 'up'))
const phoneReady = computed(() => Boolean(snapshot.value.androidReady && snapshot.value.androidStatus))
const phoneControllable = computed(() => Boolean(phoneReady.value && snapshot.value.androidStatus?.root.granted))
const tetheringEnabled = computed(() => Boolean(phoneReady.value && snapshot.value.androidStatus?.usb.tetheringEnabled))
const cellularUpstream = computed(() => Boolean(phoneReady.value && snapshot.value.androidStatus?.usb.cellularUpstream))
const ipModeConflict = computed(() => {
  const status = snapshot.value.androidStatus
  const mobile = status?.mobile
  if (!mobile) return ''
  const ipv4Available = mobile.ipv4Available ?? Boolean(status.publicIp?.ipv4)
  const ipv6Available = mobile.ipv6Available ?? Boolean(status.publicIp?.ipv6)
  if (snapshot.value.ipMode === 'ipv6' && !ipv6Available && ipv4Available) {
    return 'USB 共享出口当前仅提供 IPv4，IPv6 模式无法联网。'
  }
  return ''
})
const proxyUsable = computed(() => Boolean(
  snapshot.value.proxyRunning &&
  tetheringEnabled.value &&
  cellularUpstream.value &&
  !ipModeConflict.value,
))
const phoneStatusLabel = computed(() => {
  if (!connected.value) return '等待手机'
  if (!phoneReady.value) return '手机端未连接'
  if (!snapshot.value.androidStatus?.root.granted) return '需要 Root 授权'
  if (ipModeConflict.value) return '协议不匹配'
  if (!tetheringEnabled.value) return '网络共享未开启'
  if (!cellularUpstream.value) return '正在切换移动网络'
  return '移动网络已就绪'
})
const phoneStatusDetail = computed(() => {
  if (!connected.value) return '连接手机并开启 USB 网络共享后即可控制。'
  if (!phoneReady.value) return '请打开手机端 USBridge，并确认手机仍通过 USB 连接。'
  if (!snapshot.value.androidStatus?.root.granted) return '请在手机的 Root 管理器中允许 USBridge。'
  if (ipModeConflict.value) return `${ipModeConflict.value}请选择自动或当前网络支持的协议。`
  if (!tetheringEnabled.value) return '请先开启手机的 USB 网络共享。'
  if (!cellularUpstream.value) return '电脑流量已暂停，等待手机切换到移动数据出口。'
  return '可以重新联网、更换公网 IP 或控制 USB 网络共享。'
})
const connectionTitle = computed(() => {
  if (proxyUsable.value) return '电脑正在使用手机移动网络'
  if (connected.value) return '正在建立移动网络连接'
  return '连接手机后自动识别'
})
const connectionDescription = computed(() => {
  if (proxyUsable.value) return 'HTTP、HTTPS 和 SOCKS5 连接均锁定到手机 USB 网络。'
  if (ipModeConflict.value) return '当前协议与手机卡提供的网络不匹配，代理流量已暂停。'
  if (connected.value) return '已发现手机 USB 设备，正在确认移动数据出口。'
  return '请用 USB 连接手机，并在手机端开启 USBridge。'
})
const publicIp = computed(() => snapshot.value.androidStatus?.publicIp)
const publicIPv4 = computed(() => publicIp.value?.ipv4 || '')
const publicIPv6 = computed(() => publicIp.value?.ipv6 || '')
const selectedLocalAddresses = computed(() => {
  if (!selected.value) return ''
  const ipv4 = selected.value.ipv4?.[0]
  const linkLocalIPv6 = selected.value.ipv6?.find((address) => address.toLowerCase().startsWith('fe80:'))
  return [ipv4, linkLocalIPv6].filter(Boolean).join(' / ')
})
const publicIpHint = computed(() => {
  if (!phoneReady.value) return '连接手机后自动读取'
  if (!publicIPv4.value && !publicIPv6.value) return '正在通过手机移动网络检测'
  return '由手机蜂窝网络独立检测'
})
const traffic = computed(() => snapshot.value.traffic ?? emptyTraffic())
const totalBytes = computed(() => traffic.value.uploadBytes + traffic.value.downloadBytes)
const openHTTPProxyURL = computed(() => `http://${snapshot.value.proxyListen || '127.0.0.1:18080'}`)
const openSOCKS5ProxyURL = computed(() => `socks5://${snapshot.value.proxyListen || '127.0.0.1:18080'}`)
const controlBaseUrl = computed(() => `http://${snapshot.value.controlListen || '127.0.0.1:18082'}`)
const controlExamples = computed<Record<ControlExample, { label: string; hint: string; code: string }>>(() => {
  const url = `${controlBaseUrl.value}/v1/mobile/reconnect`
  return {
    windows: {
      label: 'Windows',
      hint: 'PowerShell / CMD · 系统自带 curl.exe',
      code: `curl.exe -X POST -H "Content-Type: application/json" ${url}`,
    },
    python: {
      label: 'Python',
      hint: 'requests',
      code: `import requests\n\nresult = requests.post("${url}", json={}, timeout=240).json()`,
    },
    go: {
      label: 'Go',
      hint: 'net/http 标准库',
      code: `req, _ := http.NewRequest(http.MethodPost, "${url}", http.NoBody)\nreq.Header.Set("Content-Type", "application/json")\nresp, err := (&http.Client{Timeout: 4 * time.Minute}).Do(req)`,
    },
  }
})
const activeControlExampleInfo = computed(() => controlExamples.value[activeControlExample.value])
const notificationPresentation = computed<{ title: string; icon: MaterialSymbolName }>(() => {
  if (appNotification.value?.kind === 'error') return { title: '操作未完成', icon: 'error' }
  if (appNotification.value?.kind === 'info') return { title: '提示', icon: 'info' }
  return { title: '操作完成', icon: 'checkCircle' }
})
const reconnectIpRows = computed(() => {
  const reconnect = lastReconnect.value
  if (!reconnect) return []
  return [
    { label: 'IPv4', before: reconnect.before?.ipv4, after: reconnect.after?.ipv4 },
    { label: 'IPv6', before: reconnect.before?.ipv6, after: reconnect.after?.ipv6 },
  ].filter((item) => item.before || item.after)
})
const visibleAdapters = computed(() => {
  const candidates = snapshot.value.adapters.filter((item) => item.usbCandidate)
  if (candidates.length > 0) return candidates
  return snapshot.value.adapters.filter((item) => item.status.toLowerCase() === 'up').slice(0, 5)
})
const actionsLocked = computed(() => Boolean(busy.value) || snapshot.value.networkChanging)
const pollingLost = computed(() => failedPolls.value >= 3)
const appVersion = computed(() => snapshot.value.version || __APP_VERSION__)
const exclusiveModePresentation = computed(() => {
  if (!snapshot.value.exclusiveModeSupported) {
    return { label: '不可用', hero: '当前不可用', detail: '此功能需要 Windows 10 或更高版本' }
  }
  if (snapshot.value.exclusiveModeError) {
    return { label: '未生效', hero: '保护未生效', detail: '请到设置查看' }
  }
  if (!snapshot.value.exclusiveModeEnabled) {
    return { label: '已关闭', hero: '允许直接使用', detail: '其他应用可直接使用手机网卡' }
  }
  if (snapshot.value.exclusiveModeActive) {
    return { label: '已生效', hero: 'USBridge 独占', detail: '其他应用需通过本机代理' }
  }
  if (!selected.value) {
    return { label: '等待 USB', hero: '等待手机 USB', detail: '连接后自动生效' }
  }
  return { label: '正在启用', hero: '正在保护网卡', detail: '等待 Windows 确认规则' }
})

async function refresh(reportError = false) {
  const sequence = ++snapshotSequence
  try {
    const next = await bridge.GetSnapshot()
    failedPolls.value = 0
    if (sequence <= appliedSnapshotSequence) return
		appliedSnapshotSequence = sequence
		snapshot.value = next
		if (next.authenticatedProxyRunning && !authenticatedProxy.value) {
			void loadAuthenticatedProxyAccess()
		}
  } catch (error) {
    failedPolls.value += 1
    if (reportError) showNotification(errorMessage(error), 'error')
  }
}

async function loadAuthenticatedProxyAccess() {
	if (authenticatedProxyAccessLoading.value) return
	authenticatedProxyAccessLoading.value = true
	try {
		authenticatedProxy.value = await bridge.GetAuthenticatedProxyAccess()
		authenticatedProxyAccessError.value = ''
	} catch (error) {
		authenticatedProxy.value = null
		authenticatedProxyAccessError.value = errorMessage(error)
	} finally {
		authenticatedProxyAccessLoading.value = false
	}
}

function dismissNotification() {
  if (notificationHandle !== undefined) window.clearTimeout(notificationHandle)
  notificationHandle = undefined
  appNotification.value = null
}

function showNotification(message: string, kind: NotificationKind = 'success') {
  const normalizedMessage = message.trim()
  if (!normalizedMessage) return
  dismissNotification()
  appNotification.value = { id: ++notificationId, message: normalizedMessage, kind }
  notificationHandle = window.setTimeout(
    dismissNotification,
    kind === 'error' ? 7000 : kind === 'info' ? 5000 : 4000,
  )
}

async function runAction(name: string, action: () => Promise<unknown>, success: string) {
  if (busy.value) return
  busy.value = name
  dismissNotification()
  try {
    const result = await action()
    const message = typeof result === 'object' && result && 'message' in result
      ? String(result.message)
      : success
    const actionMessage = message || success
    let actionIsError = false
    if (typeof result === 'object' && result && 'ok' in result) {
      actionIsError = !Boolean(result.ok)
    }
    await refresh()
    showNotification(actionMessage, actionIsError ? 'error' : 'success')
  } catch (error) {
    showNotification(errorMessage(error), 'error')
  } finally {
    busy.value = ''
  }
}

function reconnectMobile() {
  void runAction(
    'reconnect',
    async () => {
      const result = await bridge.ReconnectMobile()
      lastReconnect.value = result
      return result
    },
    '移动网络重连完成',
  )
}

function refreshPublicIp() {
  void runAction('public-ip', () => bridge.RefreshPublicIP(), '公网 IP 已刷新')
}

function setMode(mode: Snapshot['ipMode']) {
  void runAction('mode', () => bridge.SetIPMode(mode), `已切换为 ${modeLabel(mode)}`)
}

function setExclusiveMode(enabled: boolean) {
  void runAction(
    'exclusive-mode',
    () => bridge.SetExclusiveMode(enabled),
    enabled ? '独占模式已开启' : '独占模式已关闭',
  )
}

function chooseAdapter(adapter: Adapter) {
  void runAction('adapter', () => bridge.SelectAdapter(adapter.id), `已选择 ${adapter.name}`)
}

function chooseAutomatic() {
  void runAction('adapter', () => bridge.SelectAdapter('auto'), '已启用自动识别')
}

function refreshAdapters() {
  void runAction('refresh', () => bridge.RefreshAdapters(), '网卡列表已刷新')
}

function resetTraffic() {
  if (!confirmingTrafficReset.value) {
    confirmingTrafficReset.value = true
    if (trafficResetConfirmHandle !== undefined) window.clearTimeout(trafficResetConfirmHandle)
    trafficResetConfirmHandle = window.setTimeout(() => { confirmingTrafficReset.value = false }, 4000)
    return
  }
  if (trafficResetConfirmHandle !== undefined) window.clearTimeout(trafficResetConfirmHandle)
  trafficResetConfirmHandle = undefined
  confirmingTrafficReset.value = false
  void runAction('traffic-reset', () => bridge.ResetTraffic(), '流量统计已清零')
}

async function copyReconnectCommand() {
  try {
    await navigator.clipboard.writeText(activeControlExampleInfo.value.code)
    copiedControlExample.value = true
    if (copyNoticeHandle !== undefined) window.clearTimeout(copyNoticeHandle)
    copyNoticeHandle = window.setTimeout(() => { copiedControlExample.value = false }, 1800)
  } catch (error) {
    showNotification(`复制失败：${errorMessage(error)}`, 'error')
  }
}

async function copyProxyValue(value: string, label: string, item: string) {
	if (!value) return
	try {
		await navigator.clipboard.writeText(value)
		copiedProxyItem.value = item
		if (proxyCopyNoticeHandle !== undefined) window.clearTimeout(proxyCopyNoticeHandle)
		proxyCopyNoticeHandle = window.setTimeout(() => { copiedProxyItem.value = '' }, 1800)
		showNotification(`${label}已复制`)
	} catch (error) {
		showNotification(`复制失败：${errorMessage(error)}`, 'error')
	}
}

function modeLabel(mode: Snapshot['ipMode']) {
  return mode === 'ipv4' ? 'IPv4' : mode === 'ipv6' ? 'IPv6' : '自动'
}

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value >= 100 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[index]}`
}

function formatDuration(startedAtMillis: number) {
  if (!startedAtMillis) return '0 分钟'
  const seconds = Math.max(0, Math.floor((now.value - startedAtMillis) / 1000))
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`
  if (minutes > 0) return `${minutes} 分钟`
  return `${seconds} 秒`
}

function emptyTraffic(): TrafficSnapshot {
  return {
    startedAtMillis: Date.now(),
    activeConnections: 0,
    totalConnections: 0,
    uploadBytes: 0,
    downloadBytes: 0,
    uploadBytesPerSecond: 0,
    downloadBytesPerSecond: 0,
    http: { connections: 0, uploadBytes: 0, downloadBytes: 0 },
    socks5: { connections: 0, uploadBytes: 0, downloadBytes: 0 },
  }
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

watch(currentPage, async () => {
  await nextTick()
  contentElement.value?.scrollTo({ top: 0 })
})

onMounted(() => {
  now.value = Date.now()
  void refresh(true)
	void loadAuthenticatedProxyAccess()
  pollingHandle = window.setInterval(() => {
    now.value = Date.now()
    void refresh()
  }, 3000)
})

onUnmounted(() => {
  if (pollingHandle !== undefined) window.clearInterval(pollingHandle)
  if (copyNoticeHandle !== undefined) window.clearTimeout(copyNoticeHandle)
	if (proxyCopyNoticeHandle !== undefined) window.clearTimeout(proxyCopyNoticeHandle)
  if (notificationHandle !== undefined) window.clearTimeout(notificationHandle)
  if (trafficResetConfirmHandle !== undefined) window.clearTimeout(trafficResetConfirmHandle)
})
</script>

<template>
  <div class="app-shell">
    <Teleport to="body">
      <div class="notification-region" aria-live="polite" aria-atomic="true">
        <Transition name="notification" mode="out-in">
          <section
            v-if="appNotification"
            :key="appNotification.id"
            class="app-notification"
            :class="`app-notification--${appNotification.kind}`"
            :role="appNotification.kind === 'error' ? 'alert' : 'status'"
          >
            <span class="app-notification__icon" aria-hidden="true">
              <MaterialIcon :name="notificationPresentation.icon" :size="24" />
            </span>
            <div class="app-notification__copy">
              <strong>{{ notificationPresentation.title }}</strong>
              <p>{{ appNotification.message }}</p>
            </div>
            <button class="app-notification__close" title="关闭提示" aria-label="关闭提示" @click="dismissNotification">
              <MaterialIcon name="close" :size="20" />
            </button>
          </section>
        </Transition>
      </div>
    </Teleport>

    <aside class="rail">
      <div class="brand-mark" aria-hidden="true"><MaterialIcon name="usb" :size="28" /></div>
      <button
        v-for="item in navigation"
        :key="item.id"
        class="rail-item"
        :class="{ 'rail-item--active': currentPage === item.id }"
        :aria-current="currentPage === item.id ? 'page' : undefined"
        @click="currentPage = item.id"
      >
        <span class="rail-icon"><MaterialIcon :name="item.icon" :size="24" /></span>
        <span>{{ item.label }}</span>
      </button>
      <div class="rail-spacer" />
      <span class="version">{{ appVersion }}</span>
    </aside>

    <main ref="contentElement" class="content">
      <section v-if="pollingLost" class="inline-error-banner" role="alert">
        <span class="inline-error-banner__icon" aria-hidden="true"><MaterialIcon name="error" :size="20" /></span>
        <p>无法连接后台服务，显示的数据可能已过期</p>
      </section>
      <section v-if="snapshot.lastError" class="inline-error-banner" role="alert">
        <span class="inline-error-banner__icon" aria-hidden="true"><MaterialIcon name="error" :size="20" /></span>
        <p>{{ snapshot.lastError }}</p>
      </section>

      <header class="page-header">
        <div>
          <p class="eyebrow">{{ pageMetadata.eyebrow }}</p>
          <h1>{{ pageMetadata.title }}</h1>
          <p>{{ pageMetadata.subtitle }}</p>
        </div>
        <button
          v-if="currentPage !== 'traffic'"
          class="icon-button"
          :disabled="actionsLocked"
          title="重新扫描手机 USB 设备"
          @click="refreshAdapters"
        >
          <MaterialIcon name="refresh" :size="24" />
        </button>
        <button
          v-else
          class="text-button header-action"
          :disabled="actionsLocked"
          @click="resetTraffic"
        >
          {{ confirmingTrafficReset ? '确认清零' : '清零统计' }}
        </button>
      </header>

      <Transition name="page" mode="out-in">
        <div :key="currentPage" class="page-panel">
          <template v-if="currentPage === 'connection'">
            <section class="hero-card">
              <div class="hero-icon" :class="{ 'hero-icon--waiting': !connected }"><MaterialIcon name="usb" :size="34" /></div>
              <div class="hero-copy">
                <div class="status-line">
                  <span class="status-pill" :class="{ 'status-pill--muted': !connected }">
                    {{ connected ? 'USB 已连接' : '等待 USB' }}
                  </span>
                  <span class="quiet-label">{{ phoneStatusLabel }}</span>
                  <span v-if="snapshot.proxyRunning" class="quiet-label">代理服务已启动</span>
                </div>
                <h2>{{ connectionTitle }}</h2>
                <p>{{ connectionDescription }}</p>
                <span v-if="selected" class="connection-device">已自动识别手机 USB 设备：{{ selected.name }}</span>
              </div>
              <div class="hero-policy">
                <span>独占模式</span>
                <strong>{{ exclusiveModePresentation.hero }}</strong>
                <small>{{ exclusiveModePresentation.detail }}</small>
              </div>
            </section>

            <section class="section-block public-ip-section">
              <div class="section-heading">
                <div>
                  <h3>当前公网 IP</h3>
                  <p>这里显示运营商分配的公网地址，不是 USB 局域网地址</p>
                </div>
                <div class="public-ip-heading-actions">
                  <span class="public-ip-source">{{ publicIpHint }}</span>
                  <button class="text-button compact-button" :disabled="!phoneControllable || actionsLocked" @click="refreshPublicIp">
                    {{ busy === 'public-ip' ? '正在刷新' : '刷新' }}
                  </button>
                </div>
              </div>
              <div class="public-ip-grid">
                <article :class="{ 'public-ip-card--empty': !publicIPv4 }">
                  <div class="public-ip-label"><span>IPv4</span><small>公网地址</small></div>
                  <code>{{ publicIPv4 || '暂未获得' }}</code>
                </article>
                <article :class="{ 'public-ip-card--empty': !publicIPv6 }">
                  <div class="public-ip-label"><span>IPv6</span><small>公网地址</small></div>
                  <code>{{ publicIPv6 || '暂未获得' }}</code>
                </article>
              </div>
            </section>

            <section class="section-block">
              <div class="section-heading">
                <div>
                  <h3>连接协议</h3>
                  <p>选择网站解析和连接使用的公网 IP 协议</p>
                </div>
                <div class="segmented" aria-label="IP 协议模式">
                  <button
                    v-for="mode in (['auto', 'ipv4', 'ipv6'] as const)"
                    :key="mode"
                    :class="{ selected: snapshot.ipMode === mode }"
                    :disabled="actionsLocked"
                    @click="setMode(mode)"
                  >
                    {{ modeLabel(mode) }}
                  </button>
                </div>
              </div>
              <div v-if="ipModeConflict" class="inline-error-banner family-warning" role="alert">
                <span class="inline-error-banner__icon" aria-hidden="true"><MaterialIcon name="error" :size="20" /></span>
                <p>{{ ipModeConflict }}建议使用自动模式。</p>
                <button class="tonal-button compact-button" :disabled="actionsLocked" @click="setMode('auto')">切换为自动</button>
              </div>
            </section>

			<section class="section-block proxy-access-section">
			  <div class="section-heading">
				<div><h3>本机代理</h3><p>HTTP、HTTPS 和 SOCKS5 可共用同一个端口</p></div>
				<span class="status-pill" :class="{ 'status-pill--muted': !proxyUsable }">{{ proxyUsable ? '可用' : '等待网络' }}</span>
			  </div>
			  <div class="proxy-access-grid">
				<article class="proxy-access-card">
				  <div class="proxy-access-heading">
					<div><strong>免密代理</strong><span>适合只在本机使用</span></div>
					<span class="status-pill" :class="{ 'status-pill--muted': !snapshot.proxyRunning }">{{ snapshot.proxyRunning ? '18080 已启动' : '未启动' }}</span>
				  </div>
				  <div class="proxy-access-rows">
					<div class="proxy-access-row">
					  <span>HTTP / HTTPS</span><code>{{ openHTTPProxyURL }}</code>
					  <button class="icon-button proxy-copy-button" :title="copiedProxyItem === 'open-http' ? '已复制' : '复制 HTTP 代理'" :aria-label="copiedProxyItem === 'open-http' ? '已复制 HTTP 代理' : '复制 HTTP 代理'" @click="copyProxyValue(openHTTPProxyURL, 'HTTP 代理', 'open-http')"><MaterialIcon :name="copiedProxyItem === 'open-http' ? 'checkCircle' : 'contentCopy'" :size="19" /></button>
					</div>
					<div class="proxy-access-row">
					  <span>SOCKS5</span><code>{{ openSOCKS5ProxyURL }}</code>
					  <button class="icon-button proxy-copy-button" :title="copiedProxyItem === 'open-socks' ? '已复制' : '复制 SOCKS5 代理'" :aria-label="copiedProxyItem === 'open-socks' ? '已复制 SOCKS5 代理' : '复制 SOCKS5 代理'" @click="copyProxyValue(openSOCKS5ProxyURL, 'SOCKS5 代理', 'open-socks')"><MaterialIcon :name="copiedProxyItem === 'open-socks' ? 'checkCircle' : 'contentCopy'" :size="19" /></button>
					</div>
				  </div>
				</article>

				<article class="proxy-access-card proxy-access-card--authenticated">
				  <div class="proxy-access-heading">
					<div><strong>账密代理</strong><span>需要用户名和密码</span></div>
					<span class="status-pill" :class="{ 'status-pill--muted': !snapshot.authenticatedProxyRunning }">{{ snapshot.authenticatedProxyRunning ? '18081 已启动' : '未启动' }}</span>
				  </div>
				  <div v-if="authenticatedProxy" class="proxy-access-rows">
					<div class="proxy-access-row">
					  <span>HTTP / HTTPS</span><code>{{ authenticatedProxy.httpUrl }}</code>
					  <button class="icon-button proxy-copy-button" :title="copiedProxyItem === 'auth-http' ? '已复制' : '复制账密 HTTP 代理'" :aria-label="copiedProxyItem === 'auth-http' ? '已复制账密 HTTP 代理' : '复制账密 HTTP 代理'" @click="copyProxyValue(authenticatedProxy.httpUrl, '账密 HTTP 代理', 'auth-http')"><MaterialIcon :name="copiedProxyItem === 'auth-http' ? 'checkCircle' : 'contentCopy'" :size="19" /></button>
					</div>
					<div class="proxy-access-row">
					  <span>SOCKS5</span><code>{{ authenticatedProxy.socks5Url }}</code>
					  <button class="icon-button proxy-copy-button" :title="copiedProxyItem === 'auth-socks' ? '已复制' : '复制账密 SOCKS5 代理'" :aria-label="copiedProxyItem === 'auth-socks' ? '已复制账密 SOCKS5 代理' : '复制账密 SOCKS5 代理'" @click="copyProxyValue(authenticatedProxy.socks5Url, '账密 SOCKS5 代理', 'auth-socks')"><MaterialIcon :name="copiedProxyItem === 'auth-socks' ? 'checkCircle' : 'contentCopy'" :size="19" /></button>
					</div>
					<div class="proxy-access-row proxy-access-row--credential">
					  <span>用户名</span><code>{{ authenticatedProxy.username }}</code>
					  <button class="icon-button proxy-copy-button" :title="copiedProxyItem === 'username' ? '已复制' : '复制用户名'" :aria-label="copiedProxyItem === 'username' ? '已复制用户名' : '复制用户名'" @click="copyProxyValue(authenticatedProxy.username, '用户名', 'username')"><MaterialIcon :name="copiedProxyItem === 'username' ? 'checkCircle' : 'contentCopy'" :size="19" /></button>
					</div>
					<div class="proxy-access-row proxy-access-row--credential">
					  <span>密码</span><code>{{ authenticatedProxy.password }}</code>
					  <button class="icon-button proxy-copy-button" :title="copiedProxyItem === 'password' ? '已复制' : '复制密码'" :aria-label="copiedProxyItem === 'password' ? '已复制密码' : '复制密码'" @click="copyProxyValue(authenticatedProxy.password, '密码', 'password')"><MaterialIcon :name="copiedProxyItem === 'password' ? 'checkCircle' : 'contentCopy'" :size="19" /></button>
					</div>
				  </div>
				  <p v-else class="proxy-access-error">{{ snapshot.authenticatedProxyError || authenticatedProxyAccessError || '账密代理正在准备' }}</p>
				</article>
			  </div>
			</section>

            <section class="section-block">
              <div class="section-heading">
                <div>
                  <h3>网络控制</h3>
                  <p>控制手机 USB 共享，并通过重连移动网络更换公网 IP</p>
                </div>
                <span class="status-pill" :class="{ 'status-pill--muted': !phoneControllable || !proxyUsable }">
                  {{ phoneStatusLabel }}
                </span>
              </div>
              <p class="control-hint" :class="{ 'control-hint--error': connected && !phoneReady }">
                {{ phoneStatusDetail }}
              </p>
              <div class="action-row">
                <button class="primary-button" :disabled="!phoneControllable || actionsLocked" @click="reconnectMobile">
                  <MaterialIcon name="sync" :size="20" />{{ busy === 'reconnect' || snapshot.networkChanging ? '正在换 IP' : '重新联网 / 换 IP' }}
                </button>
                <button class="tonal-button" :disabled="!phoneControllable || actionsLocked" @click="runAction('start', () => bridge.StartTethering(), '已请求开启共享')">
                  开启共享
                </button>
                <button class="text-button" :disabled="!phoneControllable || actionsLocked" @click="runAction('stop', () => bridge.StopTethering(), '已请求关闭共享')">
                  关闭共享
                </button>
              </div>
              <article v-if="lastReconnect" class="reconnect-result" :class="{ 'reconnect-result--error': !lastReconnect.ok }">
                <div class="reconnect-result__heading">
                  <strong>{{ lastReconnect.message }}</strong>
                  <span>{{ lastReconnect.ipChanged === true ? '公网 IP 已变化' : lastReconnect.ipChanged === false ? '公网 IP 未变化' : '重连已完成' }}</span>
                </div>
                <div v-if="reconnectIpRows.length" class="reconnect-ip-list">
                  <div v-for="item in reconnectIpRows" :key="item.label">
                    <small>{{ item.label }}</small>
                    <code>{{ item.before || '—' }} → {{ item.after || '—' }}</code>
                  </div>
                </div>
                <div class="reconnect-checks">
                  <span v-if="lastReconnect.networkDisconnected">已完成断开重连</span>
                  <span v-if="lastReconnect.networkRecovered">移动网络已恢复</span>
                  <span v-if="lastReconnect.ipChanged === false">运营商分配了相同地址</span>
                </div>
              </article>
            </section>
          </template>

          <template v-else-if="currentPage === 'traffic'">
            <section class="speed-grid">
              <article class="speed-card speed-card--accent">
                <span class="metric-icon"><MaterialIcon name="upload" :size="24" /></span>
                <div><small>实时上传</small><strong>{{ formatBytes(traffic.uploadBytesPerSecond) }}/s</strong></div>
              </article>
              <article class="speed-card">
                <span class="metric-icon"><MaterialIcon name="download" :size="24" /></span>
                <div><small>实时下载</small><strong>{{ formatBytes(traffic.downloadBytesPerSecond) }}/s</strong></div>
              </article>
            </section>

            <section class="traffic-summary-grid">
              <article><small>累计流量</small><strong>{{ formatBytes(totalBytes) }}</strong></article>
              <article><small>活跃连接</small><strong>{{ traffic.activeConnections }}</strong></article>
              <article><small>连接次数</small><strong>{{ traffic.totalConnections }}</strong></article>
              <article><small>统计时长</small><strong>{{ formatDuration(traffic.startedAtMillis) }}</strong></article>
            </section>

            <section class="section-block">
              <div class="section-heading">
                <div><h3>按代理类型</h3><p>同一端口会自动识别 HTTP 与 SOCKS5</p></div>
              </div>
              <div class="protocol-traffic-grid">
                <article>
                  <div class="protocol-title"><span class="proxy-badge">HTTP</span><strong>HTTP / HTTPS</strong></div>
                  <div class="traffic-detail"><span>上传 {{ formatBytes(traffic.http.uploadBytes) }}</span><span>下载 {{ formatBytes(traffic.http.downloadBytes) }}</span><span>{{ traffic.http.connections }} 次连接</span></div>
                </article>
                <article>
                  <div class="protocol-title"><span class="proxy-badge proxy-badge--soft">S5</span><strong>SOCKS5</strong></div>
                  <div class="traffic-detail"><span>上传 {{ formatBytes(traffic.socks5.uploadBytes) }}</span><span>下载 {{ formatBytes(traffic.socks5.downloadBytes) }}</span><span>{{ traffic.socks5.connections }} 次连接</span></div>
                </article>
              </div>
            </section>

            <section v-if="traffic.totalConnections === 0" class="empty-state traffic-empty">
              <span><MaterialIcon name="monitoring" :size="25" /></span>
              <div><strong>还没有代理流量</strong><p>把软件代理设置为 HTTP 或 SOCKS5：{{ snapshot.proxyListen }}</p></div>
            </section>
          </template>

          <template v-else>
            <section class="settings-stack">
			  <article class="settings-card settings-card--column">
				<div class="section-heading"><div><h3>代理端口</h3><p>两个端口都支持 HTTP、HTTPS 和 SOCKS5</p></div></div>
				<div class="settings-proxy-list">
				  <div><span>免密</span><code>{{ snapshot.proxyListen }}</code><span class="status-pill" :class="{ 'status-pill--muted': !snapshot.proxyRunning }">{{ snapshot.proxyRunning ? '已启动' : '未启动' }}</span></div>
				  <div><span>账密</span><code>{{ snapshot.authenticatedProxyListen }}</code><span class="status-pill" :class="{ 'status-pill--muted': !snapshot.authenticatedProxyRunning }">{{ snapshot.authenticatedProxyRunning ? '已启动' : '未启动' }}</span></div>
				</div>
				<p v-if="snapshot.authenticatedProxyError" class="control-api-error">{{ snapshot.authenticatedProxyError }}</p>
			  </article>

              <article class="settings-card settings-card--column">
                <div class="section-heading">
                  <div><h3>连接协议</h3><p>决定之后新建连接使用的公网 IP 协议</p></div>
                  <div class="segmented">
                    <button v-for="mode in (['auto', 'ipv4', 'ipv6'] as const)" :key="mode" :class="{ selected: snapshot.ipMode === mode }" :disabled="actionsLocked" @click="setMode(mode)">{{ modeLabel(mode) }}</button>
                  </div>
                </div>
                <div class="policy-note"><strong>仅使用手机网络</strong><span>手机连接不可用时会暂停代理，不会改走电脑的 Wi-Fi、以太网或 VPN。</span></div>
              </article>

              <article class="settings-card exclusive-setting">
                <div class="exclusive-setting-copy">
                  <div><h3>独占模式</h3><p>仅允许 USBridge 直接使用所选手机 USB 网卡；其他应用使用手机网络时需连接本机代理。</p></div>
                  <small>Wi-Fi、以太网和 VPN 不受影响。首次开启需要 Windows 管理员授权。</small>
                  <p v-if="snapshot.exclusiveModeError" class="control-api-error">{{ snapshot.exclusiveModeError }}</p>
                </div>
                <div class="exclusive-setting-control">
                  <span class="status-pill" :class="{ 'status-pill--muted': !snapshot.exclusiveModeActive }">{{ exclusiveModePresentation.label }}</span>
                  <button
                    class="m3-switch"
                    :class="{ 'm3-switch--selected': snapshot.exclusiveModeEnabled }"
                    type="button"
                    role="switch"
                    :aria-checked="snapshot.exclusiveModeEnabled"
                    :aria-label="snapshot.exclusiveModeEnabled ? '关闭独占模式' : '开启独占模式'"
                    :disabled="actionsLocked || !snapshot.exclusiveModeSupported"
                    @click="setExclusiveMode(!snapshot.exclusiveModeEnabled)"
                  >
                    <span aria-hidden="true"></span>
                  </button>
                </div>
              </article>

              <article class="settings-card settings-card--column">
                <div class="section-heading">
                  <div><h3>手机 USB 设备</h3><p>{{ selected ? `已自动选择：${selected.name}` : '连接手机后自动识别' }}</p></div>
                  <button class="tonal-button" :disabled="actionsLocked" @click="chooseAutomatic">自动选择</button>
                </div>
                <div class="compact-adapters">
                  <button v-for="item in visibleAdapters" :key="item.id" :class="{ selected: selected?.id === item.id }" :disabled="actionsLocked" @click="chooseAdapter(item)">
                    <span>{{ item.name }}</span><small>{{ selected?.id === item.id ? '当前正在使用' : '可用的手机共享设备' }}</small>
                  </button>
                  <p v-if="!visibleAdapters.length" class="muted-copy">还没有发现手机 USB 网络，请检查连接和共享状态。</p>
                </div>
              </article>

              <details class="settings-disclosure">
                <summary>
                  <div><h3>自动化控制</h3><p>供本机脚本或其他 Windows 程序调用</p></div>
                  <div class="disclosure-status">
                    <span class="status-pill" :class="{ 'status-pill--muted': !snapshot.controlRunning }">{{ snapshot.controlRunning ? '可用' : '不可用' }}</span>
                    <span class="disclosure-chevron"><MaterialIcon name="expandMore" :size="20" /></span>
                  </div>
                </summary>

                <div class="disclosure-body control-api-card">
                  <p class="advanced-intro">此功能仅监听本机，不需要账号或 Token。普通使用无需展开或修改。</p>

                <div class="control-api-address">
                  <div>
                    <small>本机接口地址</small>
                    <code>{{ controlBaseUrl }}</code>
                  </div>
                  <span>仅限 127.0.0.1</span>
                </div>

                <p v-if="snapshot.controlError" class="control-api-error">{{ snapshot.controlError }}</p>

                <div class="api-endpoint-list">
                  <div v-for="endpoint in controlEndpoints" :key="`${endpoint.method}-${endpoint.path}`" class="api-endpoint-row">
                    <span class="api-method" :class="`api-method--${endpoint.method.toLowerCase()}`">{{ endpoint.method }}</span>
                    <code>{{ endpoint.path }}</code>
                    <span>{{ endpoint.label }}</span>
                  </div>
                </div>

                <div class="api-example">
                  <div class="api-example-heading">
                    <div><strong>换 IP 调用示例</strong><small>{{ activeControlExampleInfo.hint }}</small></div>
                    <div class="api-example-actions">
                      <div class="api-example-tabs" aria-label="示例语言">
                        <button
                          v-for="example in (['windows', 'python', 'go'] as const)"
                          :key="example"
                          :class="{ selected: activeControlExample === example }"
                          @click="activeControlExample = example"
                        >
                          {{ controlExamples[example].label }}
                        </button>
                      </div>
                      <button class="text-button api-copy-button" @click="copyReconnectCommand">
                        {{ copiedControlExample ? '已复制' : '复制' }}
                      </button>
                    </div>
                  </div>
                  <pre><code>{{ activeControlExampleInfo.code }}</code></pre>
                  <p>写操作需要使用 JSON。响应中的 <code>ipChanged</code>、<code>before</code> 和 <code>after</code> 可确认公网 IP 是否真的变化。</p>
                </div>
                </div>
              </details>

              <details class="settings-disclosure">
                <summary>
                  <div><h3>技术信息</h3><p>仅在排查连接问题时查看</p></div>
                  <span class="disclosure-chevron"><MaterialIcon name="expandMore" :size="20" /></span>
                </summary>
                <div class="disclosure-body technical-grid">
                  <article>
                    <small>手机端版本</small>
                    <strong>{{ snapshot.androidStatus?.version || '未连接' }}</strong>
                  </article>
                  <article>
                    <small>Root 管理器</small>
                    <strong>{{ snapshot.androidStatus?.root.implementation || '未读取' }}</strong>
                  </article>
                  <article>
                    <small>手机控制地址</small>
                    <code>{{ snapshot.androidEndpoint || '尚未识别' }}</code>
                  </article>
                  <article>
                    <small>Windows USB 设备</small>
                    <strong>{{ selected?.name || '尚未识别' }}</strong>
                    <span>{{ selected?.description || '—' }}</span>
                  </article>
                  <article class="technical-grid--wide">
                    <small>USB 本地地址</small>
                    <code>{{ selectedLocalAddresses || '尚未分配' }}</code>
                  </article>
                  <article v-if="snapshot.androidError" class="technical-grid--wide">
                    <small>手机端错误</small>
                    <span>{{ snapshot.androidError }}</span>
                  </article>
                </div>
              </details>

              <article class="settings-card">
                <div><h3>USBridge</h3><p>手机移动网络共享与本机代理</p></div>
                <div class="about-copy"><strong>版本 {{ appVersion }}</strong><small>设置与统计保存在本机</small></div>
              </article>
            </section>
          </template>
        </div>
      </Transition>
    </main>
  </div>
</template>
