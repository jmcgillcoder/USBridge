package com.usbridge.core.network

import com.usbridge.core.model.MobileReconnectStatus
import com.usbridge.core.model.PublicIpSnapshot
import org.junit.Assert.assertEquals
import org.junit.Test

class PublicIpComparatorTest {
    @Test
    fun `reports changed when either comparable family changes`() {
        val result = PublicIpComparator.compare(
            before = PublicIpSnapshot(ipv4 = "1.1.1.1", ipv6 = "2001:db8::1"),
            after = PublicIpSnapshot(ipv4 = "2.2.2.2", ipv6 = "2001:db8::1")
        )

        assertEquals(MobileReconnectStatus.IP_CHANGED, result.status)
    }

    @Test
    fun `reports unchanged when all comparable addresses match`() {
        val result = PublicIpComparator.compare(
            before = PublicIpSnapshot(ipv4 = "1.1.1.1"),
            after = PublicIpSnapshot(ipv4 = "1.1.1.1")
        )

        assertEquals(MobileReconnectStatus.IP_UNCHANGED, result.status)
    }

    @Test
    fun `reports unverifiable without matching address families`() {
        val result = PublicIpComparator.compare(
            before = PublicIpSnapshot(ipv4 = "1.1.1.1"),
            after = PublicIpSnapshot(ipv6 = "2001:db8::1")
        )

        assertEquals(MobileReconnectStatus.COMPLETED_WITHOUT_IP, result.status)
    }
}
