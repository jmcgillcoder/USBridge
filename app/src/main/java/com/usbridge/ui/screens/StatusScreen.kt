package com.usbridge.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ArrowDownward
import androidx.compose.material.icons.outlined.ArrowUpward
import androidx.compose.material.icons.outlined.Construction
import androidx.compose.material.icons.outlined.PowerSettingsNew
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.Security
import androidx.compose.material.icons.outlined.SwapHoriz
import androidx.compose.material.icons.outlined.Usb
import androidx.compose.material.icons.outlined.VerifiedUser
import androidx.compose.material.icons.outlined.WarningAmber
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.usbridge.core.model.IpMode
import com.usbridge.core.model.MobileReconnectStatus
import com.usbridge.core.model.RootState
import com.usbridge.core.model.UsbConnectionState
import com.usbridge.ui.MainUiState
import com.usbridge.ui.components.DetailRow
import com.usbridge.ui.components.IconTile
import com.usbridge.ui.components.NoticeCard
import com.usbridge.ui.components.ScreenList
import com.usbridge.ui.components.SectionTitle
import com.usbridge.ui.components.StatusPill
import com.usbridge.ui.components.formatBytes
import com.usbridge.service.UsbAutomationStatus

@Composable
fun StatusScreen(
    state: MainUiState,
    contentPadding: PaddingValues,
    onRefresh: () -> Unit,
    onRetryRoot: () -> Unit,
    onStartUsbTethering: () -> Unit,
    onStopUsbTethering: () -> Unit,
    onRefreshPublicIp: () -> Unit,
    onReconnectMobileNetwork: () -> Unit
) {
    ScreenList(scaffoldPadding = contentPadding) {
        if (state.isRefreshing) {
            item { LinearProgressIndicator(modifier = Modifier.fillMaxWidth()) }
        }

        state.errorMessage?.let { message ->
            item {
                NoticeCard(
                    icon = Icons.Outlined.WarningAmber,
                    title = "状态更新失败",
                    body = message
                )
            }
        }

        item {
            UsbStatusCard(state = state, onRefresh = onRefresh)
        }

        if (state.rootState != RootState.GRANTED) {
            item {
                RootRequiredCard(
                    rootState = state.rootState,
                    onRetryRoot = onRetryRoot
                )
            }
        }

        item {
            MetricRow(state = state)
        }

        item {
            SectionTitle("公网地址")
        }
        item {
            NetworkCard(state = state, onRefreshPublicIp = onRefreshPublicIp)
        }

        item {
            SectionTitle("快捷操作")
        }
        item {
            TetheringActionsCard(
                state = state,
                onStartUsbTethering = onStartUsbTethering,
                onStopUsbTethering = onStopUsbTethering,
                onReconnectMobileNetwork = onReconnectMobileNetwork
            )
        }
        if (state.mobileReconnect.status != MobileReconnectStatus.IDLE) {
            item { MobileReconnectCard(state) }
        }
    }
}

@Composable
private fun UsbStatusCard(state: MainUiState, onRefresh: () -> Unit) {
    val connectionLabel = when {
        state.device.usbConnectionState == UsbConnectionState.INTERFACE_READY -> "网络共享已开启"
        state.automationRuntime.status == UsbAutomationStatus.ENABLING -> "正在开启网络共享"
        state.automationRuntime.status == UsbAutomationStatus.ACTIVE -> "网络共享已开启"
        state.automationRuntime.status == UsbAutomationStatus.DISABLING -> "正在关闭网络共享"
        state.automationRuntime.status == UsbAutomationStatus.ERROR -> "网络共享失败"
        else -> when (state.device.usbConnectionState) {
            UsbConnectionState.DISCONNECTED -> "未连接电脑"
            UsbConnectionState.CONNECTED -> "已连接电脑"
            UsbConnectionState.INTERFACE_READY -> "网络共享已开启"
        }
    }
    val description = when (state.device.usbConnectionState) {
        UsbConnectionState.DISCONNECTED -> "请用数据线连接电脑"
        UsbConnectionState.CONNECTED -> "正在准备网络共享"
        UsbConnectionState.INTERFACE_READY -> "电脑可使用手机网络"
    }

    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.extraLarge,
        color = MaterialTheme.colorScheme.surfaceContainerHigh
    ) {
        Column(
            modifier = Modifier.padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(18.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                IconTile(
                    icon = Icons.Outlined.Usb,
                    contentDescription = null,
                    containerColor = MaterialTheme.colorScheme.primaryContainer,
                    iconColor = MaterialTheme.colorScheme.primary
                )
                StatusPill(
                    label = if (state.rootState == RootState.GRANTED) "已授权" else "需要授权",
                    positive = state.rootState == RootState.GRANTED
                )
            }
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    text = connectionLabel,
                    style = MaterialTheme.typography.headlineSmall,
                    fontWeight = FontWeight.SemiBold
                )
                Text(
                    text = description,
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            FilledTonalButton(
                onClick = onRefresh,
                enabled = !state.isRefreshing,
                modifier = Modifier.fillMaxWidth()
            ) {
                if (state.isRefreshing) {
                    CircularProgressIndicator(
                        modifier = Modifier.width(18.dp),
                        strokeWidth = 2.dp
                    )
                    Spacer(Modifier.width(10.dp))
                } else {
                    Icon(Icons.Outlined.Refresh, contentDescription = null)
                    Spacer(Modifier.width(10.dp))
                }
                Text(if (state.isRefreshing) "正在刷新" else "刷新状态")
            }
        }
    }
}

