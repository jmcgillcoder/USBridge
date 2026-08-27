package com.usbridge.service

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

enum class UsbAutomationStatus {
    STOPPED,
    MONITORING,
    USB_CONNECTED,
    ENABLING,
    ACTIVE,
    DISABLING,
    ERROR
}

data class UsbAutomationRuntimeState(
    val status: UsbAutomationStatus = UsbAutomationStatus.STOPPED,
    val usbConnected: Boolean = false,
    val usbConfigured: Boolean = false,
    val message: String = "自动共享当前已暂停",
    val trafficInterfaceName: String? = null,
    val trafficSessionStartedAtMillis: Long? = null,
    val sessionUploadBytes: Long = 0,
    val sessionDownloadBytes: Long = 0,
    val uploadBytesPerSecond: Long = 0,
    val downloadBytesPerSecond: Long = 0,
    val lastChangedAtMillis: Long = System.currentTimeMillis()
)

object UsbAutomationRuntime {
    private val _state = MutableStateFlow(UsbAutomationRuntimeState())
    val state: StateFlow<UsbAutomationRuntimeState> = _state.asStateFlow()

    fun update(
        status: UsbAutomationStatus = _state.value.status,
        usbConnected: Boolean = _state.value.usbConnected,
        usbConfigured: Boolean = _state.value.usbConfigured,
        message: String = _state.value.message
    ) {
        _state.update { current ->
            current.copy(
                status = status,
                usbConnected = usbConnected,
                usbConfigured = usbConfigured,
                message = message,
                lastChangedAtMillis = System.currentTimeMillis()
            )
        }
    }

    fun updateTraffic(
        interfaceName: String?,
        sessionStartedAtMillis: Long?,
        sessionUploadBytes: Long,
        sessionDownloadBytes: Long,
        uploadBytesPerSecond: Long,
        downloadBytesPerSecond: Long
    ) {
        _state.update { current ->
            current.copy(
                trafficInterfaceName = interfaceName,
                trafficSessionStartedAtMillis = sessionStartedAtMillis,
                sessionUploadBytes = sessionUploadBytes,
                sessionDownloadBytes = sessionDownloadBytes,
                uploadBytesPerSecond = uploadBytesPerSecond,
                downloadBytesPerSecond = downloadBytesPerSecond,
                lastChangedAtMillis = System.currentTimeMillis()
            )
        }
    }
}
