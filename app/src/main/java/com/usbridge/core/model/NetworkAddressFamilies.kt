package com.usbridge.core.model

fun NetworkInterfaceSnapshot.hasUsableIpv4Address(): Boolean = ipv4Addresses.isNotEmpty()

fun NetworkInterfaceSnapshot.hasUsableIpv6Address(): Boolean =
    ipv6Addresses.any(::isUsableIpv6Address)

fun NetworkInterfaceSnapshot.hasUsableIpAddress(): Boolean =
    hasUsableIpv4Address() || hasUsableIpv6Address()

internal fun isUsableIpv6Address(address: String): Boolean {
    if (address == "::" || address == "::1") return false
    val firstHextet = address.substringBefore(':').toIntOrNull(16) ?: return false
    val isLinkLocal = firstHextet and 0xffc0 == 0xfe80
    val isMulticast = firstHextet and 0xff00 == 0xff00
    return !isLinkLocal && !isMulticast
}