@Composable
private fun RootRequiredCard(rootState: RootState, onRetryRoot: () -> Unit) {
    val isChecking = rootState == RootState.CHECKING
    NoticeCard(
        icon = if (isChecking) Icons.Outlined.Security else Icons.Outlined.WarningAmber,
        title = if (isChecking) "正在检查权限" else "需要控制权限",
        body = if (isChecking) {
            "在 Root 管理器中选择允许"
        } else {
            "请在 Root 管理器中允许 USBridge"
        }
    )
    if (!isChecking) {
        Spacer(Modifier.height(10.dp))
        Button(onClick = onRetryRoot, modifier = Modifier.fillMaxWidth()) {
            Icon(Icons.Outlined.VerifiedUser, contentDescription = null)
            Spacer(Modifier.width(8.dp))
            Text("授权")
        }
    }
}

@Composable
private fun MetricRow(state: MainUiState) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        MetricCard(
            modifier = Modifier.weight(1f),
            icon = Icons.Outlined.ArrowUpward,
            label = "上传",
            value = formatBytes(state.automationRuntime.sessionUploadBytes)
        )
        MetricCard(
            modifier = Modifier.weight(1f),
            icon = Icons.Outlined.ArrowDownward,
            label = "下载",
            value = formatBytes(state.automationRuntime.sessionDownloadBytes)
        )
        MetricCard(
            modifier = Modifier.weight(1f),
            icon = Icons.Outlined.SwapHoriz,
            label = "网络模式",
            value = state.automation.ipMode.displayName
        )
    }
}

@Composable
private fun MetricCard(
    modifier: Modifier,
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    label: String,
    value: String
) {
    Surface(
        modifier = modifier,
        shape = MaterialTheme.shapes.medium,
        color = MaterialTheme.colorScheme.surfaceContainerLow
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 14.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Icon(
                imageVector = icon,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.primary
            )
            Text(
                text = label,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = value,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1
            )
        }
    }
}

@Composable
private fun NetworkCard(state: MainUiState, onRefreshPublicIp: () -> Unit) {
    val mobileConnected = state.device.cellularInterfaces.any { it.isUp }
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow
    ) {
        Column(modifier = Modifier.padding(horizontal = 18.dp, vertical = 6.dp)) {
            DetailRow("公网 IPv4", state.publicIp.ipv4 ?: "—")
            DetailRow("公网 IPv6", state.publicIp.ipv6 ?: "—")
            DetailRow("移动网络状态", if (mobileConnected) "已连接" else "未连接")
            DetailRow("上网方式", state.device.activeTransport, showDivider = false)
            state.publicIpError?.let {
                Text(
                    text = it,
                    modifier = Modifier.padding(vertical = 8.dp),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error
                )
            }
            OutlinedButton(
                onClick = onRefreshPublicIp,
                enabled = !state.isCheckingPublicIp && !state.mobileReconnect.isRunning,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(vertical = 10.dp)
            ) {
                if (state.isCheckingPublicIp) {
                    CircularProgressIndicator(
                        modifier = Modifier.width(18.dp),
                        strokeWidth = 2.dp
                    )
                } else {
                    Icon(Icons.Outlined.Refresh, contentDescription = null)
                }
                Spacer(Modifier.width(8.dp))
                Text(if (state.isCheckingPublicIp) "正在刷新" else "刷新地址")
            }
        }
    }
}

