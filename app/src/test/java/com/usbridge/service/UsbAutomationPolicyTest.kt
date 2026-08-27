package com.usbridge.service

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class UsbAutomationPolicyTest {
    @Test
    fun `auto start requires a connected USB host and enabled rule`() {
        assertTrue(
            UsbAutomationPolicy.shouldAutoStart(
                connected = true,
                autoTetherEnabled = true,
                currentStatus = UsbAutomationStatus.MONITORING
            )
        )
        assertFalse(
            UsbAutomationPolicy.shouldAutoStart(
                connected = false,
                autoTetherEnabled = true,
                currentStatus = UsbAutomationStatus.MONITORING
            )
        )
    }

    @Test
    fun `active tethering is not started twice`() {
        assertFalse(
            UsbAutomationPolicy.shouldAutoStart(
                connected = true,
                autoTetherEnabled = true,
                currentStatus = UsbAutomationStatus.ACTIVE
            )
        )
    }

    @Test
    fun `disconnect rule only stops an active session`() {
        assertTrue(
            UsbAutomationPolicy.shouldStopAfterDisconnect(
                connected = false,
                stopOnDisconnect = true,
                currentStatus = UsbAutomationStatus.ACTIVE
            )
        )
        assertFalse(
            UsbAutomationPolicy.shouldStopAfterDisconnect(
                connected = false,
                stopOnDisconnect = true,
                currentStatus = UsbAutomationStatus.MONITORING
            )
        )
    }
}
