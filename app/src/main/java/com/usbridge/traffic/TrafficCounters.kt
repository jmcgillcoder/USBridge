package com.usbridge.traffic

import com.topjohnwu.superuser.Shell

data class TrafficCounterSample(
    val interfaceName: String,
    val rxBytes: Long,
    val txBytes: Long,
    val timestampMillis: Long
)

data class TrafficDelta(
    val uploadBytes: Long,
    val downloadBytes: Long,
    val elapsedMillis: Long,
    val uploadBytesPerSecond: Long,
    val downloadBytesPerSecond: Long
)

object TrafficDeltaCalculator {
    fun calculate(previous: TrafficCounterSample, current: TrafficCounterSample): TrafficDelta {
        if (previous.interfaceName != current.interfaceName) return EMPTY_DELTA
        val elapsed = (current.timestampMillis - previous.timestampMillis).coerceAtLeast(1)
        val upload = monotonicDelta(previous.rxBytes, current.rxBytes)
        val download = monotonicDelta(previous.txBytes, current.txBytes)
        return TrafficDelta(
            uploadBytes = upload,
            downloadBytes = download,
            elapsedMillis = elapsed,
            uploadBytesPerSecond = upload * MILLIS_PER_SECOND / elapsed,
            downloadBytesPerSecond = download * MILLIS_PER_SECOND / elapsed
        )
    }

    private fun monotonicDelta(previous: Long, current: Long): Long =
        if (current >= previous) current - previous else 0

    private const val MILLIS_PER_SECOND = 1_000L
    private val EMPTY_DELTA = TrafficDelta(0, 0, 0, 0, 0)
}

class UsbTrafficReader {
    fun readSample(nowMillis: Long = System.currentTimeMillis()): TrafficCounterSample? {
        val result = runCatching { Shell.cmd(READ_COUNTERS_COMMAND).exec() }.getOrNull()
            ?: return null
        if (result.code != 0) return null
        return parseUsbCounterLines(result.out)
            .sortedBy { candidatePriority(it.interfaceName) }
            .firstOrNull()
            ?.let { counters ->
                TrafficCounterSample(
                    interfaceName = counters.interfaceName,
                    rxBytes = counters.rxBytes,
                    txBytes = counters.txBytes,
                    timestampMillis = nowMillis
                )
            }
    }

    private fun candidatePriority(name: String): Int = when {
        name.startsWith("rndis", ignoreCase = true) -> 0
        name.startsWith("ncm", ignoreCase = true) -> 1
        else -> 2
    }

    private companion object {
        val READ_COUNTERS_COMMAND = """
            for path in /sys/class/net/rndis* /sys/class/net/ncm* /sys/class/net/usb*; do
                [ -d "${'$'}path" ] || continue
                name=${'$'}{path##*/}
                rx=$(/system/bin/cat "${'$'}path/statistics/rx_bytes" 2>/dev/null) || continue
                tx=$(/system/bin/cat "${'$'}path/statistics/tx_bytes" 2>/dev/null) || continue
                printf '%s %s %s\n' "${'$'}name" "${'$'}rx" "${'$'}tx"
            done
        """.trimIndent()
    }
}

internal data class UsbCounterValues(
    val interfaceName: String,
    val rxBytes: Long,
    val txBytes: Long
)

internal fun parseUsbCounterLines(lines: List<String>): List<UsbCounterValues> =
    lines.mapNotNull { line ->
        val parts = line.trim().split(WHITESPACE).filter(String::isNotEmpty)
        if (parts.size != 3 || !SAFE_INTERFACE_NAME.matches(parts[0])) {
            return@mapNotNull null
        }
        val name = parts[0]
        if (!name.startsWith("rndis", ignoreCase = true) &&
            !name.startsWith("ncm", ignoreCase = true) &&
            !name.startsWith("usb", ignoreCase = true)
        ) {
            return@mapNotNull null
        }
        val rxBytes = parts[1].toLongOrNull()?.takeIf { it >= 0 } ?: return@mapNotNull null
        val txBytes = parts[2].toLongOrNull()?.takeIf { it >= 0 } ?: return@mapNotNull null
        UsbCounterValues(name, rxBytes, txBytes)
    }

private val SAFE_INTERFACE_NAME = Regex("[A-Za-z0-9_.-]{1,32}")
private val WHITESPACE = Regex("\\s+")
