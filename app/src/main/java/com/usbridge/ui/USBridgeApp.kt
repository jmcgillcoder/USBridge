package com.usbridge.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AutoMode
import androidx.compose.material.icons.outlined.BarChart
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material.icons.outlined.Usb
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.usbridge.core.model.IpMode
import com.usbridge.ui.screens.AutomationScreen
import com.usbridge.ui.screens.SettingsScreen
import com.usbridge.ui.screens.StatisticsScreen
import com.usbridge.ui.screens.StatusScreen
import com.usbridge.ui.theme.USBridgeTheme

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun USBridgeApp(
    state: MainUiState,
    onRefresh: () -> Unit,
    onRetryRoot: () -> Unit,
    onAutoTetherChanged: (Boolean) -> Unit,
    onStopOnDisconnectChanged: (Boolean) -> Unit,
    onStartOnBootChanged: (Boolean) -> Unit,
    onRetryOnFailureChanged: (Boolean) -> Unit,
    onIpModeChanged: (IpMode) -> Unit,
    onStartUsbTethering: () -> Unit,
    onStopUsbTethering: () -> Unit,
    onRefreshPublicIp: () -> Unit,
    onReconnectMobileNetwork: () -> Unit,
    onCheckForUpdates: () -> Unit,
    onInstallUpdate: () -> Unit,
    onOpenProjectPage: () -> Unit
) {
    var currentRoute by rememberSaveable { mutableStateOf(AppDestination.Status.route) }
    val currentDestination = destinations.first { it.route == currentRoute }

    Scaffold(
        modifier = Modifier.fillMaxSize(),
        containerColor = androidx.compose.material3.MaterialTheme.colorScheme.surface,
        topBar = {
            TopAppBar(
                title = {
                    Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
                        Text(
                            text = currentDestination.title,
                            style = androidx.compose.material3.MaterialTheme.typography.titleLarge
                        )
                        Text(
                            text = currentDestination.subtitle,
                            style = androidx.compose.material3.MaterialTheme.typography.bodySmall,
                            color = androidx.compose.material3.MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = androidx.compose.material3.MaterialTheme.colorScheme.surface,
                    scrolledContainerColor = androidx.compose.material3.MaterialTheme.colorScheme.surfaceContainer
                )
            )
        },
        bottomBar = {
            NavigationBar(
                containerColor = androidx.compose.material3.MaterialTheme.colorScheme.surfaceContainer,
                tonalElevation = 0.dp
            ) {
                destinations.forEach { destination ->
                    NavigationBarItem(
                        selected = destination.route == currentRoute,
                        onClick = { currentRoute = destination.route },
                        icon = {
                            Icon(
                                imageVector = destination.icon,
                                contentDescription = destination.label
                            )
                        },
                        label = { Text(destination.label) },
                        colors = NavigationBarItemDefaults.colors(
                            selectedIconColor = androidx.compose.material3.MaterialTheme.colorScheme.onSecondaryContainer,
                            selectedTextColor = androidx.compose.material3.MaterialTheme.colorScheme.onSurface,
                            indicatorColor = androidx.compose.material3.MaterialTheme.colorScheme.secondaryContainer,
                            unselectedIconColor = androidx.compose.material3.MaterialTheme.colorScheme.onSurfaceVariant,
                            unselectedTextColor = androidx.compose.material3.MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    )
                }
            }
        }
    ) { innerPadding ->
        AnimatedContent(
            targetState = currentRoute,
            modifier = Modifier.fillMaxSize(),
            transitionSpec = {
                (fadeIn(animationSpec = tween(160)) +
                    slideInVertically(
                        animationSpec = spring(
                            dampingRatio = 0.86f,
                            stiffness = Spring.StiffnessMediumLow
                        ),
                        initialOffsetY = { it / 18 }
                    )).togetherWith(fadeOut(animationSpec = tween(90)))
            },
            label = "destination transition"
        ) { route ->
            when (route) {
                AppDestination.Status.route -> StatusScreen(
                    state = state,
                    contentPadding = innerPadding,
                    onRefresh = onRefresh,
                    onRetryRoot = onRetryRoot,
                    onStartUsbTethering = onStartUsbTethering,
                    onStopUsbTethering = onStopUsbTethering,
                    onRefreshPublicIp = onRefreshPublicIp,
                    onReconnectMobileNetwork = onReconnectMobileNetwork
                )

                AppDestination.Automation.route -> AutomationScreen(
                    state = state,
                    contentPadding = innerPadding,
                    onAutoTetherChanged = onAutoTetherChanged,
                    onStopOnDisconnectChanged = onStopOnDisconnectChanged,
                    onStartOnBootChanged = onStartOnBootChanged,
                    onRetryOnFailureChanged = onRetryOnFailureChanged,
                    onIpModeChanged = onIpModeChanged
                )

                AppDestination.Statistics.route -> StatisticsScreen(
                    state = state,
                    contentPadding = innerPadding
                )

                AppDestination.Settings.route -> SettingsScreen(
                    state = state,
                    contentPadding = innerPadding,
                    onRefresh = onRefresh,
                    onRetryRoot = onRetryRoot,
                    onCheckForUpdates = onCheckForUpdates,
                    onInstallUpdate = onInstallUpdate,
                    onOpenProjectPage = onOpenProjectPage
                )
            }
        }
    }
}

@Immutable
private data class Destination(
    val route: String,
    val label: String,
    val title: String,
    val subtitle: String,
    val icon: ImageVector
)

private sealed class AppDestination(val route: String) {
    data object Status : AppDestination("status")
    data object Automation : AppDestination("automation")
    data object Statistics : AppDestination("statistics")
    data object Settings : AppDestination("settings")
}

private val destinations = listOf(
    Destination(
        route = AppDestination.Status.route,
        label = "首页",
        title = "USBridge",
        subtitle = "手机网络共享",
        icon = Icons.Outlined.Usb
    ),
    Destination(
        route = AppDestination.Automation.route,
        label = "自动",
        title = "自动共享",
        subtitle = "连接电脑后自动共享",
        icon = Icons.Outlined.AutoMode
    ),
    Destination(
        route = AppDestination.Statistics.route,
        label = "统计",
        title = "流量统计",
        subtitle = "电脑使用的流量",
        icon = Icons.Outlined.BarChart
    ),
    Destination(
        route = AppDestination.Settings.route,
        label = "设置",
        title = "设置",
        subtitle = "授权和版本",
        icon = Icons.Outlined.Settings
    )
)

@Preview(showBackground = true)
@Composable
private fun USBridgeAppPreview() {
    USBridgeTheme {
        USBridgeApp(
            state = MainUiState(isRefreshing = false),
            onRefresh = {},
            onRetryRoot = {},
            onAutoTetherChanged = {},
            onStopOnDisconnectChanged = {},
            onStartOnBootChanged = {},
            onRetryOnFailureChanged = {},
            onIpModeChanged = {},
            onStartUsbTethering = {},
            onStopUsbTethering = {},
            onRefreshPublicIp = {},
            onReconnectMobileNetwork = {},
            onCheckForUpdates = {},
            onInstallUpdate = {},
            onOpenProjectPage = {}
        )
    }
}
