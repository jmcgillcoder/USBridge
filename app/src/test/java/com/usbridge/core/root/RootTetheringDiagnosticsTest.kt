package com.usbridge.core.root

import com.usbridge.core.model.InterfaceKind
import com.usbridge.core.model.NetworkInterfaceSnapshot
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class RootTetheringDiagnosticsTest {
    @Test
    fun parsesActiveDunTetheringSnapshot() {
        val snapshot = parseRootTetheringSnapshot(
            listOf(
                "Tethering:",
                "  Configuration:",
                "    isDunRequired: true",
                "  Tether state:",
                "    rndis0 - TetheredState - lastError = 0",
                "    Current upstream interface(s): [rmnet_data2]",
                "  Hardware offload:"
            )
        )

        assertTrue(snapshot.tetherStateKnown)
        assertTrue(snapshot.upstreamStateKnown)
        assertEquals(setOf("rndis0"), snapshot.tetheredInterfaceNames)
        assertEquals(setOf("rmnet_data2"), snapshot.upstreamInterfaceNames)
        assertEquals(true, snapshot.isDunRequired)
    }

    @Test
    fun parsesClatSetAndIgnoresMalformedNames() {
        val snapshot = parseRootTetheringSnapshot(
            listOf(
                "Current upstream interface(s): [v4-rmnet_data2, rmnet_data2, bad/name]",
                "ncm0 - TetheredState - lastError = 0",
                "wlan0 - AvailableState - lastError = 0"
            )
        )

        assertEquals(
            setOf("v4-rmnet_data2", "rmnet_data2"),
            snapshot.upstreamInterfaceNames
        )
        assertEquals(setOf("ncm0"), snapshot.tetheredInterfaceNames)
    }

    @Test
    fun distinguishesKnownEmptyUpstreamFromMissingField() {
        val empty = parseRootTetheringSnapshot(
            listOf("Current upstream interface(s): null")
        )
        val missing = parseRootTetheringSnapshot(listOf("Tether state:"))

        assertTrue(empty.upstreamStateKnown)
        assertTrue(empty.upstreamInterfaceNames.isEmpty())
        assertFalse(missing.upstreamStateKnown)
        assertNull(missing.upstreamTransport(emptyList()))
    }

    @Test
    fun resolvesBaseAndClatInterfacesFromEitherReportedName() {
        val interfaces = listOf(
            snapshot("rmnet_data2", InterfaceKind.CELLULAR, ipv6 = listOf("2607:fb90::2")),
            snapshot("v4-rmnet_data2", InterfaceKind.CELLULAR, ipv4 = listOf("192.0.0.4")),
            snapshot("rmnet_data3", InterfaceKind.CELLULAR, ipv4 = listOf("192.0.0.2"))
        )
        val fromBase = RootTetheringSnapshot(
            upstreamInterfaceNames = setOf("rmnet_data2"),
            upstreamStateKnown = true
        )
        val fromClat = RootTetheringSnapshot(
            upstreamInterfaceNames = setOf("v4-rmnet_data2"),
            upstreamStateKnown = true
        )

        assertEquals(
            listOf("rmnet_data2", "v4-rmnet_data2"),
            fromBase.resolveUpstreamInterfaces(interfaces).map { it.name }
        )
        assertEquals(
            listOf("rmnet_data2", "v4-rmnet_data2"),
            fromClat.resolveUpstreamInterfaces(interfaces).map { it.name }
        )
    }

    @Test
    fun identifiesUsbDownstreamAndWifiUpstream() {
        val interfaces = listOf(
            snapshot("rndis0", InterfaceKind.USB, ipv4 = listOf("10.0.0.1")),
            snapshot("wlan0", InterfaceKind.WIFI, ipv6 = listOf("2001:db8::2"))
        )
        val snapshot = RootTetheringSnapshot(
            tetheredInterfaceNames = setOf("rndis0"),
            upstreamInterfaceNames = setOf("v4-wlan0"),
            tetherStateKnown = true,
            upstreamStateKnown = true
        )

        assertEquals(true, snapshot.usbTetheringActive(interfaces))
        assertEquals("wifi", snapshot.upstreamTransport(interfaces))
    }

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
