package com.usbridge.core.diagnostics

import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.os.BatteryManager
import android.os.Build
import com.usbridge.core.model.DeviceSnapshot
import com.usbridge.core.model.InterfaceKind
import com.usbridge.core.model.NetworkInterfaceSnapshot
import com.usbridge.core.model.RootState
import com.usbridge.core.model.TrafficSnapshot
import com.usbridge.core.model.UsbConnectionState
import com.usbridge.core.root.RootEnvironment
import com.usbridge.core.root.RootGateway
import com.usbridge.core.root.RootNetworkInterfaces
import com.usbridge.traffic.UsbTrafficReader
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

class DeviceDiagnostics(
    private val context: Context,
    private val rootGateway: RootGateway
) {
    private val trafficReader = UsbTrafficReader()

    suspend fun capture(rootState: RootState): DeviceSnapshot = withContext(Dispatchers.IO) {
        val rootEnvironment = if (rootState == RootState.GRANTED) {
            rootGateway.readEnvironment()
        } else {
            RootEnvironment()
        }
        val interfaces = if (rootState == RootState.GRANTED) {
            RootNetworkInterfaces.read().map { networkInterface ->
                networkInterface.copy(
                    ipv6Addresses = networkInterface.ipv6Addresses
                        .filterNot { it.startsWith("fe80:", ignoreCase = true) }
                )
            }
        } else {
            emptyList()
        }
        val usbInterfaces = interfaces.filter { it.kind == InterfaceKind.USB }
        val usbFunctions = rootEnvironment.usbState
            ?.takeIf(String::isNotBlank)
            ?: rootEnvironment.usbConfig?.takeIf(String::isNotBlank)

        DeviceSnapshot(
            manufacturer = Build.MANUFACTURER.toDisplayName(),
            model = Build.MODEL,
            androidVersion = Build.VERSION.RELEASE,
            apiLevel = Build.VERSION.SDK_INT,
            appVersion = readAppVersion(),
            selinuxMode = rootEnvironment.selinuxMode,
            rootImplementation = rootEnvironment.rootImplementation,
            rootUid = rootEnvironment.uid,
            usbConnectionState = determineUsbState(
                usbInterfaces = usbInterfaces,
                usbFunctions = usbFunctions,
                rootUsbConnected = rootEnvironment.usbDeviceConnected
            ),
            usbFunctions = usbFunctions,
            usbConfigured = rootEnvironment.usbConfigured,
            usbDataRole = rootEnvironment.usbDataRole,
            activeTransport = readActiveTransport(),
            interfaces = interfaces,
            availableRootTools = rootEnvironment.availableTools,
            traffic = readUsbTraffic()
        )
    }

    private fun determineUsbState(
        usbInterfaces: List<NetworkInterfaceSnapshot>,
        usbFunctions: String?,
        rootUsbConnected: Boolean?
    ): UsbConnectionState {
        return UsbStateResolver.resolve(
            hasReadyInterface = usbInterfaces.any { it.isUp },
            usbFunctions = usbFunctions,
            rootUsbConnected = rootUsbConnected,
            usbPowered = isUsbPowered()
        )
    }

    private fun isUsbPowered(): Boolean {
        val batteryIntent = context.registerReceiver(
            null,
            IntentFilter(Intent.ACTION_BATTERY_CHANGED)
        ) ?: return false
        return batteryIntent.getIntExtra(BatteryManager.EXTRA_PLUGGED, 0) ==
            BatteryManager.BATTERY_PLUGGED_USB
    }

    private fun readActiveTransport(): String {
        val manager = context.getSystemService(ConnectivityManager::class.java)
        val capabilities = manager.getNetworkCapabilities(manager.activeNetwork) ?: return "未连接"
        return when {
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "移动网络"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "Wi-Fi"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "以太网"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_VPN) -> "VPN"
            else -> "其他网络"
        }
    }

    private fun readUsbTraffic(): TrafficSnapshot {
        val sample = trafficReader.readSample()
        return TrafficSnapshot(
            uploadBytes = sample?.rxBytes ?: 0,
            downloadBytes = sample?.txBytes ?: 0
        )
    }

    @Suppress("DEPRECATION")
    private fun readAppVersion(): String = runCatching {
        context.packageManager.getPackageInfo(context.packageName, 0).versionName.orEmpty()
    }.getOrDefault("")

    private fun String.toDisplayName(): String = replaceFirstChar { character ->
        if (character.isLowerCase()) character.titlecase() else character.toString()
    }

}
