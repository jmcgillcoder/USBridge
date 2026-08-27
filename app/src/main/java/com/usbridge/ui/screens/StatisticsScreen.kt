package com.usbridge.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ArrowDownward
import androidx.compose.material.icons.outlined.ArrowUpward
import androidx.compose.material.icons.outlined.History
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.usbridge.core.model.TrafficSessionRecord
import com.usbridge.ui.MainUiState
import com.usbridge.ui.components.DetailRow
import com.usbridge.ui.components.ScreenList
import com.usbridge.ui.components.SectionTitle
import com.usbridge.ui.components.formatBytes
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

@Composable
fun StatisticsScreen(
    state: MainUiState,
    contentPadding: PaddingValues
) {
    val runtime = state.automationRuntime
    val summary = state.trafficSummary

    ScreenList(scaffoldPadding = contentPadding) {
        item {
            SectionTitle(
                title = "实时速度",
                subtitle = if (runtime.trafficInterfaceName != null) {
                    "本次共享"
                } else {
                    "暂无实时数据"
                }
            )
        }
        item {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                TrafficMetricCard(
                    modifier = Modifier.weight(1f),
                    icon = Icons.Outlined.ArrowUpward,
                    label = "实时上传",
                    value = "${formatBytes(runtime.uploadBytesPerSecond)}/s"
                )
                TrafficMetricCard(
                    modifier = Modifier.weight(1f),
                    icon = Icons.Outlined.ArrowDownward,
                    label = "实时下载",
                    value = "${formatBytes(runtime.downloadBytesPerSecond)}/s"
                )
            }
        }

        if (runtime.trafficInterfaceName != null) {
            item {
                Surface(
                    modifier = Modifier.fillMaxWidth(),
                    shape = MaterialTheme.shapes.large,
                    color = MaterialTheme.colorScheme.surfaceContainerLow
                ) {
                    Column(modifier = Modifier.padding(horizontal = 18.dp, vertical = 6.dp)) {
                        DetailRow("本次上传", formatBytes(runtime.sessionUploadBytes))
                        DetailRow("本次下载", formatBytes(runtime.sessionDownloadBytes))
                        DetailRow(
                            "共享时长",
                            runtime.trafficSessionStartedAtMillis
                                ?.let { formatDuration(System.currentTimeMillis() - it) }
                                ?: "—",
                            showDivider = false
                        )
                    }
                }
            }
        }

        item {
            SectionTitle("使用总量")
        }
        item {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                SummaryTotalCard(
                    modifier = Modifier.weight(1f),
                    label = "今日总量",
                    value = formatBytes(summary.todayUploadBytes + summary.todayDownloadBytes),
                    emphasized = true
                )
                SummaryTotalCard(
                    modifier = Modifier.weight(1f),
                    label = "本月总量",
                    value = formatBytes(summary.monthUploadBytes + summary.monthDownloadBytes)
                )
            }
        }
        item {
            Surface(
                modifier = Modifier.fillMaxWidth(),
                shape = MaterialTheme.shapes.large,
                color = MaterialTheme.colorScheme.surfaceContainerLow
            ) {
                Column(modifier = Modifier.padding(horizontal = 18.dp, vertical = 6.dp)) {
                    DetailRow("今日上传", formatBytes(summary.todayUploadBytes))
                    DetailRow("今日下载", formatBytes(summary.todayDownloadBytes))
                    DetailRow("今日时长", formatDuration(summary.todayDurationMillis))
                    DetailRow("本月上传", formatBytes(summary.monthUploadBytes))
                    DetailRow("本月下载", formatBytes(summary.monthDownloadBytes))
                    DetailRow("共享次数", summary.sessionCount.toString(), showDivider = false)
                }
            }
        }

        item {
            SectionTitle("使用记录")
        }
        if (summary.recentSessions.isEmpty()) {
            item { EmptyHistoryCard() }
        } else {
            items(summary.recentSessions, key = TrafficSessionRecord::id) { session ->
                TrafficSessionCard(session)
            }
        }
    }
}

@Composable
private fun TrafficMetricCard(
    modifier: Modifier,
    icon: ImageVector,
    label: String,
    value: String
) {
    Surface(
        modifier = modifier,
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerHigh
    ) {
        Column(
            modifier = Modifier.padding(18.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Icon(
                imageVector = icon,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.secondary
            )
            Text(
                text = label,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = value,
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold
            )
        }
    }
}

@Composable
private fun SummaryTotalCard(
    modifier: Modifier,
    label: String,
    value: String,
    emphasized: Boolean = false
) {
    Surface(
        modifier = modifier,
        shape = MaterialTheme.shapes.large,
        color = if (emphasized) {
            MaterialTheme.colorScheme.primaryContainer
        } else {
            MaterialTheme.colorScheme.surfaceContainerHigh
        }
    ) {
        Column(
            modifier = Modifier.padding(18.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = label,
                style = MaterialTheme.typography.labelLarge,
                color = if (emphasized) {
                    MaterialTheme.colorScheme.onPrimaryContainer
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                }
            )
            Text(
                text = value,
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold
            )
        }
    }
}

@Composable
private fun TrafficSessionCard(session: TrafficSessionRecord) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.medium,
        color = MaterialTheme.colorScheme.surfaceContainerLow
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text(
                        text = formatDateTime(session.startedAtMillis),
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.SemiBold
                    )
                    Text(
                        text = "网络共享",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
                Text(
                    text = if (session.isActive) "进行中" else formatDuration(
                        (session.endedAtMillis ?: session.startedAtMillis) - session.startedAtMillis
                    ),
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.primary
                )
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(18.dp)
            ) {
                Text(
                    text = "↑ ${formatBytes(session.uploadBytes)}",
                    style = MaterialTheme.typography.bodyMedium
                )
                Text(
                    text = "↓ ${formatBytes(session.downloadBytes)}",
                    style = MaterialTheme.typography.bodyMedium
                )
                Text(
                    text = "共 ${formatBytes(session.uploadBytes + session.downloadBytes)}",
                    style = MaterialTheme.typography.bodyMedium,
                    fontWeight = FontWeight.Medium
                )
            }
        }
    }
}

@Composable
private fun EmptyHistoryCard() {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(28.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Icon(
                imageVector = Icons.Outlined.History,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(text = "暂无共享记录", style = MaterialTheme.typography.titleMedium)
        }
    }
}

private fun formatDateTime(timestampMillis: Long): String =
    SimpleDateFormat("MM-dd HH:mm", Locale.getDefault()).format(Date(timestampMillis))

private fun formatDuration(durationMillis: Long): String {
    val totalSeconds = durationMillis.coerceAtLeast(0) / 1_000
    val hours = totalSeconds / 3_600
    val minutes = totalSeconds % 3_600 / 60
    val seconds = totalSeconds % 60
    return when {
        hours > 0 -> "${hours}时 ${minutes}分"
        minutes > 0 -> "${minutes}分 ${seconds}秒"
        else -> "${seconds}秒"
    }
}
