package com.usbridge.control

import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.net.wifi.WifiManager
import com.usbridge.core.model.InterfaceKind
import com.usbridge.core.model.IpMode
import com.usbridge.core.model.MobileReconnectState
import com.usbridge.core.model.MobileReconnectStatus
import com.usbridge.core.model.NetworkInterfaceSnapshot
import com.usbridge.core.model.PublicIpCheckResult
import com.usbridge.core.model.PublicIpSnapshot
import com.usbridge.core.model.RootState
import com.usbridge.core.model.hasUsableIpAddress
import com.usbridge.core.model.hasUsableIpv4Address
import com.usbridge.core.model.hasUsableIpv6Address
import com.usbridge.core.network.PublicIpComparator
import com.usbridge.core.network.PublicIpRepository
import com.usbridge.core.preferences.AppPreferences
import com.usbridge.core.root.RootControlClient
import com.usbridge.core.root.RootGateway
import com.usbridge.core.root.RootNetworkInterfaces
import com.usbridge.core.root.RootTetheringDiagnostics
import com.usbridge.core.root.RootTetheringSnapshot
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

data class PhoneControlState(
    val rootState: RootState = RootState.CHECKING,
    val rootImplementation: String? = null,
    val publicIp: PublicIpSnapshot = PublicIpSnapshot(),
    val tetheringPath: PhoneTetheringPathState = PhoneTetheringPathState(),
    val isCheckingPublicIp: Boolean = false,
    val publicIpError: String? = null,
    val reconnect: MobileReconnectState = MobileReconnectState(),
    val lastUpdatedAtMillis: Long = System.currentTimeMillis()
)

data class PhoneOperationResult(
    val ok: Boolean,
    val message: String,
    val before: PublicIpSnapshot? = null,
    val after: PublicIpSnapshot? = null,
    val commandSucceeded: Boolean? = null,
    val networkDisconnected: Boolean? = null,
    val networkRecovered: Boolean? = null,
    val ipChanged: Boolean? = null
)

data class PhoneTetheringPathState(
    val usbInterfaceNames: List<String> = emptyList(),
    val tetheringEnabled: Boolean = false,
    val upstreamTransport: String = "none",
    val upstreamInterfaceNames: List<String> = emptyList(),
    val mobileInterfaceNames: List<String> = emptyList(),
    val ipv4Available: Boolean = false,
    val ipv6Available: Boolean = false,
    val diagnosticsAvailable: Boolean = false
)

class PhoneControlRuntime private constructor(context: Context) {
    private val appContext = context.applicationContext
    private val preferences = AppPreferences(appContext)
    private val rootGateway = RootGateway()
    private val rootControlClient = RootControlClient(appContext)
    private val publicIpRepository = PublicIpRepository(appContext)
    private val connectivityManager = appContext.getSystemService(ConnectivityManager::class.java)
    private val wifiManager = appContext.getSystemService(WifiManager::class.java)
    private val initializationMutex = Mutex()
    private val mobileOperationMutex = Mutex()
    private val tetherOperationMutex = Mutex()
    private val upstreamOperationMutex = Mutex()
    private val tetheringStateMutex = Mutex()
    private val operationScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    private val _state = MutableStateFlow(PhoneControlState())
    val state: StateFlow<PhoneControlState> = _state.asStateFlow()

    @Volatile
    private var initialized = false

    suspend fun initialize() {
        if (initialized) return
        initializationMutex.withLock {
            if (initialized) return
            refreshRootStatus()
            refreshTetheringPath()
            refreshPublicIp()
            initialized = true
        }
    }

    suspend fun refreshRootStatus(): RootState {
        val rootState = rootGateway.requestRoot()
        val implementation = if (rootState == RootState.GRANTED) {
            rootGateway.readEnvironment().rootImplementation
        } else {
            null
        }
        _state.update {
            it.copy(
                rootState = rootState,
                rootImplementation = implementation,
                lastUpdatedAtMillis = System.currentTimeMillis()
            )
        }
        return rootState
    }

    suspend fun refreshPublicIp(): PublicIpCheckResult {
        if (!mobileOperationMutex.tryLock()) {
            return PublicIpCheckResult(
                snapshot = _state.value.publicIp,
                errorMessage = "移动网络操作正在进行"
            )
        }
        return try {
            _state.update {
                it.copy(isCheckingPublicIp = true, publicIpError = null)
            }
            val result = publicIpRepository.checkCellularPublicIps()
            _state.update {
                it.copy(
                    publicIp = result.snapshot,
                    isCheckingPublicIp = false,
                    publicIpError = result.errorMessage,
                    lastUpdatedAtMillis = System.currentTimeMillis()
                )
            }
            result
        } finally {
            withContext(NonCancellable) {
                _state.update {
                    if (it.isCheckingPublicIp) it.copy(isCheckingPublicIp = false) else it
                }
            }
            mobileOperationMutex.unlock()
        }
    }

