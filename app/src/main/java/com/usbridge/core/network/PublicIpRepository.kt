package com.usbridge.core.network

import android.content.Context
import android.net.ConnectivityManager
import android.net.InetAddresses
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import com.usbridge.BuildConfig
import com.usbridge.core.model.PublicIpCheckResult
import com.usbridge.core.model.PublicIpSnapshot
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import java.net.HttpURLConnection
import java.net.Inet4Address
import java.net.Inet6Address
import java.net.URL
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.ConcurrentHashMap
import kotlin.coroutines.resume

class PublicIpRepository(context: Context) {
    private val connectivityManager = context.getSystemService(ConnectivityManager::class.java)
    private val cellularNetworks = ConcurrentHashMap.newKeySet<Network>()
    private val trackingCallback = object : ConnectivityManager.NetworkCallback() {
        override fun onAvailable(network: Network) {
            cellularNetworks += network
        }

        override fun onLost(network: Network) {
            cellularNetworks -= network
        }

        override fun onCapabilitiesChanged(
            network: Network,
            capabilities: NetworkCapabilities
        ) {
            if (capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) &&
                capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            ) {
                cellularNetworks += network
            } else {
                cellularNetworks -= network
            }
        }
    }

    init {
        val request = NetworkRequest.Builder()
            .addTransportType(NetworkCapabilities.TRANSPORT_CELLULAR)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .build()
        runCatching { connectivityManager.registerNetworkCallback(request, trackingCallback) }
    }

    fun close() {
        runCatching { connectivityManager.unregisterNetworkCallback(trackingCallback) }
        cellularNetworks.clear()
    }

    suspend fun checkCellularPublicIps(): PublicIpCheckResult {
        val network = findExistingCellularNetwork(requireValidated = false)
            ?: requestCellularNetwork()
            ?: return PublicIpCheckResult(errorMessage = "没有可用的移动数据网络")

        val (ipv4, ipv6) = coroutineScope {
            val ipv4Deferred = async(Dispatchers.IO) {
                queryAddress(network, IPV4_ENDPOINTS, AddressFamily.IPV4)
            }
            val ipv6Deferred = async(Dispatchers.IO) {
                queryAddress(network, IPV6_ENDPOINTS, AddressFamily.IPV6)
            }
            ipv4Deferred.await() to ipv6Deferred.await()
        }
        val snapshot = PublicIpSnapshot(
            ipv4 = ipv4,
            ipv6 = ipv6,
            checkedAtMillis = System.currentTimeMillis()
        )
        return PublicIpCheckResult(
            snapshot = snapshot,
            cellularNetworkAvailable = true,
            errorMessage = when {
                ipv4 == null && ipv6 == null -> "移动网络可用，但公网 IP 查询失败"
                ipv4 == null -> "未检测到 IPv4 公网地址"
                ipv6 == null -> "未检测到 IPv6 公网地址"
                else -> null
            }
        )
    }

    suspend fun awaitCellularInternet(timeoutMillis: Long = DEFAULT_NETWORK_WAIT_MILLIS): Boolean {
        val deadline = System.currentTimeMillis() + timeoutMillis
        while (System.currentTimeMillis() < deadline) {
            if (findExistingCellularNetwork(requireValidated = true) != null) return true
            delay(NETWORK_POLL_INTERVAL_MILLIS)
        }
        return findExistingCellularNetwork(requireValidated = false) != null
    }

    suspend fun awaitCellularUnavailable(
        timeoutMillis: Long = DEFAULT_NETWORK_LOSS_WAIT_MILLIS
    ): Boolean {
        val deadline = System.currentTimeMillis() + timeoutMillis
        while (System.currentTimeMillis() < deadline) {
            if (findExistingCellularNetwork(requireValidated = true) == null) return true
            delay(NETWORK_POLL_INTERVAL_MILLIS)
        }
        return findExistingCellularNetwork(requireValidated = true) == null
    }

    private fun findExistingCellularNetwork(requireValidated: Boolean): Network? {
        return cellularNetworks.firstOrNull { network ->
            val capabilities = connectivityManager.getNetworkCapabilities(network)
                ?: return@firstOrNull false
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) &&
                capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) &&
                (!requireValidated ||
                    capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED))
        }
    }

    private suspend fun requestCellularNetwork(): Network? = withTimeoutOrNull(
        CELLULAR_REQUEST_TIMEOUT_MILLIS
    ) {
        suspendCancellableCoroutine { continuation ->
            val completed = AtomicBoolean(false)
            lateinit var callback: ConnectivityManager.NetworkCallback

            fun finish(network: Network?) {
                if (!completed.compareAndSet(false, true)) return
                runCatching { connectivityManager.unregisterNetworkCallback(callback) }
                if (continuation.isActive) continuation.resume(network)
            }

            callback = object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(network: Network) = finish(network)
                override fun onUnavailable() = finish(null)
            }
            continuation.invokeOnCancellation {
                if (completed.compareAndSet(false, true)) {
                    runCatching { connectivityManager.unregisterNetworkCallback(callback) }
                }
            }
            val request = NetworkRequest.Builder()
                .addTransportType(NetworkCapabilities.TRANSPORT_CELLULAR)
                .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                .build()
            runCatching { connectivityManager.requestNetwork(request, callback) }
                .onFailure { finish(null) }
        }
    }

    private suspend fun queryAddress(
        network: Network,
        endpoints: List<String>,
        family: AddressFamily
    ): String? = withContext(Dispatchers.IO) {
        endpoints.firstNotNullOfOrNull { endpoint ->
            runCatching {
                val connection = network.openConnection(URL(endpoint)) as HttpURLConnection
                try {
                    connection.connectTimeout = HTTP_TIMEOUT_MILLIS
                    connection.readTimeout = HTTP_TIMEOUT_MILLIS
                    connection.instanceFollowRedirects = true
                    connection.setRequestProperty("Accept", "text/plain")
                    connection.setRequestProperty(
                        "User-Agent",
                        "USBridge-Android/${BuildConfig.VERSION_NAME}"
                    )
                    connection.inputStream.bufferedReader().use { reader ->
                        reader.readText().trim().take(MAX_ADDRESS_LENGTH)
                    }
                } finally {
                    connection.disconnect()
                }
            }.getOrNull()?.takeIf { address -> address.matchesFamily(family) }
        }
    }

    private fun String.matchesFamily(family: AddressFamily): Boolean {
        if (isBlank() || any { it !in ALLOWED_ADDRESS_CHARACTERS }) return false
        if (!InetAddresses.isNumericAddress(this)) return false
        val address = runCatching { InetAddresses.parseNumericAddress(this) }.getOrNull()
            ?: return false
        return when (family) {
            AddressFamily.IPV4 -> address is Inet4Address
            AddressFamily.IPV6 -> address is Inet6Address
        }
    }

    private enum class AddressFamily {
        IPV4,
        IPV6
    }

    private companion object {
        val IPV4_ENDPOINTS = listOf(
            "https://api4.ipify.org",
            "https://v4.ident.me",
            "https://ipv4.icanhazip.com"
        )
        val IPV6_ENDPOINTS = listOf(
            "https://api6.ipify.org",
            "https://v6.ident.me",
            "https://ipv6.icanhazip.com"
        )
        val ALLOWED_ADDRESS_CHARACTERS = "0123456789abcdefABCDEF:.".toSet()
        const val MAX_ADDRESS_LENGTH = 64
        const val HTTP_TIMEOUT_MILLIS = 5_000
        const val CELLULAR_REQUEST_TIMEOUT_MILLIS = 12_000L
        const val DEFAULT_NETWORK_WAIT_MILLIS = 20_000L
        const val DEFAULT_NETWORK_LOSS_WAIT_MILLIS = 8_000L
        const val NETWORK_POLL_INTERVAL_MILLIS = 500L
    }
}
