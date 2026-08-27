package com.usbridge.core.preferences

import android.content.Context
import androidx.core.content.edit
import com.usbridge.core.model.AutomationSettings
import com.usbridge.core.model.IpMode

class AppPreferences(context: Context) {
    private val preferences = context.getSharedPreferences(FILE_NAME, Context.MODE_PRIVATE)

    fun readAutomationSettings(): AutomationSettings = AutomationSettings(
        autoTetherOnUsb = preferences.getBoolean(KEY_AUTO_TETHER, false),
        stopOnDisconnect = preferences.getBoolean(KEY_STOP_ON_DISCONNECT, true),
        startOnBoot = preferences.getBoolean(KEY_START_ON_BOOT, false),
        retryOnFailure = preferences.getBoolean(KEY_RETRY_ON_FAILURE, true),
        ipMode = preferences.getString(KEY_IP_MODE, null)
            ?.let { stored -> IpMode.entries.firstOrNull { it.name == stored } }
            ?: IpMode.AUTO
    )

    fun setAutoTether(enabled: Boolean) = update(KEY_AUTO_TETHER, enabled)

    fun setStopOnDisconnect(enabled: Boolean) = update(KEY_STOP_ON_DISCONNECT, enabled)

    fun setStartOnBoot(enabled: Boolean) = update(KEY_START_ON_BOOT, enabled)

    fun setRetryOnFailure(enabled: Boolean) = update(KEY_RETRY_ON_FAILURE, enabled)

    fun setIpMode(mode: IpMode) {
        preferences.edit { putString(KEY_IP_MODE, mode.name) }
    }

    fun shouldRestoreWifi(): Boolean =
        preferences.getBoolean(KEY_RESTORE_WIFI, false)

    fun setRestoreWifiPending(pending: Boolean) = update(KEY_RESTORE_WIFI, pending)

    private fun update(key: String, value: Boolean) {
        preferences.edit { putBoolean(key, value) }
    }

    private companion object {
        const val FILE_NAME = "usbridge_preferences"
        const val KEY_AUTO_TETHER = "auto_tether_on_usb"
        const val KEY_STOP_ON_DISCONNECT = "stop_on_disconnect"
        const val KEY_START_ON_BOOT = "start_on_boot"
        const val KEY_RETRY_ON_FAILURE = "retry_on_failure"
        const val KEY_IP_MODE = "ip_mode"
        const val KEY_RESTORE_WIFI = "restore_wifi_after_usb"
    }
}
