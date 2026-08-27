package com.usbridge.core.root

import com.usbridge.core.model.InterfaceKind
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class RootNetworkInterfacesTest {
    @Test
    fun parsesLinkAndAddressSnapshots() {
        val result = parseRootNetworkInterfaces(
            listOf(
                "__USBRIDGE_LINKS__",
                "17: wlan0: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 state DOWN",
                "20: rmnet_data0@rmnet_ipa0: <UP,LOWER_UP> mtu 9216 state UNKNOWN",
                "44: rndis0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 state UP",
                "__USBRIDGE_IPV4__",
                "44: rndis0    inet 10.239.44.114/24 brd 10.239.44.255 scope global rndis0",
                "__USBRIDGE_IPV6__",
                "20: rmnet_data0    inet6 fe80::7002:ff:fef8:93ab/64 scope link",
                "44: rndis0    inet6 2408:8478:c760:78bd::cb/64 scope global"
            )
        )

        val mobile = result.first { it.name == "rmnet_data0" }
        assertEquals(InterfaceKind.CELLULAR, mobile.kind)
        assertTrue(mobile.isUp)
        assertEquals(listOf("fe80::7002:ff:fef8:93ab"), mobile.ipv6Addresses)

        val usb = result.first { it.name == "rndis0" }
        assertEquals(InterfaceKind.USB, usb.kind)
        assertTrue(usb.isUp)
        assertEquals(listOf("10.239.44.114"), usb.ipv4Addresses)
        assertEquals(listOf("2408:8478:c760:78bd::cb"), usb.ipv6Addresses)

        val wifi = result.first { it.name == "wlan0" }
        assertTrue(wifi.isUp)
        assertFalse(wifi.ipv4Addresses.isNotEmpty())
    }

    @Test
    fun classifiesInterfaceNamesByKind() {
        assertEquals(InterfaceKind.USB, classifyRootInterface("rndis0"))
        assertEquals(InterfaceKind.USB, classifyRootInterface("usb0"))
        assertEquals(InterfaceKind.USB, classifyRootInterface("ncm0"))
        assertEquals(InterfaceKind.USB, classifyRootInterface("RNDIS42"))
        assertEquals(InterfaceKind.WIFI, classifyRootInterface("wlan0"))
        assertEquals(InterfaceKind.CELLULAR, classifyRootInterface("rmnet_data2"))
        assertEquals(InterfaceKind.OTHER, classifyRootInterface("tun0"))
        assertEquals(InterfaceKind.OTHER, classifyRootInterface("eth0"))
    }

    @Test
    fun ignoresMalformedNamesAndAddresses() {
        val result = parseRootNetworkInterfaces(
            listOf(
                "__USBRIDGE_LINKS__",
                "1: bad name: <UP> mtu 1500",
                "2: rndis0: <UP> mtu 1500",
                "__USBRIDGE_IPV4__",
                "2: rndis0 inet 999.1.1.1/24 scope global",
                "__USBRIDGE_IPV6__",
                "2: rndis0 inet6 not-an-address/64 scope global"
            )
        )

        assertEquals(1, result.size)
        assertTrue(result.single().ipv4Addresses.isEmpty())
        assertTrue(result.single().ipv6Addresses.isEmpty())
    }
}
