package com.usbridge.core.update

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.core.content.FileProvider
import com.usbridge.BuildConfig
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest

const val REPOSITORY_URL = "https://github.com/jmcgillcoder/USBridge"
private const val LATEST_RELEASE_API = "https://api.github.com/repos/jmcgillcoder/USBridge/releases/latest"
private const val MAX_APK_SIZE = 200L * 1024L * 1024L

data class AppUpdateUiState(
    val isChecking: Boolean = false,
    val isDownloading: Boolean = false,
    val progress: Int = 0,
    val available: Boolean = false,
    val latestVersion: String? = null,
    val releaseNotes: String? = null,
    val message: String? = null
)

data class UpdateRelease(
    val version: String,
    val notes: String,
    val releaseUrl: String,
    val assetName: String,
    val assetUrl: String,
    val assetSize: Long,
    val checksumsUrl: String
)

enum class InstallLaunchResult {
    STARTED,
    PERMISSION_REQUIRED
}

class AppUpdater(context: Context) {
    private val appContext = context.applicationContext

    fun checkLatest(): UpdateRelease {
        val release = JSONObject(downloadText(LATEST_RELEASE_API, 2L * 1024L * 1024L))
        check(!release.optBoolean("draft") && !release.optBoolean("prerelease")) {
            "最新版本尚未公开发布"
        }
        val version = release.getString("tag_name").trim().removePrefix("v")
        check(parseVersion(version) != null) { "无法识别版本号 $version" }
        val assets = release.getJSONArray("assets")
        var apkName = ""
        var apkUrl = ""
        var apkSize = 0L
        var checksumsUrl = ""
        for (index in 0 until assets.length()) {
            val asset = assets.getJSONObject(index)
            val name = asset.getString("name")
            val lower = name.lowercase()
            if (lower == "sha256sums.txt") {
                checksumsUrl = asset.getString("browser_download_url")
            }
            if (lower.startsWith("usbridge-android-") && lower.endsWith(".apk")) {
                if (apkName.isBlank() || lower.endsWith("-signed.apk")) {
                    apkName = name
                    apkUrl = asset.getString("browser_download_url")
                    apkSize = asset.optLong("size")
                }
            }
        }
        if (compareVersions(version, BuildConfig.VERSION_NAME) > 0) {
            check(apkUrl.isNotBlank() && checksumsUrl.isNotBlank()) {
                "该版本缺少 Android 安装文件或校验文件"
            }
        }
        return UpdateRelease(
            version = version,
            notes = release.optString("body").trim(),
            releaseUrl = release.optString("html_url"),
            assetName = apkName,
            assetUrl = apkUrl,
            assetSize = apkSize,
            checksumsUrl = checksumsUrl
        )
    }

    fun isNewer(release: UpdateRelease): Boolean =
        compareVersions(release.version, BuildConfig.VERSION_NAME) > 0

    fun download(release: UpdateRelease, onProgress: (Int) -> Unit): File {
        check(release.assetUrl.isNotBlank() && release.checksumsUrl.isNotBlank()) {
            "没有可安装的新版本"
        }
        check(release.assetSize in 1..MAX_APK_SIZE) { "更新文件大小异常" }
        val expected = checksumFor(
            downloadText(release.checksumsUrl, 2L * 1024L * 1024L),
            release.assetName
        ) ?: error("校验文件中没有找到 Android 安装文件")
        val directory = File(appContext.cacheDir, "updates").apply { mkdirs() }
        directory.listFiles()?.forEach(File::delete)
        val partial = File(directory, "${release.assetName}.part")
        val destination = File(directory, release.assetName)
        val connection = open(release.assetUrl)
        try {
            check(connection.responseCode == HttpURLConnection.HTTP_OK) {
                "下载更新失败：HTTP ${connection.responseCode}"
            }
            val length = connection.contentLengthLong.takeIf { it > 0 } ?: release.assetSize
            check(length <= MAX_APK_SIZE) { "更新文件大小异常" }
            connection.inputStream.use { input ->
                partial.outputStream().use { output ->
                    val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
                    var total = 0L
                    while (true) {
                        val count = input.read(buffer)
                        if (count < 0) break
                        total += count
                        check(total <= MAX_APK_SIZE) { "更新文件大小异常" }
                        output.write(buffer, 0, count)
                        onProgress(((total * 100L) / length.coerceAtLeast(1L)).toInt().coerceIn(0, 100))
                    }
                }
            }
        } finally {
            connection.disconnect()
        }
        check(partial.length() == release.assetSize) { "更新文件大小不匹配" }
        check(sha256(partial).equals(expected, ignoreCase = true)) {
            partial.delete()
            "更新文件校验失败，请稍后重试"
        }
        check(partial.renameTo(destination)) { "保存更新文件失败" }
        return destination
    }

