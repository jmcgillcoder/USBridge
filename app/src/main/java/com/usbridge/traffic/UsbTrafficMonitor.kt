package com.usbridge.traffic

import android.content.Context
import com.usbridge.service.UsbAutomationRuntime
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.withContext

class UsbTrafficMonitor(context: Context) {
    private val trafficReader = UsbTrafficReader()
    private val store = TrafficStore(context)

    suspend fun monitor() = withContext(Dispatchers.IO) {
        var session: ActiveSession? = null
        var missingSamples = 0
        TrafficStatisticsRuntime.publish(store.readSummary())

        try {
            while (currentCoroutineContext().isActive) {
                val sample = trafficReader.readSample()
                if (sample == null) {
                    missingSamples++
                    session?.let { active ->
                        UsbAutomationRuntime.updateTraffic(
                            interfaceName = active.interfaceName,
                            sessionStartedAtMillis = active.startedAtMillis,
                            sessionUploadBytes = active.totalUploadBytes,
                            sessionDownloadBytes = active.totalDownloadBytes,
                            uploadBytesPerSecond = 0,
                            downloadBytesPerSecond = 0
                        )
                        if (missingSamples >= MISSING_SAMPLE_GRACE_COUNT) {
                            finishSession(active, System.currentTimeMillis())
                            session = null
                            clearRuntimeTraffic()
                        }
                    }
                } else {
                    missingSamples = 0
                    if (session == null || session?.interfaceName != sample.interfaceName) {
                        session?.let { finishSession(it, sample.timestampMillis) }
                        session = ActiveSession(
                            id = store.startSession(sample.timestampMillis, sample.interfaceName),
                            interfaceName = sample.interfaceName,
                            startedAtMillis = sample.timestampMillis,
                            previousSample = sample,
                            lastFlushAtMillis = sample.timestampMillis
                        )
                    } else {
                        val active = checkNotNull(session)
                        val delta = TrafficDeltaCalculator.calculate(active.previousSample, sample)
                        active.previousSample = sample
                        active.totalUploadBytes += delta.uploadBytes
                        active.totalDownloadBytes += delta.downloadBytes
                        active.pendingUploadBytes += delta.uploadBytes
                        active.pendingDownloadBytes += delta.downloadBytes
                        active.pendingDurationMillis += delta.elapsedMillis

                        UsbAutomationRuntime.updateTraffic(
                            interfaceName = active.interfaceName,
                            sessionStartedAtMillis = active.startedAtMillis,
                            sessionUploadBytes = active.totalUploadBytes,
                            sessionDownloadBytes = active.totalDownloadBytes,
                            uploadBytesPerSecond = delta.uploadBytesPerSecond,
                            downloadBytesPerSecond = delta.downloadBytesPerSecond
                        )
                        if (sample.timestampMillis - active.lastFlushAtMillis >=
                            DATABASE_FLUSH_INTERVAL_MILLIS
                        ) {
                            flushSession(active, sample.timestampMillis)
                        }
                    }
                }
                delay(
                    if (UsbAutomationRuntime.state.value.usbConnected) {
                        SAMPLE_INTERVAL_MILLIS
                    } else {
                        IDLE_SAMPLE_INTERVAL_MILLIS
                    }
                )
            }
        } finally {
            withContext(NonCancellable + Dispatchers.IO) {
                session?.let { finishSession(it, System.currentTimeMillis()) }
                store.close()
            }
        }
    }

    private fun flushSession(session: ActiveSession, timestampMillis: Long) {
        store.recordDelta(
            sessionId = session.id,
            timestampMillis = timestampMillis,
            uploadBytes = session.pendingUploadBytes,
            downloadBytes = session.pendingDownloadBytes,
            durationMillis = session.pendingDurationMillis
        )
        session.pendingUploadBytes = 0
        session.pendingDownloadBytes = 0
        session.pendingDurationMillis = 0
        session.lastFlushAtMillis = timestampMillis
        TrafficStatisticsRuntime.publish(store.readSummary(timestampMillis))
    }

    private fun finishSession(session: ActiveSession, timestampMillis: Long) {
        if (session.pendingDurationMillis > 0 ||
            session.pendingUploadBytes > 0 ||
            session.pendingDownloadBytes > 0
        ) {
            flushSession(session, timestampMillis)
        }
        store.finishSession(session.id, timestampMillis)
        TrafficStatisticsRuntime.publish(store.readSummary(timestampMillis))
    }

    private fun clearRuntimeTraffic() {
        UsbAutomationRuntime.updateTraffic(
            interfaceName = null,
            sessionStartedAtMillis = null,
            sessionUploadBytes = 0,
            sessionDownloadBytes = 0,
            uploadBytesPerSecond = 0,
            downloadBytesPerSecond = 0
        )
    }

    private data class ActiveSession(
        val id: Long,
        val interfaceName: String,
        val startedAtMillis: Long,
        var previousSample: TrafficCounterSample,
        var lastFlushAtMillis: Long,
        var totalUploadBytes: Long = 0,
        var totalDownloadBytes: Long = 0,
        var pendingUploadBytes: Long = 0,
        var pendingDownloadBytes: Long = 0,
        var pendingDurationMillis: Long = 0
    )

    private companion object {
        const val SAMPLE_INTERVAL_MILLIS = 1_000L
        const val IDLE_SAMPLE_INTERVAL_MILLIS = 5_000L
        const val DATABASE_FLUSH_INTERVAL_MILLIS = 5_000L
        const val MISSING_SAMPLE_GRACE_COUNT = 3
    }
}