    fun refreshPublicIpAsync() {
        operationScope.launch { refreshPublicIp() }
    }

    suspend fun reconnectMobileNetwork(): PhoneOperationResult {
        if (!mobileOperationMutex.tryLock()) {
            return PhoneOperationResult(false, "已有移动网络操作正在进行，请稍候")
        }

        var mobileDataDisabled = false
        var mobileDataRestored = false
        return try {
            if (refreshRootStatus() != RootState.GRANTED) {
                return operationError("尚未获得 Root 权限")
            }

            val usbTetheringWasReady = isUsbTetherInterfaceReady()
            if (usbTetheringWasReady) {
                val upstream = ensureCellularUpstream()
                if (!upstream.ok) {
                    return operationError("无法准备移动网络出口：${upstream.message}")
                }
            }

            val startedAt = System.currentTimeMillis()
            updateReconnect(
                status = MobileReconnectStatus.CHECKING_BEFORE,
                message = "正在读取重连前的蜂窝公网 IP",
                startedAtMillis = startedAt
            )
            val beforeResult = publicIpRepository.checkCellularPublicIps()
            val before = beforeResult.snapshot
            _state.update { it.copy(publicIp = before, publicIpError = beforeResult.errorMessage) }

            updateReconnect(
                status = MobileReconnectStatus.RECONNECTING,
                message = "正在关闭移动数据",
                before = before,
                startedAtMillis = startedAt
            )
            val disableStartedAt = System.currentTimeMillis()
            val disableResult = rootControlClient.setMobileDataEnabled(false)
            if (!disableResult.success) {
                return operationError(
                    message = disableResult.message,
                    before = before,
                    commandSucceeded = false,
                    startedAtMillis = startedAt
                )
            }
            mobileDataDisabled = true

            updateReconnect(
                status = MobileReconnectStatus.RECONNECTING,
                message = "移动数据关闭命令已执行，正在确认蜂窝网络断开",
                before = before,
                commandSucceeded = true,
                startedAtMillis = startedAt
            )
            val networkDisconnected = publicIpRepository.awaitCellularUnavailable()
            val elapsedDownTime = System.currentTimeMillis() - disableStartedAt
            if (elapsedDownTime < MINIMUM_DATA_DOWN_TIME_MILLIS) {
                delay(MINIMUM_DATA_DOWN_TIME_MILLIS - elapsedDownTime)
            }

            updateReconnect(
                status = MobileReconnectStatus.WAITING_FOR_NETWORK,
                message = "正在重新开启移动数据",
                before = before,
                commandSucceeded = true,
                networkDisconnected = networkDisconnected,
                startedAtMillis = startedAt
            )
            val enableResult = withContext(NonCancellable + Dispatchers.IO) {
                rootControlClient.setMobileDataEnabled(true)
            }
            mobileDataRestored = enableResult.success
            if (!enableResult.success) {
                return operationError(
                    message = enableResult.message,
                    before = before,
                    commandSucceeded = false,
                    networkDisconnected = networkDisconnected,
                    networkRecovered = false,
                    startedAtMillis = startedAt
                )
            }

            updateReconnect(
                status = MobileReconnectStatus.WAITING_FOR_NETWORK,
                message = "移动数据已开启，正在等待蜂窝网络恢复",
                before = before,
                commandSucceeded = true,
                networkDisconnected = networkDisconnected,
                startedAtMillis = startedAt
            )
            val networkRecovered = publicIpRepository.awaitCellularInternet()
            if (!networkRecovered) {
                return operationError(
                    message = "移动数据已开启，但蜂窝网络在限定时间内没有恢复",
                    before = before,
                    commandSucceeded = true,
                    networkDisconnected = networkDisconnected,
                    networkRecovered = false,
                    startedAtMillis = startedAt
                )
            }
            if (usbTetheringWasReady && !awaitCellularTetheringUpstream()) {
                return operationError(
                    message = "移动网络已恢复，但 USB 共享出口尚未恢复",
                    before = before,
                    commandSucceeded = true,
                    networkDisconnected = networkDisconnected,
                    networkRecovered = true,
                    startedAtMillis = startedAt
                )
            }

            delay(PUBLIC_IP_SETTLE_DELAY_MILLIS)
            updateReconnect(
                status = MobileReconnectStatus.VERIFYING,
                message = "蜂窝网络已恢复，正在验证新公网 IP",
                before = before,
                commandSucceeded = true,
                networkDisconnected = networkDisconnected,
                networkRecovered = true,
                startedAtMillis = startedAt
            )
            val afterResult = checkPublicIpAfterReconnect()
            val after = afterResult.snapshot
            val comparison = PublicIpComparator.compare(before, after)
            val hadCellularBefore = beforeResult.cellularNetworkAvailable
            val disconnectWasExpected = hadCellularBefore
            val disconnectProven = !disconnectWasExpected || networkDisconnected
            val ipChanged = comparison.status == MobileReconnectStatus.IP_CHANGED
            val finalStatus = if (!disconnectProven && !ipChanged) {
                MobileReconnectStatus.ERROR
            } else {
                comparison.status
            }
            val finalMessage = if (!disconnectProven && !ipChanged) {
                "Root 命令已执行，但未观察到蜂窝网络断开，公网 IP 也没有变化"
            } else {
                comparison.message
            }
            val completed = MobileReconnectState(
                status = finalStatus,
                message = finalMessage,
                before = before,
                after = after,
                commandSucceeded = true,
                networkDisconnected = networkDisconnected,
                networkRecovered = true,
                startedAtMillis = startedAt,
                completedAtMillis = System.currentTimeMillis()
            )
            _state.update {
                it.copy(
                    publicIp = after,
                    publicIpError = afterResult.errorMessage,
                    reconnect = completed,
                    lastUpdatedAtMillis = System.currentTimeMillis()
                )
            }
            PhoneOperationResult(
                ok = finalStatus != MobileReconnectStatus.ERROR,
                message = finalMessage,
                before = before,
                after = after,
                commandSucceeded = true,
                networkDisconnected = networkDisconnected,
                networkRecovered = true,
                ipChanged = when (comparison.status) {
                    MobileReconnectStatus.IP_CHANGED -> true
                    MobileReconnectStatus.IP_UNCHANGED -> false
                    else -> null
                }
            )
        } finally {
            if (mobileDataDisabled && !mobileDataRestored) {
                withContext(NonCancellable + Dispatchers.IO) {
                    rootControlClient.setMobileDataEnabled(true)
                }
            }
            if (_state.value.reconnect.isRunning) {
                withContext(NonCancellable) {
                    updateReconnect(
                        status = MobileReconnectStatus.ERROR,
                        message = "移动网络重连已中断",
                        completedAtMillis = System.currentTimeMillis()
                    )
                }
            }
            mobileOperationMutex.unlock()
        }
    }

