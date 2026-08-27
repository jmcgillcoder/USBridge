package com.usbridge.core.root

import android.content.Context
import android.net.TetheringManager
import androidx.annotation.RequiresApi
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executor
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

@RequiresApi(36)
object Api36TetheringController {
    fun isAvailable(context: Context): Boolean =
        context.getSystemService(TetheringManager::class.java) != null

    fun startUsbTethering(context: Context): Int {
        return try {
            val manager = context.getSystemService(TetheringManager::class.java)
                ?: return RootControlCodes.SERVICE_UNAVAILABLE
            val resultCode = AtomicInteger(RootControlCodes.OPERATION_TIMEOUT)
            val completion = CountDownLatch(1)
            val request = TetheringManager.TetheringRequest.Builder(TETHERING_USB_TYPE).build()

            manager.startTethering(
                request,
                DIRECT_EXECUTOR,
                object : TetheringManager.StartTetheringCallback {
                    override fun onTetheringStarted() {
                        resultCode.set(RootControlCodes.SUCCESS)
                        completion.countDown()
                    }

                    override fun onTetheringFailed(error: Int) {
                        resultCode.set(RootControlCodes.FRAMEWORK_ERROR_OFFSET + error)
                        completion.countDown()
                    }
                }
            )
            completion.await(TETHERING_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            resultCode.get()
        } catch (_: SecurityException) {
            RootControlCodes.PERMISSION_DENIED
        } catch (_: Throwable) {
            RootControlCodes.INTERNAL_ERROR
        }
    }

    fun stopUsbTethering(context: Context): Int {
        return try {
            val manager = context.getSystemService(TetheringManager::class.java)
                ?: return RootControlCodes.SERVICE_UNAVAILABLE
            val resultCode = AtomicInteger(RootControlCodes.OPERATION_TIMEOUT)
            val completion = CountDownLatch(1)
            val request = TetheringManager.TetheringRequest.Builder(TETHERING_USB_TYPE).build()
            manager.stopTethering(
                request,
                DIRECT_EXECUTOR,
                object : TetheringManager.StopTetheringCallback {
                    override fun onStopTetheringSucceeded() {
                        resultCode.set(RootControlCodes.SUCCESS)
                        completion.countDown()
                    }

                    override fun onStopTetheringFailed(error: Int) {
                        resultCode.set(RootControlCodes.FRAMEWORK_ERROR_OFFSET + error)
                        completion.countDown()
                    }
                }
            )
            completion.await(TETHERING_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            resultCode.get()
        } catch (_: SecurityException) {
            RootControlCodes.PERMISSION_DENIED
        } catch (_: Throwable) {
            RootControlCodes.INTERNAL_ERROR
        }
    }

    private val DIRECT_EXECUTOR = Executor(Runnable::run)
    private const val TETHERING_TIMEOUT_SECONDS = 20L

    // USB tethering keeps the framework value 1 even though API 36 hides the named constant.
    private const val TETHERING_USB_TYPE = 1
}
