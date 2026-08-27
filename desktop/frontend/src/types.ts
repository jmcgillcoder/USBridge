export interface Adapter {
  id: string
  name: string
  description: string
  interfaceIndex: number
  status: string
  macAddress?: string
  linkSpeed?: number
  ipv4: string[] | null
  ipv6: string[] | null
  gateways: string[] | null
  usbCandidate: boolean
  score: number
}

export interface ProtocolTraffic {
  connections: number
  uploadBytes: number
  downloadBytes: number
}

export interface TrafficSnapshot {
  startedAtMillis: number
  activeConnections: number
  totalConnections: number
  uploadBytes: number
  downloadBytes: number
  uploadBytesPerSecond: number
  downloadBytesPerSecond: number
  http: ProtocolTraffic
  socks5: ProtocolTraffic
}

export interface PublicIPStatus {
  ipv4?: string
  ipv6?: string
}

export interface AndroidStatus {
  version: string
  root: {
    granted: boolean
    implementation?: string
  }
  usb: {
    connected: boolean
    tetheringEnabled: boolean
    upstream?: string
    cellularUpstream: boolean
    interfaces: string[] | null
  }
  mobile: {
    connected: boolean
    ipv4Available?: boolean
    ipv6Available?: boolean
    interfaces: string[] | null
  }
  ipMode: 'auto' | 'ipv4' | 'ipv6'
  publicIp: PublicIPStatus
  observedAt: string
}

export interface Snapshot {
  adapters: Adapter[]
  selectedAdapter?: Adapter
  ipMode: 'auto' | 'ipv4' | 'ipv6'
  androidEndpoint?: string
  androidReady: boolean
  androidStatus?: AndroidStatus
  androidError?: string
  androidCheckedAt?: string
  proxyListen: string
  traffic: TrafficSnapshot
  proxyRunning: boolean
  lastError?: string
  controlListen: string
  controlRunning: boolean
  controlError?: string
	 authenticatedProxyListen: string
	 authenticatedProxyRunning: boolean
	 authenticatedProxyError?: string
  exclusiveModeSupported: boolean
  exclusiveModeEnabled: boolean
  exclusiveModeActive: boolean
  exclusiveModeInterface?: string
  exclusiveModeError?: string
  networkChanging: boolean
  version?: string
}

export interface AuthenticatedProxyAccess {
	listen: string
	username: string
	password: string
	httpUrl: string
	socks5Url: string
}

export interface OperationResponse {
  ok: boolean
  message: string
  before?: PublicIPStatus
  after?: PublicIPStatus
  commandSucceeded?: boolean
  networkDisconnected?: boolean
  networkRecovered?: boolean
  ipChanged?: boolean
}

export interface AppBridge {
  GetSnapshot(): Promise<Snapshot>
	GetAuthenticatedProxyAccess(): Promise<AuthenticatedProxyAccess>
  RefreshAdapters(): Promise<void>
  SelectAdapter(selector: string): Promise<void>
  SetExclusiveMode(enabled: boolean): Promise<void>
  SetIPMode(mode: string): Promise<void>
  ResetTraffic(): Promise<void>
  ReconnectMobile(): Promise<OperationResponse>
  RefreshPublicIP(): Promise<OperationResponse>
  StartTethering(): Promise<OperationResponse>
  StopTethering(): Promise<OperationResponse>
}

declare global {
  interface Window {
    go?: {
      main?: {
        App?: AppBridge
      }
    }
  }
}
