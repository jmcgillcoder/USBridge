package com.usbridge.service

object UsbAutomationPolicy {
    fun shouldAutoStart(
        connected: Boolean,
        autoTetherEnabled: Boolean,
        currentStatus: UsbAutomationStatus
    ): Boolean = connected &&
        autoTetherEnabled &&
        currentStatus !in setOf(
            UsbAutomationStatus.ENABLING,
            UsbAutomationStatus.ACTIVE,
            UsbAutomationStatus.DISABLING
        )

    fun shouldStopAfterDisconnect(
        connected: Boolean,
        stopOnDisconnect: Boolean,
        currentStatus: UsbAutomationStatus
    ): Boolean = !connected &&
        stopOnDisconnect &&
        currentStatus == UsbAutomationStatus.ACTIVE
}
