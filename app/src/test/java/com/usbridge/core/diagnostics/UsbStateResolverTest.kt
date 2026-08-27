package com.usbridge.core.diagnostics

import com.usbridge.core.model.UsbConnectionState
import org.junit.Assert.assertEquals
import org.junit.Test

class UsbStateResolverTest {
    @Test
    fun `ready USB interface wins over disconnected root signal`() {
        val state = UsbStateResolver.resolve(
            hasReadyInterface = true,
            usbFunctions = "adb",
            rootUsbConnected = false,
            usbPowered = false
        )

        assertEquals(UsbConnectionState.INTERFACE_READY, state)
    }

    @Test
    fun `rndis function is treated as ready`() {
        val state = UsbStateResolver.resolve(
            hasReadyInterface = false,
            usbFunctions = "rndis,adb",
            rootUsbConnected = true,
            usbPowered = true
        )

        assertEquals(UsbConnectionState.INTERFACE_READY, state)
    }

    @Test
    fun `root disconnected signal prevents charger false positive`() {
        val state = UsbStateResolver.resolve(
            hasReadyInterface = false,
            usbFunctions = "adb",
            rootUsbConnected = false,
            usbPowered = true
        )

        assertEquals(UsbConnectionState.DISCONNECTED, state)
    }
}
