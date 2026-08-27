package com.usbridge.core.root

import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

object RootWifiController {
    @Synchronized
    fun setEnabled(enabled: Boolean): Int {
        val state = if (enabled) "enabled" else "disabled"
        val action = if (enabled) "enable" else "disable"
        val commands = listOf(
            listOf("/system/bin/cmd", "wifi", "set-wifi-enabled", state),
            listOf("/system/bin/svc", "wifi", action)
        )
        return if (commands.any { execute(it) == 0 }) {
            RootControlCodes.SUCCESS
        } else {
            RootControlCodes.WIFI_CONTROL_FAILED
        }
    }

    private fun execute(command: List<String>): Int {
        return try {
            val process = ProcessBuilder(command)
                .redirectErrorStream(true)
                .start()
            val completed = CountDownLatch(1)
            val exitCode = AtomicInteger(COMMAND_FAILED_CODE)
            val waiter = Thread({
                try {
                    exitCode.set(process.waitFor())
                } catch (_: InterruptedException) {
                    Thread.currentThread().interrupt()
                } finally {
                    completed.countDown()
                }
            }, "USBridge-wifi-command-waiter")
            waiter.start()
            if (!completed.await(COMMAND_TIMEOUT_SECONDS, TimeUnit.SECONDS)) {
                process.destroy()
                waiter.interrupt()
                COMMAND_TIMEOUT_CODE
            } else {
                exitCode.get()
            }
        } catch (_: Throwable) {
            COMMAND_FAILED_CODE
        }
    }

    private const val COMMAND_TIMEOUT_SECONDS = 10L
    private const val COMMAND_TIMEOUT_CODE = -100
    private const val COMMAND_FAILED_CODE = -101
}