    fun reconnectMobileNetworkAsync() {
        operationScope.launch { reconnectMobileNetwork() }
    }

    suspend fun setUsbTetheringEnabled(enabled: Boolean): PhoneOperationResult {
        if (!tetherOperationMutex.tryLock()) {
            return PhoneOperationResult(false, "USB 网络共享操作正在进行")
        }
        return try {
            if (refreshRootStatus() != RootState.GRANTED) {
                return PhoneOperationResult(false, "尚未获得 Root 权限")
            }
            val result = if (enabled) {
                rootControlClient.startUsbTethering()
            } else {
                rootControlClient.stopUsbTethering()
            }
            val operation = PhoneOperationResult(
                ok = result.success,
                message = if (result.success) {
                    if (enabled) "已请求开启 USB 网络共享" else "已请求关闭 USB 网络共享"
                } else {
                    result.message
                },
                commandSucceeded = result.success
            )
            if (result.success && enabled) {
                delay(TETHER_UPSTREAM_SETTLE_DELAY_MILLIS)
                val upstream = ensureCellularUpstream()
                if (!upstream.ok) return upstream
            } else if (result.success) {
                restoreWifiIfNeeded()
                refreshTetheringPath()
            }
            operation
        } finally {
            tetherOperationMutex.unlock()
        }
    }

