package com.usbridge.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.usbridge.core.model.IpMode
import com.usbridge.core.model.RootState
import com.usbridge.core.model.UsbConnectionState
import com.usbridge.ui.MainUiState
import com.usbridge.ui.components.NoticeCard
import com.usbridge.ui.components.PreferenceSwitchRow
import com.usbridge.ui.components.ScreenList
import com.usbridge.ui.components.SectionTitle
import com.usbridge.service.UsbAutomationStatus

@Composable
fun AutomationScreen(
    state: MainUiState,
    contentPadding: PaddingValues,
    onAutoTetherChanged: (Boolean) -> Unit,
    onStopOnDisconnectChanged: (Boolean) -> Unit,
    onStartOnBootChanged: (Boolean) -> Unit,
    onRetryOnFailureChanged: (Boolean) -> Unit,
    onIpModeChanged: (IpMode) -> Unit
) {
    val sharingReady = state.device.usbConnectionState == UsbConnectionState.INTERFACE_READY ||
        state.automationRuntime.status == UsbAutomationStatus.ACTIVE ||
        state.tetheringPath.tetheringEnabled
    ScreenList(scaffoldPadding = contentPadding) {
        item {
            NoticeCard(
                icon = Icons.Outlined.Info,
                title = if (state.rootState == RootState.GRANTED) {
                    if (sharingReady) "网络共享已开启" else state.automationRuntime.status.userTitle
                } else {
                    "需要控制权限"
                },
                body = if (state.rootState == RootState.GRANTED) {
                    if (sharingReady) "电脑正在使用手机网络" else state.automationRuntime.status.userMessage
                } else {
                    "请先在设置中授权"
                },
                positive = state.rootState == RootState.GRANTED &&
                    (sharingReady || state.automationRuntime.status != UsbAutomationStatus.ERROR)
            )
        }

        item {
            SectionTitle("网络模式")
        }
        item {
            IpModeCard(
                selectedMode = state.automation.ipMode,
                ipv4Available = state.tetheringPath.ipv4Available,
                ipv6Available = state.tetheringPath.ipv6Available,
                sharingActive = sharingReady,
                onModeSelected = onIpModeChanged
            )
        }

        item {
            SectionTitle("自动共享")
        }
        item {
            Surface(
                modifier = Modifier.fillMaxWidth(),
                shape = MaterialTheme.shapes.large,
                color = MaterialTheme.colorScheme.surfaceContainerLow
            ) {
                Column(modifier = Modifier.padding(horizontal = 18.dp, vertical = 4.dp)) {
                    PreferenceSwitchRow(
                        title = "连接电脑后自动共享",
                        description = "插上数据线后自动开启网络共享",
                        checked = state.automation.autoTetherOnUsb,
                        onCheckedChange = onAutoTetherChanged
                    )
                    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                    PreferenceSwitchRow(
                        title = "断开后关闭共享",
                        description = "拔掉数据线后自动关闭网络共享",
                        checked = state.automation.stopOnDisconnect,
                        onCheckedChange = onStopOnDisconnectChanged
                    )
                    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                    PreferenceSwitchRow(
                        title = "开机后自动运行",
                        description = "重启手机后继续运行",
                        checked = state.automation.startOnBoot,
                        onCheckedChange = onStartOnBootChanged
                    )
                    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                    PreferenceSwitchRow(
                        title = "失败后自动重试",
                        description = "开启失败时再次尝试",
                        checked = state.automation.retryOnFailure,
                        onCheckedChange = onRetryOnFailureChanged
                    )
                }
            }
        }

    }
}

@Composable
private fun IpModeCard(
    selectedMode: IpMode,
    ipv4Available: Boolean,
    ipv6Available: Boolean,
    sharingActive: Boolean,
    onModeSelected: (IpMode) -> Unit
) {
    val networkLabel = if (sharingActive) "USB 共享出口" else "当前移动网络"
    val compatibilityMessage = when {
        selectedMode == IpMode.IPV6 && !ipv6Available && ipv4Available ->
            "$networkLabel 当前仅提供 IPv4，请选择自动或 IPv4"

        else -> null
    }
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow
    ) {
        Column(
            modifier = Modifier.padding(18.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                IpMode.entries.forEach { mode ->
                    FilterChip(
                        selected = mode == selectedMode,
                        onClick = { onModeSelected(mode) },
                        label = { Text(mode.displayName) },
                        modifier = Modifier.weight(1f)
                    )
                }
            }
            Text(
                text = selectedMode.description,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            compatibilityMessage?.let { message ->
                Text(
                    text = message,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error
                )
            }
        }
    }
}

private val IpMode.displayName: String
    get() = when (this) {
        IpMode.AUTO -> "自动"
        IpMode.IPV4 -> "IPv4"
        IpMode.IPV6 -> "IPv6"
    }

private val IpMode.description: String
    get() = when (this) {
        IpMode.AUTO -> "自动选择网站支持的协议"
        IpMode.IPV4 -> "仅连接 IPv4 地址，必要时自动转换"
        IpMode.IPV6 -> "仅连接 IPv6 地址"
    }

private val UsbAutomationStatus.userMessage: String
    get() = when (this) {
        UsbAutomationStatus.STOPPED -> "连接电脑后不会自动共享"
        UsbAutomationStatus.MONITORING -> "连接电脑后自动开启网络共享"
        UsbAutomationStatus.USB_CONNECTED -> "正在准备网络共享"
        UsbAutomationStatus.ENABLING -> "已连接电脑"
        UsbAutomationStatus.ACTIVE -> "电脑正在使用手机网络"
        UsbAutomationStatus.DISABLING -> "正在处理"
        UsbAutomationStatus.ERROR -> "检查连接后重试"
    }

private val UsbAutomationStatus.userTitle: String
    get() = when (this) {
        UsbAutomationStatus.STOPPED -> "自动共享已关闭"
        UsbAutomationStatus.MONITORING -> "等待电脑连接"
        UsbAutomationStatus.USB_CONNECTED -> "已连接电脑"
        UsbAutomationStatus.ENABLING -> "正在开启网络共享"
        UsbAutomationStatus.ACTIVE -> "网络共享已开启"
        UsbAutomationStatus.DISABLING -> "正在关闭网络共享"
        UsbAutomationStatus.ERROR -> "自动共享失败"
    }
