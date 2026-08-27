package com.usbridge.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import com.usbridge.MainActivity
import com.usbridge.R
import com.usbridge.control.PhoneControlRuntime
import com.usbridge.control.PhoneControlServer
import com.usbridge.core.preferences.AppPreferences
import com.usbridge.core.root.RootControlClient
import com.usbridge.traffic.UsbTrafficMonitor
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

class UsbAutomationService : Service() {
    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private lateinit var preferences: AppPreferences
    private lateinit var rootControlClient: RootControlClient
    private lateinit var phoneControlRuntime: PhoneControlRuntime
    private var phoneControlServer: PhoneControlServer? = null
    private var operationJob: Job? = null
    private var autoStartJob: Job? = null
    private var disconnectJob: Job? = null
    private var trafficJob: Job? = null
    private var upstreamGuardJob: Job? = null
    private var receiverRegistered = false

    private val usbStateReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            if (intent.action == ACTION_USB_STATE) handleUsbState(intent)
        }
    }

    override fun onCreate() {
        super.onCreate()
        preferences = AppPreferences(this)
        rootControlClient = RootControlClient(this)
        phoneControlRuntime = PhoneControlRuntime.get(this)
        createNotificationChannel()
        startForeground(NOTIFICATION_ID, buildNotification())
        phoneControlServer = PhoneControlServer(this, phoneControlRuntime).also { server ->
            runCatching { server.start() }
                .onFailure {
                    UsbAutomationRuntime.update(
                        status = UsbAutomationStatus.ERROR,
                        message = "自动共享暂时不可用"
                    )
                }
        }
        serviceScope.launch {
            phoneControlRuntime.initialize()
            delay(CELLULAR_UPSTREAM_DEBOUNCE_MILLIS)
            phoneControlRuntime.ensureCellularUpstream()
        }
        registerUsbReceiver()
        trafficJob = serviceScope.launch(Dispatchers.IO) {
            UsbTrafficMonitor(this@UsbAutomationService).monitor()
        }
        upstreamGuardJob = serviceScope.launch {
            while (isActive) {
                delay(UPSTREAM_GUARD_INTERVAL_MILLIS)
                if (phoneControlRuntime.activeUpstreamTransport() != "cellular") {
                    phoneControlRuntime.ensureCellularUpstream()
                }
            }
        }
        updateRuntime(
            status = UsbAutomationStatus.MONITORING,
            message = "等待连接电脑"
        )
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START_TETHERING -> startTethering(manual = true)
            ACTION_STOP_TETHERING -> stopTethering(manual = true)
            ACTION_STOP_MONITORING -> stopSelf()
            else -> updateRuntime(
                status = if (UsbAutomationRuntime.state.value.usbConnected) {
                    UsbAutomationStatus.USB_CONNECTED
                } else {
                    UsbAutomationStatus.MONITORING
                },
                message = if (UsbAutomationRuntime.state.value.usbConnected) {
                    "已连接电脑，等待开启网络共享"
                } else {
                    "等待连接电脑"
                }
            )
        }
        return START_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        operationJob?.cancel()
        autoStartJob?.cancel()
        disconnectJob?.cancel()
        trafficJob?.cancel()
        upstreamGuardJob?.cancel()
        phoneControlServer?.stop()
        phoneControlServer = null
        if (receiverRegistered) unregisterReceiver(usbStateReceiver)
        serviceScope.cancel()
        UsbAutomationRuntime.update(
            status = UsbAutomationStatus.STOPPED,
            message = "自动共享当前已暂停"
        )
        super.onDestroy()
    }

    private fun registerUsbReceiver() {
        val stickyIntent = ContextCompat.registerReceiver(
            this,
            usbStateReceiver,
            IntentFilter(ACTION_USB_STATE),
            ContextCompat.RECEIVER_NOT_EXPORTED
        )
        receiverRegistered = true
        stickyIntent?.let(::handleUsbState)
    }

    private fun handleUsbState(intent: Intent) {
        val connected = intent.getBooleanExtra(EXTRA_CONNECTED, false)
        val configured = intent.getBooleanExtra(EXTRA_CONFIGURED, false)
        UsbAutomationRuntime.update(
            status = when {
                UsbAutomationRuntime.state.value.status in BUSY_OR_ACTIVE_STATES ->
                    UsbAutomationRuntime.state.value.status
                connected -> UsbAutomationStatus.USB_CONNECTED
                else -> UsbAutomationStatus.MONITORING
            },
            usbConnected = connected,
            usbConfigured = configured,
            message = if (connected) "已连接电脑" else "电脑已断开"
        )
        notifyStateChanged()

        if (connected) {
            disconnectJob?.cancel()
            serviceScope.launch {
                delay(CELLULAR_UPSTREAM_DEBOUNCE_MILLIS)
                phoneControlRuntime.ensureCellularUpstream()
            }
            if (UsbAutomationPolicy.shouldAutoStart(
                    connected = true,
                    autoTetherEnabled = preferences.readAutomationSettings().autoTetherOnUsb,
                    currentStatus = UsbAutomationRuntime.state.value.status
                )
            ) {
                autoStartJob?.cancel()
                autoStartJob = serviceScope.launch {
                    delay(AUTO_START_DEBOUNCE_MILLIS)
                    startTethering(manual = false)
                }
            }
        } else {
            serviceScope.launch { phoneControlRuntime.restoreWifiIfNeeded() }
            if (!preferences.readAutomationSettings().stopOnDisconnect) return
            disconnectJob?.cancel()
            disconnectJob = serviceScope.launch {
                delay(DISCONNECT_GRACE_MILLIS)
                if (UsbAutomationPolicy.shouldStopAfterDisconnect(
                        connected = UsbAutomationRuntime.state.value.usbConnected,
                        stopOnDisconnect = true,
                        currentStatus = UsbAutomationRuntime.state.value.status
                    )
                ) {
                    stopTethering(manual = false)
                }
            }
        }
    }

    private fun startTethering(manual: Boolean) {
        if (operationJob?.isActive == true) return
        operationJob = serviceScope.launch {
            updateRuntime(UsbAutomationStatus.ENABLING, "正在开启 USB 网络共享")
            val settings = preferences.readAutomationSettings()
            val attempts = if (!manual && settings.retryOnFailure) MAX_AUTO_ATTEMPTS else 1
            var lastMessage = "开启失败"

            repeat(attempts) { attempt ->
                val result = rootControlClient.startUsbTethering()
                if (result.success) {
                    delay(CELLULAR_UPSTREAM_DEBOUNCE_MILLIS)
                    val upstream = phoneControlRuntime.ensureCellularUpstream()
                    if (upstream.ok) {
                        updateRuntime(UsbAutomationStatus.ACTIVE, "USB 网络共享已开启")
                        return@launch
                    }
                    lastMessage = upstream.message
                } else {
                    lastMessage = result.message
                }
                if (attempt < attempts - 1) {
                    updateRuntime(
                        UsbAutomationStatus.ENABLING,
                        "$lastMessage，准备重试 ${attempt + 2}/$attempts"
                    )
                    delay(RETRY_DELAY_MILLIS)
                }
            }
            updateRuntime(UsbAutomationStatus.ERROR, lastMessage)
        }
    }

    private fun stopTethering(manual: Boolean) {
        if (operationJob?.isActive == true) return
        operationJob = serviceScope.launch {
            updateRuntime(UsbAutomationStatus.DISABLING, "正在关闭 USB 网络共享")
            val result = rootControlClient.stopUsbTethering()
            if (result.success) {
                phoneControlRuntime.restoreWifiIfNeeded()
                updateRuntime(
                    status = if (UsbAutomationRuntime.state.value.usbConnected) {
                        UsbAutomationStatus.USB_CONNECTED
                    } else {
                        UsbAutomationStatus.MONITORING
                    },
                    message = if (manual) "网络共享已关闭" else "电脑已断开，网络共享已关闭"
                )
            } else {
                updateRuntime(UsbAutomationStatus.ERROR, result.message)
            }
        }
    }

    private fun updateRuntime(status: UsbAutomationStatus, message: String) {
        UsbAutomationRuntime.update(status = status, message = message)
        notifyStateChanged()
    }

    private fun notifyStateChanged() {
        getSystemService(NotificationManager::class.java)
            .notify(NOTIFICATION_ID, buildNotification())
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val channel = NotificationChannel(
            NOTIFICATION_CHANNEL_ID,
            "USB 自动共享",
            NotificationManager.IMPORTANCE_LOW
        ).apply {
            description = "电脑连接与网络共享"
            setShowBadge(false)
        }
        getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
    }

    private fun buildNotification(): Notification {
        val state = UsbAutomationRuntime.state.value
        val openAppIntent = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val actionIntent = if (state.status == UsbAutomationStatus.ACTIVE) {
            Intent(this, UsbAutomationService::class.java).setAction(ACTION_STOP_TETHERING)
        } else {
            Intent(this, UsbAutomationService::class.java).setAction(ACTION_STOP_MONITORING)
        }
        val actionPendingIntent = PendingIntent.getService(
            this,
            1,
            actionIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        return NotificationCompat.Builder(this, NOTIFICATION_CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_usbridge_notification)
            .setContentTitle("USBridge")
            .setContentText(state.status.notificationMessage)
            .setContentIntent(openAppIntent)
            .setOngoing(state.status != UsbAutomationStatus.ERROR)
            .setOnlyAlertOnce(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .addAction(
                0,
                if (state.status == UsbAutomationStatus.ACTIVE) "关闭共享" else "暂停自动共享",
                actionPendingIntent
            )
            .build()
    }

    private val UsbAutomationStatus.notificationMessage: String
        get() = when (this) {
            UsbAutomationStatus.STOPPED -> "自动共享已关闭"
            UsbAutomationStatus.MONITORING -> "等待电脑连接"
            UsbAutomationStatus.USB_CONNECTED -> "正在准备网络共享"
            UsbAutomationStatus.ENABLING -> "正在开启网络共享"
            UsbAutomationStatus.ACTIVE -> "网络共享已开启"
            UsbAutomationStatus.DISABLING -> "正在关闭网络共享"
            UsbAutomationStatus.ERROR -> "网络共享失败"
        }

    companion object {
        private const val ACTION_USB_STATE = "android.hardware.usb.action.USB_STATE"
        private const val EXTRA_CONNECTED = "connected"
        private const val EXTRA_CONFIGURED = "configured"
        private const val ACTION_START_MONITORING = "com.usbridge.action.START_MONITORING"
        private const val ACTION_STOP_MONITORING = "com.usbridge.action.STOP_MONITORING"
        private const val ACTION_START_TETHERING = "com.usbridge.action.START_TETHERING"
        private const val ACTION_STOP_TETHERING = "com.usbridge.action.STOP_TETHERING"
        private const val NOTIFICATION_CHANNEL_ID = "usb_automation"
        private const val NOTIFICATION_ID = 1001
        private const val AUTO_START_DEBOUNCE_MILLIS = 1_500L
        private const val CELLULAR_UPSTREAM_DEBOUNCE_MILLIS = 1_500L
        private const val UPSTREAM_GUARD_INTERVAL_MILLIS = 5_000L
        private const val DISCONNECT_GRACE_MILLIS = 3_000L
        private const val RETRY_DELAY_MILLIS = 5_000L
        private const val MAX_AUTO_ATTEMPTS = 3

        private val BUSY_OR_ACTIVE_STATES = setOf(
            UsbAutomationStatus.ENABLING,
            UsbAutomationStatus.ACTIVE,
            UsbAutomationStatus.DISABLING
        )

        fun startMonitoring(context: Context) {
            startService(context, ACTION_START_MONITORING)
        }

        fun stopMonitoring(context: Context) {
            context.stopService(Intent(context, UsbAutomationService::class.java))
        }

        fun requestStartTethering(context: Context) {
            startService(context, ACTION_START_TETHERING)
        }

        fun requestStopTethering(context: Context) {
            startService(context, ACTION_STOP_TETHERING)
        }

        private fun startService(context: Context, action: String) {
            ContextCompat.startForegroundService(
                context,
                Intent(context, UsbAutomationService::class.java).setAction(action)
            )
        }
    }
}
