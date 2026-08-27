package com.usbridge.ui.screens

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Construction
import androidx.compose.material.icons.outlined.ExpandLess
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.VerifiedUser
import androidx.compose.material.icons.outlined.WarningAmber
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.usbridge.core.model.RootState
import com.usbridge.core.model.UsbConnectionState
import com.usbridge.ui.MainUiState
import com.usbridge.ui.components.DetailRow
import com.usbridge.ui.components.IconTile
import com.usbridge.ui.components.ScreenList
import com.usbridge.ui.components.SectionTitle
import com.usbridge.ui.components.StatusPill
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

@Composable
fun SettingsScreen(
    state: MainUiState,
    contentPadding: PaddingValues,
    onRefresh: () -> Unit,
    onRetryRoot: () -> Unit
) {
    var diagnosticsExpanded by remember { mutableStateOf(false) }
    ScreenList(scaffoldPadding = contentPadding) {
        item {
            RootStatusCard(state = state, onRetryRoot = onRetryRoot)
        }

        item {
            SectionTitle("应用信息")
        }
        item {
            AppInformationCard(state)
        }

        item {
            DiagnosticsCard(
                state = state,
                expanded = diagnosticsExpanded,
                onExpandedChange = { diagnosticsExpanded = !diagnosticsExpanded },
                onRefresh = onRefresh
            )
        }

    }
}

@Composable
private fun RootStatusCard(state: MainUiState, onRetryRoot: () -> Unit) {
    val granted = state.rootState == RootState.GRANTED
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.extraLarge,
        color = MaterialTheme.colorScheme.surfaceContainerHigh
    ) {
        Column(
            modifier = Modifier.padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                IconTile(
                    icon = if (granted) Icons.Outlined.VerifiedUser else Icons.Outlined.WarningAmber,
                    contentDescription = null,
                    containerColor = if (granted) {
                        MaterialTheme.colorScheme.primaryContainer
                    } else {
                        MaterialTheme.colorScheme.errorContainer
                    },
                    iconColor = if (granted) {
                        MaterialTheme.colorScheme.primary
                    } else {
                        MaterialTheme.colorScheme.error
                    }
                )
                StatusPill(
                    label = state.rootState.label,
                    positive = granted
                )
            }
            Text(
                text = if (granted) "控制权限已开启" else "需要控制权限",
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold
            )
            Text(
                text = if (granted) {
                    "网络共享和重新联网可用"
                } else {
                    "请在 Root 管理器中允许 USBridge"
                },
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            if (!granted && state.rootState != RootState.CHECKING) {
                Button(onClick = onRetryRoot, modifier = Modifier.fillMaxWidth()) {
                    Text("授权")
                }
            }
        }
    }
}

@Composable
private fun AppInformationCard(state: MainUiState) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow
    ) {
        Column(modifier = Modifier.padding(horizontal = 18.dp, vertical = 6.dp)) {
            DetailRow("手机型号", "${state.device.manufacturer} ${state.device.model}".trim())
            DetailRow(
                "系统版本",
                state.device.androidVersion.takeIf(String::isNotBlank)?.let { "Android $it" } ?: "—"
            )
            DetailRow("应用版本", state.device.appVersion.ifBlank { "—" }, showDivider = false)
        }
    }
}

@Composable
private fun DiagnosticsCard(
    state: MainUiState,
    expanded: Boolean,
    onExpandedChange: () -> Unit,
    onRefresh: () -> Unit
) {
    val lastUpdated = state.lastUpdatedAtMillis?.let(::formatTime) ?: "暂无"
    val controlStatus = when {
        state.rootState != RootState.GRANTED -> "等待授权"
        state.rootServiceProbe != null -> "运行正常"
        state.rootServiceError != null -> "暂时不可用"
        else -> "正在检查"
    }
    val usbStatus = when (state.device.usbConnectionState) {
        UsbConnectionState.DISCONNECTED -> "未连接电脑"
        UsbConnectionState.CONNECTED -> "已连接电脑"
        UsbConnectionState.INTERFACE_READY -> "网络共享已就绪"
    }
    val mobileStatus = if (state.device.cellularInterfaces.any { it.isUp }) {
        "已连接"
    } else {
        "未连接"
    }
    val systemProtection = when (state.device.selinuxMode?.lowercase()) {
        "enforcing" -> "已开启"
        "permissive" -> "兼容模式"
        "disabled" -> "未开启"
        else -> "未读取"
    }

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
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Row(
                    modifier = Modifier.weight(1f),
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    IconTile(
                        icon = Icons.Outlined.Construction,
                        contentDescription = null,
                        containerColor = MaterialTheme.colorScheme.surfaceContainerHighest
                    )
                    Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
                        Text(
                            text = "故障排查",
                            style = MaterialTheme.typography.titleMedium,
                            fontWeight = FontWeight.SemiBold
                        )
                        Text(
                            text = "连接有问题时查看",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                }
                TextButton(onClick = onExpandedChange) {
                    Text(if (expanded) "收起" else "展开")
                    Spacer(Modifier.width(4.dp))
                    Icon(
                        imageVector = if (expanded) Icons.Outlined.ExpandLess else Icons.Outlined.ExpandMore,
                        contentDescription = null
                    )
                }
            }
            AnimatedVisibility(visible = expanded) {
                Column {
                    DetailRow("控制功能", controlStatus)
                    DetailRow("USB 连接", usbStatus)
                    DetailRow("移动网络", mobileStatus)
                    DetailRow("系统保护", systemProtection)
                    DetailRow("更新时间", lastUpdated, showDivider = false)
                    OutlinedButton(
                        onClick = onRefresh,
                        enabled = !state.isRefreshing,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Icon(Icons.Outlined.Refresh, contentDescription = null)
                        Spacer(Modifier.width(8.dp))
                        Text(if (state.isRefreshing) "正在刷新" else "重新检查")
                    }
                }
            }
        }
    }
}

private val RootState.label: String
    get() = when (this) {
        RootState.CHECKING -> "正在检查"
        RootState.GRANTED -> "已授权"
        RootState.DENIED -> "未授权"
        RootState.ERROR -> "检查失败"
    }

private fun formatTime(timestamp: Long): String =
    SimpleDateFormat("HH:mm:ss", Locale.getDefault()).format(Date(timestamp))
