package com.usbridge.core.diagnostics

import com.usbridge.core.model.UsbConnectionState

object UsbStateResolver {
    fun resolve(
        hasReadyInterface: Boolean,
        usbFunctions: String?,
        rootUsbConnected: Boolean?,
        usbPowered: Boolean
    ): UsbConnectionState {
        if (hasReadyInterface) return UsbConnectionState.INTERFACE_READY

        val functions = usbFunctions.orEmpty().lowercase()
        if ("rndis" in functions || "ncm" in functions) {
            return UsbConnectionState.INTERFACE_READY
        }

        if (rootUsbConnected == true) return UsbConnectionState.CONNECTED
        if (rootUsbConnected == false) return UsbConnectionState.DISCONNECTED

        return if (usbPowered) {
            UsbConnectionState.CONNECTED
        } else {
            UsbConnectionState.DISCONNECTED
        }
    }
}