    suspend fun ensureCellularUpstream(): PhoneOperationResult = upstreamOperationMutex.withLock {
        if (!isUsbTetherInterfaceReady()) {
            return PhoneOperationResult(false, "USB 网络共享接口尚未就绪")
        }
        if (_state.value.rootState != RootState.GRANTED &&
            refreshRootStatus() != RootState.GRANTED
        ) {
            return PhoneOperationResult(false, "尚未获得 Root 权限")
        }

        if (wifiManager.isWifiEnabled) {
            preferences.setRestoreWifiPending(true)
            val wifiResult = rootControlClient.setWifiEnabled(false)
            if (!wifiResult.success) {
                preferences.setRestoreWifiPending(false)
                return PhoneOperationResult(false, wifiResult.message, commandSucceeded = false)
            }
        }

        val cellularReady = publicIpRepository.awaitCellularInternet()
        val tetheringUpstreamIsCellular = awaitCellularTetheringUpstream()
        return if (cellularReady && tetheringUpstreamIsCellular) {
            PhoneOperationResult(
                ok = true,
                message = "USB 共享已锁定移动网络出口",
                commandSucceeded = true,
                networkRecovered = true
            )
        } else {
            PhoneOperationResult(
                ok = false,
                message = "Wi-Fi 已关闭，但系统尚未把 USB 共享切换到移动网络",
                commandSucceeded = true,
                networkRecovered = cellularReady
            )
        }
    }

    suspend fun restoreWifiIfNeeded(): PhoneOperationResult = upstreamOperationMutex.withLock {
        if (!preferences.shouldRestoreWifi()) {
            return PhoneOperationResult(true, "Wi-Fi 无需恢复")
        }
        val result = rootControlClient.setWifiEnabled(true)
        if (result.success) preferences.setRestoreWifiPending(false)
        PhoneOperationResult(
            ok = result.success,
            message = if (result.success) "已恢复重连前的 Wi-Fi 状态" else result.message,
            commandSucceeded = result.success
        )
    }

    suspend fun refreshTetheringPath(): PhoneTetheringPathState =
        tetheringStateMutex.withLock {
            val path = withContext(Dispatchers.IO) {
                val interfaces = RootNetworkInterfaces.read()
                val tethering = RootTetheringDiagnostics.read()
                buildPhoneTetheringPathState(
                    interfaces = interfaces,
                    tethering = tethering,
                    fallbackUpstreamTransport = defaultUpstreamTransport()
                )
            }
            _state.update {
                it.copy(
                    tetheringPath = path,
                    lastUpdatedAtMillis = System.currentTimeMillis()
                )
            }
            path
        }

    suspend fun activeUpstreamTransport(): String =
        refreshTetheringPath().upstreamTransport

