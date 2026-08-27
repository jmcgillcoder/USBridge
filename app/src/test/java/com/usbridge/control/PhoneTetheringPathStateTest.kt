package com.usbridge.control

import com.usbridge.core.model.InterfaceKind
import com.usbridge.core.model.NetworkInterfaceSnapshot
import com.usbridge.core.root.RootTetheringSnapshot
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PhoneTetheringPathStateTest {
    @Test
    fun usesOnlyRealDunUpstreamForAddressFamilies() {
        val path = buildPhoneTetheringPathState(
            interfaces = listOf(
                snapshot("rndis0", InterfaceKind.USB, ipv4 = listOf("10.179.36.200")),
                snapshot("rmnet_data2", InterfaceKind.CELLULAR, ipv6 = listOf("2607:fb90::2")),
                snapshot(
                    "rmnet_data3",
                    InterfaceKind.CELLULAR,
                    ipv4 = listOf("192.0.0.2"),
                    ipv6 = listOf("2607:fb90::3")
                )
            ),
            tethering = activeTethering(setOf("rmnet_data2")),
            fallbackUpstreamTransport = "wifi"
        )

        assertTrue(path.tetheringEnabled)
        assertEquals("cellular", path.upstreamTransport)
        assertEquals(listOf("rmnet_data2"), path.mobileInterfaceNames)
        assertFalse(path.ipv4Available)
        assertTrue(path.ipv6Available)
    }

    @Test
    fun includesClatStackedInterfaceForIpv4Availability() {
        val path = buildPhoneTetheringPathState(
            interfaces = listOf(
                snapshot("rndis0", InterfaceKind.USB, ipv4 = listOf("10.0.0.1")),
                snapshot("rmnet_data2", InterfaceKind.CELLULAR, ipv6 = listOf("2001:db8::2")),
                snapshot("v4-rmnet_data2", InterfaceKind.CELLULAR, ipv4 = listOf("192.0.0.4"))
            ),
            tethering = activeTethering(setOf("rmnet_data2")),
            fallbackUpstreamTransport = "wifi"
        )

        assertTrue(path.ipv4Available)
        assertTrue(path.ipv6Available)
        assertEquals(listOf("rmnet_data2", "v4-rmnet_data2"), path.mobileInterfaceNames)
    }

    @Test
    fun knownEmptyUpstreamDoesNotFallBackToOtherMobileInterfaces() {
        val path = buildPhoneTetheringPathState(
            interfaces = listOf(
                snapshot("rmnet_data3", InterfaceKind.CELLULAR, ipv4 = listOf("192.0.0.2"))
            ),
            tethering = RootTetheringSnapshot(upstreamStateKnown = true),
            fallbackUpstreamTransport = "cellular"
        )

        assertEquals("none", path.upstreamTransport)
        assertFalse(path.ipv4Available)
        assertFalse(path.ipv6Available)
    }

    @Test
    fun unavailableDiagnosticsRetainLegacyFallback() {
        val path = buildPhoneTetheringPathState(
            interfaces = listOf(
                snapshot("rndis0", InterfaceKind.USB, ipv4 = listOf("10.0.0.1")),
                snapshot("rmnet_data3", InterfaceKind.CELLULAR, ipv4 = listOf("192.0.0.2"))
            ),
            tethering = null,
            fallbackUpstreamTransport = "cellular"
        )

        assertTrue(path.tetheringEnabled)
        assertEquals("cellular", path.upstreamTransport)
        assertTrue(path.ipv4Available)
        assertFalse(path.diagnosticsAvailable)
    }

    private fun activeTethering(upstream: Set<String>) = RootTetheringSnapshot(
        tetheredInterfaceNames = setOf("rndis0"),
        upstreamInterfaceNames = upstream,
        tetherStateKnown = true,
        upstreamStateKnown = true
    )

    private fun snapshot(
        name: String,
        kind: InterfaceKind,
        ipv4: List<String> = emptyList(),
        ipv6: List<String> = emptyList()
    ) = NetworkInterfaceSnapshot(
        name = name,
        kind = kind,
        isUp = true,
        ipv4Addresses = ipv4,
        ipv6Addresses = ipv6
    )
}
