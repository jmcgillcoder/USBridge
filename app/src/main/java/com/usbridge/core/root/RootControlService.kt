package com.usbridge.core.root

import android.content.Intent
import android.os.Build
import android.os.IBinder
import android.os.Process
import com.topjohnwu.superuser.ipc.RootService

class RootControlService : RootService() {
    private val binder = object : IRootControlService.Stub() {
        override fun probe(): String {
            val tetheringAvailable = when {
                Build.VERSION.SDK_INT >= API_36 ->
                    Api36TetheringController.isAvailable(this@RootControlService)
                else -> Api30TetheringController.isAvailable(this@RootControlService)
            }
            return "uid=${Process.myUid()};tetheringManager=$tetheringAvailable"
        }

        override fun startUsbTethering(): Int = startUsbTetheringInternal()

        override fun stopUsbTethering(): Int = stopUsbTetheringInternal()

        override fun setMobileDataEnabled(enabled: Boolean): Int =
            RootMobileDataController.setEnabled(enabled)

        override fun setWifiEnabled(enabled: Boolean): Int =
            RootWifiController.setEnabled(enabled)

        override fun reconnectMobileData(downTimeMillis: Int): Int =
            RootMobileDataController.reconnect(downTimeMillis)
    }

    override fun onBind(intent: Intent): IBinder = binder

    private fun startUsbTetheringInternal(): Int {
        return when {
            Build.VERSION.SDK_INT >= API_36 ->
                Api36TetheringController.startUsbTethering(this)
            else -> Api30TetheringController.startUsbTethering(this)
        }
    }

    private fun stopUsbTetheringInternal(): Int {
        return when {
            Build.VERSION.SDK_INT >= API_36 ->
                Api36TetheringController.stopUsbTethering(this)
            else -> Api30TetheringController.stopUsbTethering(this)
        }
    }

    private companion object {
        const val API_36 = 36
    }
}

object RootControlCodes {
    const val SUCCESS = 0
    const val OPERATION_TIMEOUT = -1
    const val SERVICE_UNAVAILABLE = -2
    const val PERMISSION_DENIED = -3
    const val UNSUPPORTED_ANDROID_VERSION = -4
    const val INTERNAL_ERROR = -5
    const val MOBILE_DATA_DISABLE_FAILED = -6
    const val MOBILE_DATA_ENABLE_FAILED = -7
    const val WIFI_CONTROL_FAILED = -8
    const val FRAMEWORK_ERROR_OFFSET = 1000
}