    fun launchInstaller(apk: File): InstallLaunchResult {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O &&
            !appContext.packageManager.canRequestPackageInstalls()
        ) {
            appContext.startActivity(
                Intent(
                    Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                    Uri.parse("package:${appContext.packageName}")
                ).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            )
            return InstallLaunchResult.PERMISSION_REQUIRED
        }
        val uri = FileProvider.getUriForFile(
            appContext,
            "${appContext.packageName}.fileprovider",
            apk
        )
        appContext.startActivity(
            Intent(Intent.ACTION_VIEW)
                .setDataAndType(uri, "application/vnd.android.package-archive")
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_GRANT_READ_URI_PERMISSION)
        )
        return InstallLaunchResult.STARTED
    }

    fun openRepository() {
        appContext.startActivity(
            Intent(Intent.ACTION_VIEW, Uri.parse(REPOSITORY_URL))
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        )
    }

    private fun downloadText(url: String, limit: Long): String {
        val connection = open(url)
        try {
            check(connection.responseCode == HttpURLConnection.HTTP_OK) {
                "GitHub 返回了 HTTP ${connection.responseCode}"
            }
            return connection.inputStream.bufferedReader().use { reader ->
                val result = StringBuilder()
                val buffer = CharArray(DEFAULT_BUFFER_SIZE)
                while (true) {
                    val count = reader.read(buffer)
                    if (count < 0) break
                    result.append(buffer, 0, count)
                    check(result.length <= limit) { "版本信息大小异常" }
                }
                result.toString()
            }
        } finally {
            connection.disconnect()
        }
    }

    private fun open(url: String): HttpURLConnection =
        (URL(url).openConnection() as HttpURLConnection).apply {
            connectTimeout = 15_000
            readTimeout = 45_000
            instanceFollowRedirects = true
            setRequestProperty("Accept", "application/vnd.github+json")
            setRequestProperty("User-Agent", "USBridge/${BuildConfig.VERSION_NAME}")
        }
}

internal fun parseVersion(value: String): List<Int>? {
    val match = Regex("^v?(\\d+)\\.(\\d+)\\.(\\d+)", RegexOption.IGNORE_CASE)
        .find(value.trim()) ?: return null
    return match.groupValues.drop(1).map(String::toInt)
}

internal fun compareVersions(left: String, right: String): Int {
    val first = parseVersion(left) ?: return 0
    val second = parseVersion(right) ?: return 0
    return first.zip(second).firstOrNull { (a, b) -> a != b }
        ?.let { (a, b) -> a.compareTo(b) } ?: 0
}

internal fun checksumFor(contents: String, assetName: String): String? =
    contents.lineSequence().mapNotNull { line ->
        val fields = line.trim().split(Regex("\\s+"))
        if (fields.size < 2) return@mapNotNull null
        val name = fields.last().removePrefix("*").substringAfterLast('/')
        val hash = fields.first()
        hash.takeIf {
            name == assetName && hash.length == 64 && hash.all { character -> character.isDigit() || character.lowercaseChar() in 'a'..'f' }
        }
    }.firstOrNull()

private fun sha256(file: File): String {
    val digest = MessageDigest.getInstance("SHA-256")
    file.inputStream().use { input ->
        val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
        while (true) {
            val count = input.read(buffer)
            if (count < 0) break
            digest.update(buffer, 0, count)
        }
    }
    return digest.digest().joinToString("") { "%02x".format(it) }
}
