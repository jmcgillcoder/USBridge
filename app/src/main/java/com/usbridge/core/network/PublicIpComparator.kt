package com.usbridge.core.network

import com.usbridge.core.model.MobileReconnectStatus
import com.usbridge.core.model.PublicIpSnapshot

data class PublicIpComparison(
    val status: MobileReconnectStatus,
    val message: String
)

object PublicIpComparator {
    fun compare(before: PublicIpSnapshot, after: PublicIpSnapshot): PublicIpComparison {
        val comparablePairs = listOfNotNull(
            before.ipv4?.let { old -> after.ipv4?.let { new -> old to new } },
            before.ipv6?.let { old -> after.ipv6?.let { new -> old to new } }
        )
        return when {
            comparablePairs.any { (old, new) -> old != new } -> PublicIpComparison(
                MobileReconnectStatus.IP_CHANGED,
                "重连完成，公网 IP 已变化"
            )

            comparablePairs.isNotEmpty() -> PublicIpComparison(
                MobileReconnectStatus.IP_UNCHANGED,
                "重连完成，但运营商仍分配了相同公网 IP"
            )

            else -> PublicIpComparison(
                MobileReconnectStatus.COMPLETED_WITHOUT_IP,
                "重连完成，但没有足够数据比较公网 IP"
            )
        }
    }
}
