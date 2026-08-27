package com.usbridge.core.root

import com.topjohnwu.superuser.Shell
import com.usbridge.core.model.InterfaceKind
import com.usbridge.core.model.NetworkInterfaceSnapshot

data class RootTetheringSnapshot(
    val tetheredInterfaceNames: Set<String> = emptySet(),
    val upstreamInterfaceNames: Set<String> = emptySet(),
    val isDunRequired: Boolean? = null,
    val tetherStateKnown: Boolean = false,
    val upstreamStateKnown: Boolean = false
) {
    fun resolveUpstreamInterfaces(
        interfaces: List<NetworkInterfaceSnapshot>
    ): List<NetworkInterfaceSnapshot> {
        if (!upstreamStateKnown || upstreamInterfaceNames.isEmpty()) return emptyList()

        val relatedNames = linkedSetOf<String>()
        upstreamInterfaceNames.forEach { reportedName ->
            val baseName = reportedName.removePrefix(CLAT_INTERFACE_PREFIX)
            relatedNames += reportedName
            relatedNames += baseName
            relatedNames += "$CLAT_INTERFACE_PREFIX$baseName"
        }
        return interfaces.filter { it.name in relatedNames }
    }

    fun usbTetheringActive(interfaces: List<NetworkInterfaceSnapshot>): Boolean? {
        if (!tetherStateKnown) return null
        return tetheredInterfaceNames.any { interfaceName ->
            interfaces.firstOrNull { it.name == interfaceName }?.kind == InterfaceKind.USB ||
                classifyRootInterface(interfaceName) == InterfaceKind.USB
        }
    }

    fun upstreamTransport(interfaces: List<NetworkInterfaceSnapshot>): String? {
        if (!upstreamStateKnown) return null
        if (upstreamInterfaceNames.isEmpty()) return "none"

        val kinds = upstreamInterfaceNames.map { interfaceName ->
            val baseName = interfaceName.removePrefix(CLAT_INTERFACE_PREFIX)
            interfaces.firstOrNull { it.name == interfaceName || it.name == baseName }?.kind
                ?: classifyRootInterface(baseName)
        }
        return when {
            InterfaceKind.CELLULAR in kinds -> "cellular"
            InterfaceKind.WIFI in kinds -> "wifi"
            upstreamInterfaceNames.any { it.removePrefix(CLAT_INTERFACE_PREFIX).isEthernetName() } ->
                "ethernet"

            upstreamInterfaceNames.any { it.removePrefix(CLAT_INTERFACE_PREFIX).isVpnName() } -> "vpn"
            else -> "other"
        }
    }
}

object RootTetheringDiagnostics {
    fun read(): RootTetheringSnapshot? {
        val result = runCatching { Shell.cmd(TETHERING_COMMAND).exec() }.getOrNull()
            ?: return null
        if (result.code != 0) return null
        return parseRootTetheringSnapshot(result.out)
    }

    private const val TETHERING_COMMAND = "/system/bin/dumpsys tethering --short 2>/dev/null"
}

internal fun parseRootTetheringSnapshot(lines: List<String>): RootTetheringSnapshot {
    val tetheredInterfaces = linkedSetOf<String>()
    var upstreamInterfaces = emptySet<String>()
    var isDunRequired: Boolean? = null
    var tetherStateKnown = false
    var upstreamStateKnown = false

    lines.forEach { rawLine ->
        val line = rawLine.trim()
        if (TETHER_STATE_HEADER.matches(line)) {
            tetherStateKnown = true
        }

        TETHERED_INTERFACE.matchEntire(line)?.groupValues?.get(1)
            ?.takeIf(::isSafeTetheringInterfaceName)
            ?.let { interfaceName ->
                tetherStateKnown = true
                tetheredInterfaces += interfaceName
            }

        CURRENT_UPSTREAM.matchEntire(line)?.groupValues?.get(1)?.let { value ->
            upstreamStateKnown = true
            upstreamInterfaces = parseInterfaceSet(value)
        }

        DUN_REQUIRED.matchEntire(line)?.groupValues?.get(1)?.let { value ->
            isDunRequired = value.equals("true", ignoreCase = true)
        }
    }

    return RootTetheringSnapshot(
        tetheredInterfaceNames = tetheredInterfaces,
        upstreamInterfaceNames = upstreamInterfaces,
        isDunRequired = isDunRequired,
        tetherStateKnown = tetherStateKnown,
        upstreamStateKnown = upstreamStateKnown
    )
}

private fun parseInterfaceSet(rawValue: String): Set<String> {
    val value = rawValue.trim()
    if (value.isEmpty() || value.equals("null", ignoreCase = true) ||
        value.equals("none", ignoreCase = true) || value == "[]" || value == "<empty>"
    ) {
        return emptySet()
    }

    val contents = if (value.startsWith('[') && value.endsWith(']')) {
        value.substring(1, value.length - 1)
    } else {
        value
    }
    return contents.split(',')
        .asSequence()
        .map(String::trim)
        .filter(::isSafeTetheringInterfaceName)
        .filterNot { it.equals("null", ignoreCase = true) || it.equals("none", ignoreCase = true) }
        .toCollection(linkedSetOf())
}

private fun isSafeTetheringInterfaceName(value: String): Boolean =
    TETHERING_INTERFACE_NAME.matches(value)

private fun String.isEthernetName(): Boolean {
    val normalized = lowercase()
    return normalized.startsWith("eth") || normalized.startsWith("en")
}

private fun String.isVpnName(): Boolean {
    val normalized = lowercase()
    return normalized.startsWith("tun") || normalized.startsWith("tap") ||
        normalized.startsWith("wg")
}

private const val CLAT_INTERFACE_PREFIX = "v4-"
private val TETHER_STATE_HEADER = Regex("^Tether state:$", RegexOption.IGNORE_CASE)
private val TETHERED_INTERFACE = Regex(
    "^([A-Za-z0-9_.-]{1,32})\\s+-\\s+TetheredState(?:\\s+-.*)?$"
)
private val CURRENT_UPSTREAM = Regex(
    "^Current upstream interface\\(s\\)\\s*:\\s*(.*)$",
    RegexOption.IGNORE_CASE
)
private val DUN_REQUIRED = Regex(
    "^isDunRequired\\s*[:=]\\s*(true|false)$",
    RegexOption.IGNORE_CASE
)
private val TETHERING_INTERFACE_NAME = Regex("[A-Za-z0-9_.-]{1,32}")
