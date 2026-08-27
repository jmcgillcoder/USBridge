package com.usbridge.traffic

import android.content.ContentValues
import android.content.Context
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper
import com.usbridge.core.model.TrafficHistorySummary
import com.usbridge.core.model.TrafficSessionRecord
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class TrafficStore(context: Context) : SQLiteOpenHelper(
    context.applicationContext,
    DATABASE_NAME,
    null,
    DATABASE_VERSION
) {
    override fun onCreate(database: SQLiteDatabase) {
        database.execSQL(
            """
            CREATE TABLE traffic_sessions (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                started_at INTEGER NOT NULL,
                ended_at INTEGER NOT NULL,
                interface_name TEXT NOT NULL,
                upload_bytes INTEGER NOT NULL DEFAULT 0,
                download_bytes INTEGER NOT NULL DEFAULT 0,
                active INTEGER NOT NULL DEFAULT 1
            )
            """.trimIndent()
        )
        database.execSQL(
            """
            CREATE TABLE traffic_daily (
                day_key TEXT PRIMARY KEY,
                upload_bytes INTEGER NOT NULL DEFAULT 0,
                download_bytes INTEGER NOT NULL DEFAULT 0,
                duration_millis INTEGER NOT NULL DEFAULT 0
            )
            """.trimIndent()
        )
        database.execSQL(
            "CREATE INDEX index_traffic_sessions_started_at " +
                "ON traffic_sessions(started_at DESC)"
        )
    }

    override fun onUpgrade(database: SQLiteDatabase, oldVersion: Int, newVersion: Int) = Unit

    @Synchronized
    fun startSession(startedAtMillis: Long, interfaceName: String): Long {
        val database = writableDatabase
        database.beginTransaction()
        return try {
            database.execSQL(
                "UPDATE traffic_sessions SET active = 0 WHERE active = 1"
            )
            val values = ContentValues().apply {
                put("started_at", startedAtMillis)
                put("ended_at", startedAtMillis)
                put("interface_name", interfaceName)
                put("active", 1)
            }
            val id = database.insertOrThrow("traffic_sessions", null, values)
            database.setTransactionSuccessful()
            id
        } finally {
            database.endTransaction()
        }
    }

    @Synchronized
    fun recordDelta(
        sessionId: Long,
        timestampMillis: Long,
        uploadBytes: Long,
        downloadBytes: Long,
        durationMillis: Long
    ) {
        val safeUpload = uploadBytes.coerceAtLeast(0)
        val safeDownload = downloadBytes.coerceAtLeast(0)
        val safeDuration = durationMillis.coerceIn(0, MAX_RECORDED_INTERVAL_MILLIS)
        val dayKey = dayKey(timestampMillis)
        val database = writableDatabase
        database.beginTransaction()
        try {
            database.execSQL(
                """
                UPDATE traffic_sessions
                SET ended_at = ?,
                    upload_bytes = upload_bytes + ?,
                    download_bytes = download_bytes + ?
                WHERE id = ?
                """.trimIndent(),
                arrayOf(timestampMillis, safeUpload, safeDownload, sessionId)
            )
            val initialDay = ContentValues().apply {
                put("day_key", dayKey)
                put("upload_bytes", 0)
                put("download_bytes", 0)
                put("duration_millis", 0)
            }
            database.insertWithOnConflict(
                "traffic_daily",
                null,
                initialDay,
                SQLiteDatabase.CONFLICT_IGNORE
            )
            database.execSQL(
                """
                UPDATE traffic_daily
                SET upload_bytes = upload_bytes + ?,
                    download_bytes = download_bytes + ?,
                    duration_millis = duration_millis + ?
                WHERE day_key = ?
                """.trimIndent(),
                arrayOf<Any>(safeUpload, safeDownload, safeDuration, dayKey)
            )
            database.setTransactionSuccessful()
        } finally {
            database.endTransaction()
        }
    }

    @Synchronized
    fun finishSession(sessionId: Long, endedAtMillis: Long) {
        val values = ContentValues().apply {
            put("ended_at", endedAtMillis)
            put("active", 0)
        }
        writableDatabase.update(
            "traffic_sessions",
            values,
            "id = ?",
            arrayOf(sessionId.toString())
        )
    }

    @Synchronized
    fun readSummary(nowMillis: Long = System.currentTimeMillis()): TrafficHistorySummary {
        val database = readableDatabase
        val today = dayKey(nowMillis)
        val monthPrefix = monthKey(nowMillis)
        val todayValues = database.rawQuery(
            """
            SELECT upload_bytes, download_bytes, duration_millis
            FROM traffic_daily WHERE day_key = ?
            """.trimIndent(),
            arrayOf(today)
        ).use { cursor ->
            if (cursor.moveToFirst()) {
                Triple(cursor.getLong(0), cursor.getLong(1), cursor.getLong(2))
            } else {
                Triple(0L, 0L, 0L)
            }
        }
        val monthValues = database.rawQuery(
            """
            SELECT COALESCE(SUM(upload_bytes), 0), COALESCE(SUM(download_bytes), 0)
            FROM traffic_daily WHERE day_key LIKE ?
            """.trimIndent(),
            arrayOf("$monthPrefix%")
        ).use { cursor ->
            cursor.moveToFirst()
            cursor.getLong(0) to cursor.getLong(1)
        }
        val sessionCount = database.rawQuery(
            "SELECT COUNT(*) FROM traffic_sessions",
            null
        ).use { cursor ->
            cursor.moveToFirst()
            cursor.getInt(0)
        }
        val recentSessions = database.rawQuery(
            """
            SELECT id, started_at, ended_at, interface_name,
                   upload_bytes, download_bytes, active
            FROM traffic_sessions
            ORDER BY started_at DESC
            LIMIT ?
            """.trimIndent(),
            arrayOf(RECENT_SESSION_LIMIT.toString())
        ).use { cursor ->
            buildList {
                while (cursor.moveToNext()) {
                    add(
                        TrafficSessionRecord(
                            id = cursor.getLong(0),
                            startedAtMillis = cursor.getLong(1),
                            endedAtMillis = cursor.getLong(2),
                            interfaceName = cursor.getString(3),
                            uploadBytes = cursor.getLong(4),
                            downloadBytes = cursor.getLong(5),
                            isActive = cursor.getInt(6) == 1
                        )
                    )
                }
            }
        }
        return TrafficHistorySummary(
            todayUploadBytes = todayValues.first,
            todayDownloadBytes = todayValues.second,
            monthUploadBytes = monthValues.first,
            monthDownloadBytes = monthValues.second,
            todayDurationMillis = todayValues.third,
            sessionCount = sessionCount,
            recentSessions = recentSessions
        )
    }

    private fun dayKey(timestampMillis: Long): String =
        SimpleDateFormat("yyyy-MM-dd", Locale.US).format(Date(timestampMillis))

    private fun monthKey(timestampMillis: Long): String =
        SimpleDateFormat("yyyy-MM", Locale.US).format(Date(timestampMillis))

    private companion object {
        const val DATABASE_NAME = "usbridge_traffic.db"
        const val DATABASE_VERSION = 1
        const val RECENT_SESSION_LIMIT = 20
        const val MAX_RECORDED_INTERVAL_MILLIS = 60_000L
    }
}
