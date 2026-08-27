package com.usbridge.core.root

import android.content.Context
import java.lang.reflect.Proxy
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executor
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

/** USB tethering support for Android 11 through Android 15. */
object Api30TetheringController {
    fun isAvailable(context: Context): Boolean = runCatching {
        Class.forName(TETHERING_MANAGER_CLASS)
        context.getSystemService(TETHERING_SERVICE_NAME) != null
    }.getOrDefault(false)

    fun startUsbTethering(context: Context): Int {
        return try {
            val manager = context.getSystemService(TETHERING_SERVICE_NAME)
                ?: return RootControlCodes.SERVICE_UNAVAILABLE
            val managerClass = Class.forName(TETHERING_MANAGER_CLASS)
            val requestClass = Class.forName(TETHERING_REQUEST_CLASS)
            val requestBuilderClass = Class.forName(TETHERING_REQUEST_BUILDER_CLASS)
            val callbackClass = Class.forName(START_CALLBACK_CLASS)
            val resultCode = AtomicInteger(RootControlCodes.OPERATION_TIMEOUT)
            val completion = CountDownLatch(1)
            val requestBuilder = requestBuilderClass
                .getConstructor(Integer.TYPE)
                .newInstance(TETHERING_USB_TYPE)
            val request = requestBuilderClass.getMethod("build").invoke(requestBuilder)
            val callback = Proxy.newProxyInstance(
                callbackClass.classLoader,
                arrayOf(callbackClass)
            ) { proxy, method, arguments ->
                when (method.name) {
                    "onTetheringStarted" -> {
                        resultCode.set(RootControlCodes.SUCCESS)
                        completion.countDown()
                    }

                    "onTetheringFailed" -> {
                        val error = arguments?.firstOrNull() as? Int ?: 0
                        resultCode.set(RootControlCodes.FRAMEWORK_ERROR_OFFSET + error)
                        completion.countDown()
                    }

                    "equals" -> proxy === arguments?.firstOrNull()
                    "hashCode" -> System.identityHashCode(proxy)
                    "toString" -> "USBridgeStartTetheringCallback"
                    else -> null
                }
            }
            managerClass.getMethod(
                "startTethering",
                requestClass,
                Executor::class.java,
                callbackClass
            ).invoke(manager, request, DIRECT_EXECUTOR, callback)
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
            val manager = context.getSystemService(TETHERING_SERVICE_NAME)
                ?: return RootControlCodes.SERVICE_UNAVAILABLE
            val managerClass = Class.forName(TETHERING_MANAGER_CLASS)

            // Android 11-15 exposes stopTethering(int) as a system API. The RootService has
            // TETHER_PRIVILEGED authority, while reflection keeps the app buildable with the
            // public Android SDK where this overload is hidden.
            val method = managerClass.getMethod(
                "stopTethering",
                Integer.TYPE
            )
            method.invoke(manager, TETHERING_USB_TYPE)
            RootControlCodes.SUCCESS
        } catch (_: SecurityException) {
            RootControlCodes.PERMISSION_DENIED
        } catch (_: Throwable) {
            RootControlCodes.INTERNAL_ERROR
        }
    }

    private val DIRECT_EXECUTOR = Executor(Runnable::run)
    private const val TETHERING_SERVICE_NAME = "tethering"
    private const val TETHERING_MANAGER_CLASS = "android.net.TetheringManager"
    private const val TETHERING_REQUEST_CLASS =
        "android.net.TetheringManager\$TetheringRequest"
    private const val TETHERING_REQUEST_BUILDER_CLASS =
        "android.net.TetheringManager\$TetheringRequest\$Builder"
    private const val START_CALLBACK_CLASS =
        "android.net.TetheringManager\$StartTetheringCallback"
    private const val TETHERING_TIMEOUT_SECONDS = 20L
    private const val TETHERING_USB_TYPE = 1
}
