package com.usbridge.core.model

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class NetworkAddressFamiliesTest {
    @Test
    fun acceptsIpv6OnlyTetherAddress() {
        val networkInterface = snapshot(ipv6 = listOf("2607:fb90:69b:62b2::4e"))

        assertFalse(networkInterface.hasUsableIpv4Address())
        assertTrue(networkInterface.hasUsableIpv6Address())
        assertTrue(networkInterface.hasUsableIpAddress())
    }

    @Test
    fun rejectsLinkLocalOnlyInterface() {
        val networkInterface = snapshot(ipv6 = listOf("fe80::4e"))

        assertFalse(networkInterface.hasUsableIpv6Address())
        assertFalse(networkInterface.hasUsableIpAddress())
    }

    @Test
    fun recognizesClatIpv4Address() {
        val networkInterface = snapshot(ipv4 = listOf("192.0.0.4"))

        assertTrue(networkInterface.hasUsableIpv4Address())
        assertTrue(networkInterface.hasUsableIpAddress())
    }

    private fun snapshot(
        ipv4: List<String> = emptyList(),
        ipv6: List<String> = emptyList()
    ) = NetworkInterfaceSnapshot(
        name = "rndis0",
        kind = InterfaceKind.USB,
        isUp = true,
        ipv4Addresses = ipv4,
        ipv6Addresses = ipv6
    )
}
