package com.usbridge.traffic

import org.junit.Assert.assertEquals
import org.junit.Test

class TrafficDeltaCalculatorTest {

    @Test
    fun parsesRootUsbCounterOutput() {
        val counters = parseUsbCounterLines(
            listOf(
                "rndis0 123 456",
                "ncm0 42 84",
                "wlan0 999 999",
                "usb-bad not-a-number 3"
            )
        )

        assertEquals(2, counters.size)
        assertEquals(UsbCounterValues("rndis0", 123, 456), counters[0])
        assertEquals(UsbCounterValues("ncm0", 42, 84), counters[1])
    }
    @Test
    fun `calculates upload download and speed from USB counters`() {
        val previous = TrafficCounterSample("rndis0", 1_000, 2_000, 1_000)
        val current = TrafficCounterSample("rndis0", 3_000, 6_000, 3_000)

        val delta = TrafficDeltaCalculator.calculate(previous, current)

        assertEquals(2_000, delta.uploadBytes)
        assertEquals(4_000, delta.downloadBytes)
        assertEquals(1_000, delta.uploadBytesPerSecond)
        assertEquals(2_000, delta.downloadBytesPerSecond)
    }

    @Test
    fun `counter reset does not create a false traffic spike`() {
        val previous = TrafficCounterSample("rndis0", 10_000, 20_000, 1_000)
        val current = TrafficCounterSample("rndis0", 100, 200, 2_000)

        val delta = TrafficDeltaCalculator.calculate(previous, current)

        assertEquals(0, delta.uploadBytes)
        assertEquals(0, delta.downloadBytes)
    }

    @Test
    fun `interface change starts with an empty delta`() {
        val previous = TrafficCounterSample("rndis0", 1_000, 2_000, 1_000)
        val current = TrafficCounterSample("ncm0", 3_000, 4_000, 2_000)

        val delta = TrafficDeltaCalculator.calculate(previous, current)

        assertEquals(0, delta.uploadBytes)
        assertEquals(0, delta.downloadBytes)
    }
}
