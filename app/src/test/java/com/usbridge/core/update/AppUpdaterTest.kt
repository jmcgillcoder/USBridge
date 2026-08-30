package com.usbridge.core.update

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class AppUpdaterTest {
    @Test
    fun comparesSemanticVersionsAndIgnoresBuildSuffix() {
        assertEquals(1, compareVersions("0.3.1", "0.3.0"))
        assertEquals(0, compareVersions("v1.0.0", "1.0.0-dev"))
        assertEquals(-1, compareVersions("0.2.9", "0.3.0"))
    }

    @Test
    fun findsChecksumForExactAsset() {
        val contents = """
            aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  first.apk
            bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb *USBridge.apk
        """.trimIndent()
        assertEquals(
            "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
            checksumFor(contents, "USBridge.apk")
        )
        assertNull(checksumFor(contents, "missing.apk"))
    }
}
