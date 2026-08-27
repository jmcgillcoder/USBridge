package com.usbridge.core.root

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import com.topjohnwu.superuser.ipc.RootService
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.withContext
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

data class RootControlResult(
    val success: Boolean,
    val code: Int,
    val message: String
)

class RootControlClient(context: Context) {
    private val applicationContext = context.applicationContext
    private val mainHandler = Handler(Looper.getMainLooper())

    suspend fun probe(): String = withRootService { service -> service.probe() }

    suspend fun startUsbTethering(): RootControlResult = runCatching {
        withRootService { service -> service.startUsbTethering() }
    }.fold(
        onSuccess = ::mapResult,
        onFailure = { RootControlResult(false, RootControlCodes.INTERNAL_ERROR, it.userMessage()) }
    )

    suspend fun stopUsbTethering(): RootControlResult = runCatching {
        withRootService { service -> service.stopUsbTethering() }
    }.fold(
        onSuccess = ::mapResult,
        onFailure = { RootControlResult(false, RootControlCodes.INTERNAL_ERROR, it.userMessage()) }
    )

    suspend fun setMobileDataEnabled(enabled: Boolean): RootControlResult = runCatching {
        withRootService { service -> service.setMobileDataEnabled(enabled) }
    }.fold(
        onSuccess = ::mapResult,
        onFailure = { RootControlResult(false, RootControlCodes.INTERNAL_ERROR, it.userMessage()) }
    )

    suspend fun setWifiEnabled(enabled: Boolean): RootControlResult = runCatching {
        withRootService { service -> service.setWifiEnabled(enabled) }
    }.fold(
        onSuccess = ::mapResult,
        onFailure = { RootControlResult(false, RootControlCodes.INTERNAL_ERROR, it.userMessage()) }
    )

    suspend fun reconnectMobileData(
        downTimeMillis: Int = DEFAULT_MOBILE_DATA_DOWN_TIME_MILLIS
    ): RootControlResult = runCatching {
        withRootService { service -> service.reconnectMobileData(downTimeMillis) }
    }.fold(
        onSuccess = ::mapResult,
        onFailure = { RootControlResult(false, RootControlCodes.INTERNAL_ERROR, it.userMessage()) }
    )

    private suspend fun <T> withRootService(
        operation: (IRootControlService) -> T
    ): T = withContext(Dispatchers.Main.immediate) {
        withTimeout(CLIENT_TIMEOUT_MILLIS) {
            suspendCancellableCoroutine { continuation ->
                val callbackExecutor = Executors.newSingleThreadExecutor()
                val finished = AtomicBoolean(false)
                lateinit var connection: ServiceConnection

                fun unbindOnMainThread() {
                    if (Looper.myLooper() == Looper.getMainLooper()) {
                        runCatching { RootService.unbind(connection) }
                    } else {
                        mainHandler.post { runCatching { RootService.unbind(connection) } }
                    }
                }

                fun finish(result: Result<T>) {
                    if (!finished.compareAndSet(false, true)) return
                    unbindOnMainThread()
                    callbackExecutor.shutdown()
                    result.fold(
                        onSuccess = continuation::resume,
                        onFailure = continuation::resumeWithException
                    )
                }

                connection = object : ServiceConnection {
                    override fun onServiceConnected(name: ComponentName, binder: IBinder) {
                        val service = IRootControlService.Stub.asInterface(binder)
                        finish(runCatching { operation(service) })
                    }

                    override fun onServiceDisconnected(name: ComponentName) {
                        finish(Result.failure(IllegalStateException("Root 服务已断开")))
                    }

                    override fun onNullBinding(name: ComponentName) {
                        finish(Result.failure(IllegalStateException("Root 服务没有返回控制接口")))
                    }
                }

                continuation.invokeOnCancellation {
                    if (finished.compareAndSet(false, true)) {
                        unbindOnMainThread()
                        callbackExecutor.shutdownNow()
                    }
                }

                runCatching {
                    RootService.bind(
                        Intent(applicationContext, RootControlService::class.java),
                        callbackExecutor,
                        connection
                    )
                }.onFailure { finish(Result.failure(it)) }
            }
        }
    }

    private fun mapResult(code: Int): RootControlResult = RootControlResult(
        success = code == RootControlCodes.SUCCESS,
        code = code,
        message = when {
            code == RootControlCodes.SUCCESS -> "操作成功"
            code == RootControlCodes.OPERATION_TIMEOUT -> "系统处理超时"
            code == RootControlCodes.SERVICE_UNAVAILABLE -> "系统共享服务不可用"
            code == RootControlCodes.PERMISSION_DENIED -> "Root 进程仍被系统拒绝"
            code == RootControlCodes.UNSUPPORTED_ANDROID_VERSION -> "当前 Android 版本暂不支持"
            code == RootControlCodes.MOBILE_DATA_DISABLE_FAILED -> "无法关闭移动数据"
            code == RootControlCodes.MOBILE_DATA_ENABLE_FAILED -> "移动数据未能自动恢复，请立即手动开启"
            code == RootControlCodes.WIFI_CONTROL_FAILED -> "无法切换 Wi-Fi，不能保证 USB 共享使用移动网络"
            code >= RootControlCodes.FRAMEWORK_ERROR_OFFSET ->
                "系统共享服务返回错误 ${code - RootControlCodes.FRAMEWORK_ERROR_OFFSET}"
            else -> "Root 控制服务执行失败"
        }
    )

    private fun Throwable.userMessage(): String = when (this) {
        is kotlinx.coroutines.TimeoutCancellationException -> "连接 Root 控制服务超时"
        else -> message ?: "Root 控制服务不可用"
    }

    private companion object {
        const val CLIENT_TIMEOUT_MILLIS = 30_000L
        const val DEFAULT_MOBILE_DATA_DOWN_TIME_MILLIS = 1_500
    }
}