    private fun defaultUpstreamTransport(): String {
        val capabilities = connectivityManager.getNetworkCapabilities(connectivityManager.activeNetwork)
            ?: return "none"
        return when {
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "cellular"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "wifi"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "ethernet"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_VPN) -> "vpn"
            else -> "other"
        }
    }

    fun setIpMode(mode: IpMode): PhoneOperationResult {
        preferences.setIpMode(mode)
        val label = when (mode) {
            IpMode.AUTO -> "自动"
            IpMode.IPV4 -> "IPv4"
            IpMode.IPV6 -> "IPv6"
        }
        return PhoneOperationResult(true, "已切换为 $label")
    }

    fun ipMode(): IpMode = preferences.readAutomationSettings().ipMode

    private suspend fun awaitCellularTetheringUpstream(
        timeoutMillis: Long = DEFAULT_UPSTREAM_WAIT_MILLIS
    ): Boolean {
        val deadline = System.currentTimeMillis() + timeoutMillis
        while (System.currentTimeMillis() < deadline) {
            if (activeUpstreamTransport() == "cellular") return true
            delay(UPSTREAM_POLL_INTERVAL_MILLIS)
        }
        return activeUpstreamTransport() == "cellular"
    }

    private suspend fun isUsbTetherInterfaceReady(): Boolean =
        refreshTetheringPath().tetheringEnabled

    private suspend fun checkPublicIpAfterReconnect(): PublicIpCheckResult {
        var result = publicIpRepository.checkCellularPublicIps()
        repeat(PUBLIC_IP_RETRY_COUNT - 1) {
            if (result.snapshot.ipv4 != null || result.snapshot.ipv6 != null) return result
            delay(PUBLIC_IP_RETRY_DELAY_MILLIS)
            result = publicIpRepository.checkCellularPublicIps()
        }
        return result
    }

    private fun operationError(
        message: String,
        before: PublicIpSnapshot? = null,
        commandSucceeded: Boolean? = null,
        networkDisconnected: Boolean? = null,
        networkRecovered: Boolean? = null,
        startedAtMillis: Long = System.currentTimeMillis()
    ): PhoneOperationResult {
        updateReconnect(
            status = MobileReconnectStatus.ERROR,
            message = message,
            before = before,
            commandSucceeded = commandSucceeded,
            networkDisconnected = networkDisconnected,
            networkRecovered = networkRecovered,
            startedAtMillis = startedAtMillis,
            completedAtMillis = System.currentTimeMillis()
        )
        return PhoneOperationResult(
            ok = false,
            message = message,
            before = before,
            commandSucceeded = commandSucceeded,
            networkDisconnected = networkDisconnected,
            networkRecovered = networkRecovered
        )
    }

    private fun updateReconnect(
        status: MobileReconnectStatus,
        message: String,
        before: PublicIpSnapshot? = _state.value.reconnect.before,
        after: PublicIpSnapshot? = null,
        commandSucceeded: Boolean? = _state.value.reconnect.commandSucceeded,
        networkDisconnected: Boolean? = _state.value.reconnect.networkDisconnected,
        networkRecovered: Boolean? = _state.value.reconnect.networkRecovered,
        startedAtMillis: Long? = _state.value.reconnect.startedAtMillis,
        completedAtMillis: Long? = null
    ) {
        _state.update {
            it.copy(
                reconnect = MobileReconnectState(
                    status = status,
                    message = message,
                    before = before,
                    after = after,
                    commandSucceeded = commandSucceeded,
                    networkDisconnected = networkDisconnected,
                    networkRecovered = networkRecovered,
                    startedAtMillis = startedAtMillis,
                    completedAtMillis = completedAtMillis
                ),
                lastUpdatedAtMillis = System.currentTimeMillis()
            )
        }
    }

    companion object {
        private const val MINIMUM_DATA_DOWN_TIME_MILLIS = 2_000L
        private const val PUBLIC_IP_SETTLE_DELAY_MILLIS = 2_000L
        private const val PUBLIC_IP_RETRY_DELAY_MILLIS = 1_500L
        private const val PUBLIC_IP_RETRY_COUNT = 4
        private const val TETHER_UPSTREAM_SETTLE_DELAY_MILLIS = 1_500L
        private const val DEFAULT_UPSTREAM_WAIT_MILLIS = 20_000L
        private const val UPSTREAM_POLL_INTERVAL_MILLIS = 500L

        @Volatile
        private var instance: PhoneControlRuntime? = null

        fun get(context: Context): PhoneControlRuntime =
            instance ?: synchronized(this) {
                instance ?: PhoneControlRuntime(context).also { instance = it }
            }
    }
}

internal fun buildPhoneTetheringPathState(
    interfaces: List<NetworkInterfaceSnapshot>,
    tethering: RootTetheringSnapshot?,
    fallbackUpstreamTransport: String
): PhoneTetheringPathState {
    val usbInterfaces = interfaces.filter { it.kind == InterfaceKind.USB }
    val fallbackMobileInterfaces = interfaces.filter {
        it.kind == InterfaceKind.CELLULAR && it.isUp && it.hasUsableIpAddress()
    }
    val resolvedUpstreamInterfaces = tethering?.resolveUpstreamInterfaces(interfaces).orEmpty()
    val addressInterfaces = if (tethering?.upstreamStateKnown == true) {
        resolvedUpstreamInterfaces
    } else {
        fallbackMobileInterfaces
    }.filter { it.isUp && it.hasUsableIpAddress() }
    val upstreamInterfaceNames = if (tethering?.upstreamStateKnown == true) {
        resolvedUpstreamInterfaces.map { it.name }
            .ifEmpty { tethering.upstreamInterfaceNames.toList() }
    } else {
        fallbackMobileInterfaces.map { it.name }
    }
    val upstreamTransport = tethering?.upstreamTransport(interfaces)
        ?: fallbackUpstreamTransport

    return PhoneTetheringPathState(
        usbInterfaceNames = usbInterfaces.map { it.name },
        tetheringEnabled = tethering?.usbTetheringActive(interfaces)
            ?: usbInterfaces.any { it.isUp && it.hasUsableIpAddress() },
        upstreamTransport = upstreamTransport,
        upstreamInterfaceNames = upstreamInterfaceNames,
        mobileInterfaceNames = addressInterfaces
            .filter { it.kind == InterfaceKind.CELLULAR }
            .map { it.name },
        ipv4Available = addressInterfaces.any { it.hasUsableIpv4Address() },
        ipv6Available = addressInterfaces.any { it.hasUsableIpv6Address() },
        diagnosticsAvailable = tethering != null &&
            (tethering.tetherStateKnown || tethering.upstreamStateKnown)
    )
}
