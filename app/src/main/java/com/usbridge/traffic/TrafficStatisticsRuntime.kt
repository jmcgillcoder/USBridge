package com.usbridge.traffic

import com.usbridge.core.model.TrafficHistorySummary
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

object TrafficStatisticsRuntime {
    private val _summary = MutableStateFlow(TrafficHistorySummary())
    val summary: StateFlow<TrafficHistorySummary> = _summary.asStateFlow()

    fun publish(summary: TrafficHistorySummary) {
        _summary.value = summary
    }
}
