package com.usbridge.core.model

enum class RootState {
    CHECKING,
    GRANTED,
    DENIED,
    ERROR
}

enum class IpMode {
    AUTO,
    IPV4,
    IPV6
}

enum class UsbConnectionState {
    DISCONNECTED,
    CONNECTED,
    INTERFACE_READY
}

enum class InterfaceKind {
    CELLULAR,
    USB,
    WIFI,
    OTHER
}

data class NetworkInterfaceSnapshot(
    val name: String,
    val kind: InterfaceKind,
    val isUp: Boolean,
    val ipv4Addresses: List<String>,
    val ipv6Addresses: List<String>
)

data class TrafficSnapshot(
    val uploadBytes: Long = 0,
    val downloadBytes: Long = 0
)

data class TrafficSessionRecord(
    val id: Long,
    val startedAtMillis: Long,
    val endedAtMillis: Long?,
    val interfaceName: String,
    val uploadBytes: Long,
    val downloadBytes: Long,
    val isActive: Boolean
)

data class TrafficHistorySummary(
    val todayUploadBytes: Long = 0,
    val todayDownloadBytes: Long = 0,
    val monthUploadBytes: Long = 0,
    val monthDownloadBytes: Long = 0,
    val todayDurationMillis: Long = 0,
    val sessionCount: Int = 0,
    val recentSessions: List<TrafficSessionRecord> = emptyList()
)

data class PublicIpSnapshot(
    val ipv4: String? = null,
    val ipv6: String? = null,
    val checkedAtMillis: Long? = null
)

data class PublicIpCheckResult(
    val snapshot: PublicIpSnapshot = PublicIpSnapshot(),
    val cellularNetworkAvailable: Boolean = false,
    val errorMessage: String? = null
)

enum class MobileReconnectStatus {
    IDLE,
    CHECKING_BEFORE,
    RECONNECTING,
    WAITING_FOR_NETWORK,
    VERIFYING,
    IP_CHANGED,
    IP_UNCHANGED,
    COMPLETED_WITHOUT_IP,
    ERROR
}

data class MobileReconnectState(
    val status: MobileReconnectStatus = MobileReconnectStatus.IDLE,
    val message: String = "尚未执行移动网络重连",
    val before: PublicIpSnapshot? = null,
    val after: PublicIpSnapshot? = null,
    val commandSucceeded: Boolean? = null,
    val networkDisconnected: Boolean? = null,
    val networkRecovered: Boolean? = null,
    val startedAtMillis: Long? = null,
    val completedAtMillis: Long? = null
) {
    val isRunning: Boolean
        get() = status in setOf(
            MobileReconnectStatus.CHECKING_BEFORE,
            MobileReconnectStatus.RECONNECTING,
            MobileReconnectStatus.WAITING_FOR_NETWORK,
            MobileReconnectStatus.VERIFYING
        )
}

data class DeviceSnapshot(
    val manufacturer: String = "",
    val model: String = "",
    val androidVersion: String = "",
    val apiLevel: Int = 0,
    val appVersion: String = "",
    val selinuxMode: String? = null,
    val rootImplementation: String? = null,
    val rootUid: String? = null,
    val usbConnectionState: UsbConnectionState = UsbConnectionState.DISCONNECTED,
    val usbFunctions: String? = null,
    val usbConfigured: Boolean? = null,
    val usbDataRole: String? = null,
    val activeTransport: String = "未连接",
    val interfaces: List<NetworkInterfaceSnapshot> = emptyList(),
    val availableRootTools: Set<String> = emptySet(),
    val traffic: TrafficSnapshot = TrafficSnapshot()
) {
    val cellularInterfaces: List<NetworkInterfaceSnapshot>
        get() = interfaces.filter { it.kind == InterfaceKind.CELLULAR }

    val usbInterfaces: List<NetworkInterfaceSnapshot>
        get() = interfaces.filter { it.kind == InterfaceKind.USB }

    val cellularIpv4: List<String>
        get() = cellularInterfaces.flatMap { it.ipv4Addresses }.distinct()

    val cellularIpv6: List<String>
        get() = cellularInterfaces.flatMap { it.ipv6Addresses }.distinct()
}

data class AutomationSettings(
    val autoTetherOnUsb: Boolean = false,
    val stopOnDisconnect: Boolean = true,
    val startOnBoot: Boolean = false,
    val retryOnFailure: Boolean = true,
    val ipMode: IpMode = IpMode.AUTO
)
