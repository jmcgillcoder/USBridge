package com.usbridge.core.root

import com.topjohnwu.superuser.Shell
import com.usbridge.core.model.InterfaceKind
import com.usbridge.core.model.NetworkInterfaceSnapshot

/** Reads interface state through the Root shell so the app process never touches sysfs_net. */
object RootNetworkInterfaces {
    fun read(): List<NetworkInterfaceSnapshot> {
        val result = runCatching { Shell.cmd(SNAPSHOT_COMMAND).exec() }.getOrNull()
            ?: return emptyList()
        if (result.code != 0) return emptyList()
        return parseRootNetworkInterfaces(result.out)
    }

    private val SNAPSHOT_COMMAND = """
        echo $LINK_MARKER
        /system/bin/ip -o link show 2>/dev/null
        echo $IPV4_MARKER
        /system/bin/ip -o -4 addr show 2>/dev/null
        echo $IPV6_MARKER
        /system/bin/ip -o -6 addr show 2>/dev/null
    """.trimIndent()
}

internal fun parseRootNetworkInterfaces(lines: List<String>): List<NetworkInterfaceSnapshot> {
    val interfaces = linkedMapOf<String, MutableInterfaceSnapshot>()
    var section = SnapshotSection.NONE

    lines.forEach { rawLine ->
        val line = rawLine.trim()
        section = when (line) {
            LINK_MARKER -> SnapshotSection.LINK
            IPV4_MARKER -> SnapshotSection.IPV4
            IPV6_MARKER -> SnapshotSection.IPV6
            else -> section
        }
        if (line == LINK_MARKER || line == IPV4_MARKER || line == IPV6_MARKER) {
            return@forEach
        }

        when (section) {
            SnapshotSection.LINK -> LINK_LINE.matchEntire(line)?.let { match ->
                val name = normalizeInterfaceName(match.groupValues[1])
                if (!SAFE_INTERFACE_NAME.matches(name)) return@let
                val flags = match.groupValues[2]
                    .split(',')
                    .map(String::trim)
                    .toSet()
                interfaces.getOrPut(name) { MutableInterfaceSnapshot(name) }.isUp = "UP" in flags
            }

            SnapshotSection.IPV4,
            SnapshotSection.IPV6 -> ADDRESS_LINE.matchEntire(line)?.let { match ->
                val name = normalizeInterfaceName(match.groupValues[1])
                if (!SAFE_INTERFACE_NAME.matches(name)) return@let
                val family = match.groupValues[2]
                val address = match.groupValues[3]
                    .substringBefore('/')
                    .substringBefore('%')
                val snapshot = interfaces.getOrPut(name) { MutableInterfaceSnapshot(name) }
                when {
                    family == "inet" && isNumericIpv4(address) -> snapshot.ipv4 += address
                    family == "inet6" && isNumericIpv6(address) && address != "::1" ->
                        snapshot.ipv6 += address
                }
            }

            SnapshotSection.NONE -> Unit
        }
    }

    return interfaces.values
        .map { snapshot ->
            NetworkInterfaceSnapshot(
                name = snapshot.name,
                kind = classifyRootInterface(snapshot.name),
                isUp = snapshot.isUp,
                ipv4Addresses = snapshot.ipv4.distinct(),
                ipv6Addresses = snapshot.ipv6.distinct()
            )
        }
        .sortedWith(compareBy<NetworkInterfaceSnapshot> { it.kind.sortOrder }.thenBy { it.name })
}

internal fun classifyRootInterface(name: String): InterfaceKind {
    val normalized = name.lowercase()
    return when {
        normalized.startsWith("rndis") ||
            normalized.startsWith("usb") ||
            normalized.startsWith("ncm") -> InterfaceKind.USB

        normalized.contains("rmnet") ||
            normalized.startsWith("ccmni") ||
            normalized.startsWith("pdp") ||
            normalized.startsWith("wwan") -> InterfaceKind.CELLULAR

        normalized.startsWith("wlan") || normalized.startsWith("wifi") -> InterfaceKind.WIFI
        else -> InterfaceKind.OTHER
    }
}

private fun normalizeInterfaceName(value: String): String = value.substringBefore('@')

private fun isNumericIpv4(value: String): Boolean {
    val parts = value.split('.')
    return parts.size == 4 && parts.all { part ->
        part.isNotEmpty() && part.length <= 3 && part.all(Char::isDigit) &&
            part.toIntOrNull() in 0..255
    }
}

private fun isNumericIpv6(value: String): Boolean =
    value.contains(':') && value.all { it.isDigit() || it.lowercaseChar() in 'a'..'f' || it == ':' }

private val InterfaceKind.sortOrder: Int
    get() = when (this) {
        InterfaceKind.CELLULAR -> 0
        InterfaceKind.USB -> 1
        InterfaceKind.WIFI -> 2
        InterfaceKind.OTHER -> 3
    }

private data class MutableInterfaceSnapshot(
    val name: String,
    var isUp: Boolean = false,
    val ipv4: MutableList<String> = mutableListOf(),
    val ipv6: MutableList<String> = mutableListOf()
)

private enum class SnapshotSection {
    NONE,
    LINK,
    IPV4,
    IPV6
}

private const val LINK_MARKER = "__USBRIDGE_LINKS__"
private const val IPV4_MARKER = "__USBRIDGE_IPV4__"
private const val IPV6_MARKER = "__USBRIDGE_IPV6__"
private val LINK_LINE = Regex("""^\d+:\s+([^:]+):\s+<([^>]*)>.*$""")
private val ADDRESS_LINE = Regex("""^\d+:\s+(\S+)\s+(inet6?)\s+(\S+).*$""")
private val SAFE_INTERFACE_NAME = Regex("[A-Za-z0-9_.-]{1,32}")
