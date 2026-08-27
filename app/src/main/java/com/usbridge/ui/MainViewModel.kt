package com.usbridge.ui

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.usbridge.control.PhoneControlRuntime
import com.usbridge.control.PhoneTetheringPathState
import com.usbridge.core.diagnostics.DeviceDiagnostics
import com.usbridge.core.model.AutomationSettings
import com.usbridge.core.model.DeviceSnapshot
import com.usbridge.core.model.IpMode
import com.usbridge.core.model.MobileReconnectState
import com.usbridge.core.model.PublicIpSnapshot
import com.usbridge.core.model.RootState
import com.usbridge.core.model.TrafficHistorySummary
import com.usbridge.core.preferences.AppPreferences
import com.usbridge.core.root.RootControlClient
import com.usbridge.core.root.RootGateway
import com.usbridge.service.UsbAutomationRuntime
import com.usbridge.service.UsbAutomationRuntimeState
import com.usbridge.service.UsbAutomationService
import com.usbridge.traffic.TrafficStatisticsRuntime
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch

data class MainUiState(
    val isRefreshing: Boolean = true,
    val rootState: RootState = RootState.CHECKING,
    val device: DeviceSnapshot = DeviceSnapshot(),
    val automation: AutomationSettings = AutomationSettings(),
    val lastUpdatedAtMillis: Long? = null,
    val errorMessage: String? = null,
    val rootServiceProbe: String? = null,
    val rootServiceError: String? = null,
    val automationRuntime: UsbAutomationRuntimeState = UsbAutomationRuntimeState(),
    val publicIp: PublicIpSnapshot = PublicIpSnapshot(),
    val tetheringPath: PhoneTetheringPathState = PhoneTetheringPathState(),
    val isCheckingPublicIp: Boolean = false,
    val publicIpError: String? = null,
    val mobileReconnect: MobileReconnectState = MobileReconnectState(),
    val trafficSummary: TrafficHistorySummary = TrafficHistorySummary()
)

class MainViewModel(application: Application) : AndroidViewModel(application) {
    private val rootGateway = RootGateway()
    private val rootControlClient = RootControlClient(application)
    private val diagnostics = DeviceDiagnostics(application, rootGateway)
    private val phoneControlRuntime = PhoneControlRuntime.get(application)
    private val preferences = AppPreferences(application)
    private val _uiState = MutableStateFlow(
        MainUiState(automation = preferences.readAutomationSettings())
    )
    val uiState: StateFlow<MainUiState> = _uiState.asStateFlow()

    private var refreshJob: Job? = null

    init {
        runCatching { UsbAutomationService.startMonitoring(application) }
        viewModelScope.launch { phoneControlRuntime.initialize() }
        viewModelScope.launch {
            phoneControlRuntime.state.collectLatest { phone ->
                _uiState.update {
                    it.copy(
                        publicIp = phone.publicIp,
                        tetheringPath = phone.tetheringPath,
                        isCheckingPublicIp = phone.isCheckingPublicIp,
                        publicIpError = phone.publicIpError,
                        mobileReconnect = phone.reconnect
                    )
                }
            }
        }
        viewModelScope.launch {
            UsbAutomationRuntime.state.collectLatest { runtime ->
                _uiState.update { it.copy(automationRuntime = runtime) }
            }
        }
        viewModelScope.launch {
            TrafficStatisticsRuntime.summary.collectLatest { summary ->
                _uiState.update { it.copy(trafficSummary = summary) }
            }
        }
        refresh(requestRoot = true)
    }

    fun refresh(requestRoot: Boolean = false) {
        if (refreshJob?.isActive == true) return
        refreshJob = viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isRefreshing = true,
                    rootState = if (requestRoot) RootState.CHECKING else it.rootState,
                    errorMessage = null
                )
            }

            val rootState = if (requestRoot || _uiState.value.rootState == RootState.CHECKING) {
                rootGateway.requestRoot()
            } else {
                _uiState.value.rootState
            }

            runCatching { diagnostics.capture(rootState) }
                .onSuccess { snapshot ->
                    _uiState.update {
                        it.copy(
                            isRefreshing = false,
                            rootState = rootState,
                            device = snapshot,
                            lastUpdatedAtMillis = System.currentTimeMillis()
                        )
                    }
                    if (rootState == RootState.GRANTED) {
                        probeRootService()
                        runCatching { UsbAutomationService.startMonitoring(getApplication()) }
                        refreshPublicIp()
                    }
                }
                .onFailure { error ->
                    _uiState.update {
                        it.copy(
                            isRefreshing = false,
                            rootState = rootState,
                            errorMessage = error.message ?: "设备检测失败"
                        )
                    }
                }
        }
    }

    private fun probeRootService() {
        viewModelScope.launch {
            runCatching { rootControlClient.probe() }
                .onSuccess { probe ->
                    _uiState.update {
                        it.copy(rootServiceProbe = probe, rootServiceError = null)
                    }
                }
                .onFailure { error ->
                    _uiState.update {
                        it.copy(rootServiceError = error.message ?: "RootService 连接失败")
                    }
                }
        }
    }

    fun retryRootAuthorization() = refresh(requestRoot = true)

    fun setAutoTether(enabled: Boolean) {
        preferences.setAutoTether(enabled)
        updateAutomation { copy(autoTetherOnUsb = enabled) }
        if (enabled && _uiState.value.rootState == RootState.GRANTED) {
            runCatching { UsbAutomationService.startMonitoring(getApplication()) }
        }
    }

    fun setStopOnDisconnect(enabled: Boolean) {
        preferences.setStopOnDisconnect(enabled)
        updateAutomation { copy(stopOnDisconnect = enabled) }
    }

    fun setStartOnBoot(enabled: Boolean) {
        preferences.setStartOnBoot(enabled)
        updateAutomation { copy(startOnBoot = enabled) }
    }

    fun setRetryOnFailure(enabled: Boolean) {
        preferences.setRetryOnFailure(enabled)
        updateAutomation { copy(retryOnFailure = enabled) }
    }

    fun setIpMode(mode: IpMode) {
        preferences.setIpMode(mode)
        phoneControlRuntime.setIpMode(mode)
        updateAutomation { copy(ipMode = mode) }
    }

    fun startUsbTethering() {
        if (_uiState.value.rootState != RootState.GRANTED) return
        runCatching { UsbAutomationService.requestStartTethering(getApplication()) }
    }

    fun stopUsbTethering() {
        if (_uiState.value.rootState != RootState.GRANTED) return
        runCatching { UsbAutomationService.requestStopTethering(getApplication()) }
    }

    fun refreshPublicIp() {
        val state = _uiState.value
        if (state.isCheckingPublicIp || state.mobileReconnect.isRunning) return
        phoneControlRuntime.refreshPublicIpAsync()
    }

    fun reconnectMobileNetwork() {
        val state = _uiState.value
        if (state.rootState != RootState.GRANTED || state.mobileReconnect.isRunning) return
        phoneControlRuntime.reconnectMobileNetworkAsync()
    }

    private fun updateAutomation(transform: AutomationSettings.() -> AutomationSettings) {
        _uiState.update { state ->
            state.copy(automation = state.automation.transform())
        }
    }
}
