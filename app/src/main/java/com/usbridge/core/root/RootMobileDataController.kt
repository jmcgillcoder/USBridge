package com.usbridge.core.root

import java.util.concurrent.TimeUnit
import java.util.concurrent.CountDownLatch
import java.util.concurrent.atomic.AtomicInteger

object RootMobileDataController {
    @Volatile
    private var lastSuccessfulStrategy: MobileDataStrategy? = null

    @Synchronized
    fun setEnabled(enabled: Boolean): Int {
        val strategies = buildList {
            lastSuccessfulStrategy?.let(::add)
            addAll(MobileDataStrategy.entries.filterNot { it == lastSuccessfulStrategy })
        }
        val successfulStrategy = strategies.firstOrNull { strategy ->
            val command = if (enabled) strategy.enableCommand else strategy.disableCommand
            execute(command) == 0
        }
        if (successfulStrategy != null) {
            lastSuccessfulStrategy = successfulStrategy
            return RootControlCodes.SUCCESS
        }
        return if (enabled) {
            RootControlCodes.MOBILE_DATA_ENABLE_FAILED
        } else {
            RootControlCodes.MOBILE_DATA_DISABLE_FAILED
        }
    }

    @Synchronized
    fun reconnect(requestedDownTimeMillis: Int): Int {
        val downTimeMillis = requestedDownTimeMillis.coerceIn(
            MIN_DOWN_TIME_MILLIS,
            MAX_DOWN_TIME_MILLIS
        )
        val disableResult = setEnabled(false)
        if (disableResult != RootControlCodes.SUCCESS) return disableResult

        return try {
            Thread.sleep(downTimeMillis.toLong())
            setEnabled(true)
        } catch (_: InterruptedException) {
            Thread.currentThread().interrupt()
            if (setEnabled(true) == RootControlCodes.SUCCESS) {
                RootControlCodes.INTERNAL_ERROR
            } else {
                RootControlCodes.MOBILE_DATA_ENABLE_FAILED
            }
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
            }, "USBridge-command-waiter")
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

    private enum class MobileDataStrategy(
        val disableCommand: List<String>,
        val enableCommand: List<String>
    ) {
        PHONE_CMD(
            disableCommand = listOf("/system/bin/cmd", "phone", "data", "disable"),
            enableCommand = listOf("/system/bin/cmd", "phone", "data", "enable")
        ),
        LEGACY_SVC(
            disableCommand = listOf("/system/bin/svc", "data", "disable"),
            enableCommand = listOf("/system/bin/svc", "data", "enable")
        )
    }

    private const val MIN_DOWN_TIME_MILLIS = 500
    private const val MAX_DOWN_TIME_MILLIS = 10_000
    private const val COMMAND_TIMEOUT_SECONDS = 10L
    private const val COMMAND_TIMEOUT_CODE = -100
    private const val COMMAND_FAILED_CODE = -101
}
