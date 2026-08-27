package com.usbridge.control

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PhoneControlServerTest {
    @Test
    fun acceptsDesktopMutationHeaderWithoutBrowserOrigin() {
        assertTrue(
            isDesktopControlRequest(
                mapOf("x-usbridge-client" to "desktop")
            )
        )
        assertFalse(
            isDesktopControlRequest(
                mapOf(
                    "x-usbridge-client" to "desktop",
                    "origin" to "https://example.invalid"
                )
            )
        )
        assertFalse(isDesktopControlRequest(emptyMap()))
    }
}