@Composable
private fun TetheringActionsCard(
    state: MainUiState,
    onStartUsbTethering: () -> Unit,
    onStopUsbTethering: () -> Unit,
    onReconnectMobileNetwork: () -> Unit
) {
    val runtimeStatus = state.automationRuntime.status
    val busy = runtimeStatus == UsbAutomationStatus.ENABLING ||
        runtimeStatus == UsbAutomationStatus.DISABLING
    val active = runtimeStatus == UsbAutomationStatus.ACTIVE
    val usbConnected = state.automationRuntime.usbConnected
    val canOperate = state.rootState == RootState.GRANTED && usbConnected && !busy
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow
    ) {
        Column(
            modifier = Modifier.padding(18.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(10.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Icon(
                    if (active) Icons.Outlined.VerifiedUser else Icons.Outlined.Construction,
                    contentDescription = null
                )
                Text(
                    text = if (active) "电脑正在使用手机网络" else runtimeStatus.userMessage,
                    style = MaterialTheme.typography.titleMedium
                )
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Button(
                    onClick = if (active) onStopUsbTethering else onStartUsbTethering,
                    enabled = canOperate,
                    modifier = Modifier.weight(1f)
                ) {
                    Icon(Icons.Outlined.PowerSettingsNew, contentDescription = null)
                    Spacer(Modifier.width(6.dp))
                    Text(
                        when {
                            busy -> "处理中"
                            active -> "关闭共享"
                            else -> "开启共享"
                        }
                    )
                }
                OutlinedButton(
                    onClick = onReconnectMobileNetwork,
                    enabled = state.rootState == RootState.GRANTED &&
                        !state.mobileReconnect.isRunning,
                    modifier = Modifier.weight(1f)
                ) {
                    Icon(Icons.Outlined.SwapHoriz, contentDescription = null)
                    Spacer(Modifier.width(6.dp))
                    Text(if (state.mobileReconnect.isRunning) "重连中" else "重新联网")
                }
            }
        }
    }
}

@Composable
private fun MobileReconnectCard(state: MainUiState) {
    val reconnect = state.mobileReconnect
    val positive = reconnect.status == MobileReconnectStatus.IP_CHANGED
    val detailLines = buildList {
        reconnect.before?.ipv4?.let { old ->
            reconnect.after?.ipv4?.let { new -> add("IPv4：$old → $new") }
        }
        reconnect.before?.ipv6?.let { old ->
            reconnect.after?.ipv6?.let { new -> add("IPv6：$old → $new") }
        }
    }
    val title = when (reconnect.status) {
        MobileReconnectStatus.IP_CHANGED -> "公网 IP 已更换"
        MobileReconnectStatus.IP_UNCHANGED -> "公网 IP 未变化"
        MobileReconnectStatus.ERROR -> "重新联网失败"
        MobileReconnectStatus.COMPLETED_WITHOUT_IP -> "重新联网成功"
        else -> "正在重新联网"
    }
    NoticeCard(
        icon = when {
            reconnect.status == MobileReconnectStatus.ERROR -> Icons.Outlined.WarningAmber
            positive -> Icons.Outlined.VerifiedUser
            else -> Icons.Outlined.SwapHoriz
        },
        title = title,
        body = buildString {
            append(reconnect.status.userDescription)
            if (detailLines.isNotEmpty()) {
                append('\n')
                append(detailLines.joinToString(separator = "\n"))
            }
        },
        positive = positive
    )
}

private val IpMode.displayName: String
    get() = when (this) {
        IpMode.AUTO -> "自动"
        IpMode.IPV4 -> "IPv4"
        IpMode.IPV6 -> "IPv6"
    }

private val UsbAutomationStatus.userMessage: String
    get() = when (this) {
        UsbAutomationStatus.STOPPED -> "自动共享已关闭"
        UsbAutomationStatus.MONITORING -> "等待电脑连接"
        UsbAutomationStatus.USB_CONNECTED -> "已连接电脑"
        UsbAutomationStatus.ENABLING -> "正在开启网络共享"
        UsbAutomationStatus.ACTIVE -> "电脑正在使用手机网络"
        UsbAutomationStatus.DISABLING -> "正在关闭网络共享"
        UsbAutomationStatus.ERROR -> "网络共享失败"
    }

private val MobileReconnectStatus.userDescription: String
    get() = when (this) {
        MobileReconnectStatus.IDLE -> "尚未重新联网"
        MobileReconnectStatus.CHECKING_BEFORE -> "正在获取当前 IP"
        MobileReconnectStatus.RECONNECTING -> "正在重新联网"
        MobileReconnectStatus.WAITING_FOR_NETWORK -> "等待移动网络恢复"
        MobileReconnectStatus.VERIFYING -> "正在检查新 IP"
        MobileReconnectStatus.IP_CHANGED -> "重新联网成功"
        MobileReconnectStatus.IP_UNCHANGED -> "已重新联网"
        MobileReconnectStatus.COMPLETED_WITHOUT_IP -> "未获取到公网 IP"
        MobileReconnectStatus.ERROR -> "检查移动网络后重试"
    }
