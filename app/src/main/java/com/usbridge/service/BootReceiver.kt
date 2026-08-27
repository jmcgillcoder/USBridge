package com.usbridge.service

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.usbridge.core.preferences.AppPreferences

class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED) return
        val settings = AppPreferences(context).readAutomationSettings()
        if (settings.startOnBoot) {
            runCatching { UsbAutomationService.startMonitoring(context) }
        }
    }
}
