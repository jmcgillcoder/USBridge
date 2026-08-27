package com.usbridge

import android.Manifest
import android.content.pm.PackageManager
import android.os.Bundle
import android.os.Build
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.usbridge.ui.USBridgeApp
import com.usbridge.ui.MainViewModel
import com.usbridge.ui.theme.USBridgeTheme
import com.usbridge.service.UsbAutomationStatus
import com.usbridge.core.model.RootState

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            USBridgeTheme {
                val viewModel: MainViewModel = viewModel()
                val state by viewModel.uiState.collectAsStateWithLifecycle()
                val context = LocalContext.current
                val notificationPermissionLauncher = rememberLauncherForActivityResult(
                    ActivityResultContracts.RequestPermission()
                ) { }
                var notificationPermissionRequested by rememberSaveable { mutableStateOf(false) }
                LaunchedEffect(
                    state.automation.autoTetherOnUsb,
                    state.automationRuntime.status,
                    state.rootState
                ) {
                    val serviceNeeded = state.automation.autoTetherOnUsb ||
                        state.automationRuntime.status != UsbAutomationStatus.STOPPED
                    if (!notificationPermissionRequested &&
                        state.rootState == RootState.GRANTED &&
                        Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                        serviceNeeded &&
                        ContextCompat.checkSelfPermission(
                            context,
                            Manifest.permission.POST_NOTIFICATIONS
                        ) != PackageManager.PERMISSION_GRANTED
                    ) {
                        notificationPermissionRequested = true
                        notificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
                    }
                }
                USBridgeApp(
                    state = state,
                    onRefresh = { viewModel.refresh() },
                    onRetryRoot = viewModel::retryRootAuthorization,
                    onAutoTetherChanged = viewModel::setAutoTether,
                    onStopOnDisconnectChanged = viewModel::setStopOnDisconnect,
                    onStartOnBootChanged = viewModel::setStartOnBoot,
                    onRetryOnFailureChanged = viewModel::setRetryOnFailure,
                    onIpModeChanged = viewModel::setIpMode,
                    onStartUsbTethering = viewModel::startUsbTethering,
                    onStopUsbTethering = viewModel::stopUsbTethering,
                    onRefreshPublicIp = viewModel::refreshPublicIp,
                    onReconnectMobileNetwork = viewModel::reconnectMobileNetwork
                )
            }
        }
    }
}
