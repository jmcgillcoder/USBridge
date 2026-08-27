package com.usbridge.control

import android.content.Context
import android.net.InetAddresses
import android.util.Log
import com.usbridge.BuildConfig
import com.usbridge.core.model.InterfaceKind
import com.usbridge.core.model.IpMode
import com.usbridge.core.model.PublicIpSnapshot
import com.usbridge.core.model.RootState
import com.usbridge.core.root.RootNetworkInterfaces
import com.usbridge.service.UsbAutomationRuntime
import com.usbridge.traffic.TrafficStatisticsRuntime
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import org.json.JSONArray
import org.json.JSONObject
import java.io.BufferedInputStream
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.io.InputStream
import java.io.OutputStream
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.net.Socket
import java.net.SocketException
import java.nio.charset.StandardCharsets
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

class PhoneControlServer(
    context: Context,
    private val runtime: PhoneControlRuntime = PhoneControlRuntime.get(context)
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    @Volatile
    private var serverSocket: ServerSocket? = null
    private var acceptJob: Job? = null

    fun start() {
        if (serverSocket != null) return
        val socket = ServerSocket().apply {
            reuseAddress = true
            bind(InetSocketAddress(PORT), BACKLOG)
        }
        serverSocket = socket
        acceptJob = scope.launch {
            while (isActive) {
                val client = try {
                    socket.accept()
                } catch (error: SocketException) {
                    if (isActive && !socket.isClosed) {
                        Log.e(TAG, "控制服务接收连接失败", error)
                    }
                    break
                }
                launch {
                    try {
                        handleClient(client)
                    } catch (error: IOException) {
                        Log.w(TAG, "控制接口客户端提前断开：${error.message}")
                    } catch (error: Throwable) {
                        Log.e(TAG, "控制接口连接处理失败", error)
                    }
                }
            }
        }
        Log.i(TAG, "手机控制服务已启动：0.0.0.0:$PORT（仅接受 USB 接口连接）")
    }

    fun stop() {
        runCatching { serverSocket?.close() }
        serverSocket = null
        acceptJob?.cancel()
        acceptJob = null
    }

    private suspend fun handleClient(socket: Socket) {
        socket.use { client ->
            client.soTimeout = SOCKET_READ_TIMEOUT_MILLIS
            if (!isUsbLocalAddress(client.localAddress)) {
                writeJson(
                    client.getOutputStream(),
                    HTTP_FORBIDDEN,
                    JSONObject()
                        .put("ok", false)
                        .put("message", "USBridge 控制接口只允许通过 USB 共享网卡访问")
                )
                return
            }

            val request = try {
                readRequest(BufferedInputStream(client.getInputStream()))
            } catch (error: HttpRequestException) {
                writeJson(
                    client.getOutputStream(),
                    error.status,
                    JSONObject().put("ok", false).put("message", error.message)
                )
                return
            }

            val response = runCatching { route(request) }.getOrElse { error ->
                Log.e(TAG, "控制接口执行失败：${request.method} ${request.path}", error)
                HttpResponse(
                    HTTP_INTERNAL_ERROR,
                    JSONObject()
                        .put("ok", false)
                        .put("message", error.message ?: "手机控制接口执行失败")
                )
            }
            writeJson(client.getOutputStream(), response.status, response.body)
        }
    }

    private suspend fun route(request: HttpRequest): HttpResponse = when {
        request.method in MUTATING_METHODS && !request.hasJsonContentType() ->
            HttpResponse(
                HTTP_UNSUPPORTED_MEDIA_TYPE,
                JSONObject().put("ok", false).put("message", "请求必须使用 JSON")
            )

        request.method in MUTATING_METHODS && !isDesktopControlRequest(request.headers) ->
            HttpResponse(
                HTTP_FORBIDDEN,
                JSONObject().put("ok", false).put("message", "请求来源不受信任")
            )

        request.method == "GET" && request.path == "/v1/status" ->
            HttpResponse(HTTP_OK, statusJson())

        request.method == "GET" && request.path == "/v1/traffic" ->
            HttpResponse(HTTP_OK, trafficJson())

        request.method == "POST" && request.path == "/v1/mobile/reconnect" ->
            HttpResponse(HTTP_OK, operationJson(runtime.reconnectMobileNetwork()))

        request.method == "POST" && request.path == "/v1/public-ip/refresh" -> {
            val result = runtime.refreshPublicIp()
            val hasAddress = result.snapshot.ipv4 != null || result.snapshot.ipv6 != null
            HttpResponse(
                HTTP_OK,
                operationJson(
                    PhoneOperationResult(
                        ok = hasAddress,
                        message = when {
                            !hasAddress -> result.errorMessage ?: "公网 IP 查询失败"
                            result.snapshot.ipv4 == null -> "已获取 IPv6 公网地址"
                            result.snapshot.ipv6 == null -> "已获取 IPv4 公网地址"
                            else -> "公网 IP 已刷新"
                        },
                        after = result.snapshot
                    )
                )
            )
        }

        request.method == "POST" && request.path == "/v1/upstream/cellular" ->
            HttpResponse(HTTP_OK, operationJson(runtime.ensureCellularUpstream()))

        request.method == "POST" && request.path == "/v1/tether/start" ->
            HttpResponse(HTTP_OK, operationJson(runtime.setUsbTetheringEnabled(true)))

        request.method == "POST" && request.path == "/v1/tether/stop" ->
            HttpResponse(HTTP_OK, operationJson(runtime.setUsbTetheringEnabled(false)))

        request.method == "PUT" && request.path == "/v1/ip-mode" -> {
            val modeName = runCatching { JSONObject(request.body).optString("mode") }
                .getOrDefault("")
            val mode = when (modeName.lowercase()) {
                "auto" -> IpMode.AUTO
                "ipv4" -> IpMode.IPV4
                "ipv6" -> IpMode.IPV6
                else -> null
            }
            if (mode == null) {
                HttpResponse(
                    HTTP_BAD_REQUEST,
                    JSONObject().put("ok", false).put("message", "mode 必须是 auto、ipv4 或 ipv6")
                )
            } else {
                HttpResponse(HTTP_OK, operationJson(runtime.setIpMode(mode)))
            }
        }

        else -> HttpResponse(
            HTTP_NOT_FOUND,
            JSONObject().put("ok", false).put("message", "接口不存在")
        )
    }

    private suspend fun statusJson(): JSONObject {
        val runtimeState = runtime.state.value
        val tetheringPath = runtime.refreshTetheringPath()
        val automation = UsbAutomationRuntime.state.value
        val upstream = tetheringPath.upstreamTransport
        return JSONObject()
            .put("version", BuildConfig.VERSION_NAME)
            .put(
                "root",
                JSONObject()
                    .put("granted", runtimeState.rootState == RootState.GRANTED)
                    .apply {
                        runtimeState.rootImplementation?.let { put("implementation", it) }
                    }
            )
            .put(
                "usb",
                JSONObject()
                    .put(
                        "connected",
                        automation.usbConnected || tetheringPath.usbInterfaceNames.isNotEmpty()
                    )
                    .put("tetheringEnabled", tetheringPath.tetheringEnabled)
                    .put("upstream", upstream)
                    .put("cellularUpstream", upstream == "cellular")
                    .put("interfaces", JSONArray(tetheringPath.usbInterfaceNames))
            )
            .put(
                "mobile",
                JSONObject()
                    .put(
                        "connected",
                        upstream == "cellular" &&
                            (tetheringPath.ipv4Available || tetheringPath.ipv6Available)
                    )
                    .put("ipv4Available", tetheringPath.ipv4Available)
                    .put("ipv6Available", tetheringPath.ipv6Available)
                    .put("interfaces", JSONArray(tetheringPath.mobileInterfaceNames))
            )
            .put("ipMode", runtime.ipMode().name.lowercase())
            .put("publicIp", publicIpJson(runtimeState.publicIp))
            .put("traffic", trafficJson())
            .put("observedAt", observedAt())
    }

    private fun trafficJson(): JSONObject {
        val runtimeTraffic = UsbAutomationRuntime.state.value
        val summary = TrafficStatisticsRuntime.summary.value
        return JSONObject()
            .apply { runtimeTraffic.trafficInterfaceName?.let { put("interfaceName", it) } }
            .put("uploadBytesPerSecond", runtimeTraffic.uploadBytesPerSecond)
            .put("downloadBytesPerSecond", runtimeTraffic.downloadBytesPerSecond)
            .put("sessionUploadBytes", runtimeTraffic.sessionUploadBytes)
            .put("sessionDownloadBytes", runtimeTraffic.sessionDownloadBytes)
            .put("todayUploadBytes", summary.todayUploadBytes)
            .put("todayDownloadBytes", summary.todayDownloadBytes)
            .put("monthUploadBytes", summary.monthUploadBytes)
            .put("monthDownloadBytes", summary.monthDownloadBytes)
    }

    private fun operationJson(result: PhoneOperationResult): JSONObject = JSONObject()
        .put("ok", result.ok)
        .put("message", result.message)
        .apply {
            result.before?.let { put("before", publicIpJson(it)) }
            result.after?.let { put("after", publicIpJson(it)) }
            result.commandSucceeded?.let { put("commandSucceeded", it) }
            result.networkDisconnected?.let { put("networkDisconnected", it) }
            result.networkRecovered?.let { put("networkRecovered", it) }
            result.ipChanged?.let { put("ipChanged", it) }
        }

    private fun publicIpJson(snapshot: PublicIpSnapshot): JSONObject = JSONObject().apply {
        snapshot.ipv4?.let { put("ipv4", it) }
        snapshot.ipv6?.let { put("ipv6", it) }
    }

    private fun isUsbLocalAddress(address: InetAddress?): Boolean {
        if (address == null || address.isAnyLocalAddress || address.isLoopbackAddress) return false
        return RootNetworkInterfaces.read().any { networkInterface ->
            networkInterface.kind == InterfaceKind.USB &&
                (networkInterface.ipv4Addresses + networkInterface.ipv6Addresses).any { candidate ->
                    runCatching { InetAddresses.parseNumericAddress(candidate) }
                        .getOrNull() == address
                }
        }
    }

    private fun readRequest(input: InputStream): HttpRequest {
        val headerBuffer = ByteArrayOutputStream()
        var delimiterState = 0
        while (headerBuffer.size() < MAX_HEADER_BYTES) {
            val value = input.read()
            if (value < 0) throw HttpRequestException(HTTP_BAD_REQUEST, "请求头不完整")
            headerBuffer.write(value)
            delimiterState = when {
                delimiterState == 0 && value == '\r'.code -> 1
                delimiterState == 1 && value == '\n'.code -> 2
                delimiterState == 2 && value == '\r'.code -> 3
                delimiterState == 3 && value == '\n'.code -> 4
                value == '\r'.code -> 1
                else -> 0
            }
            if (delimiterState == 4) break
        }
        if (delimiterState != 4) {
            throw HttpRequestException(HTTP_REQUEST_TOO_LARGE, "请求头过大")
        }

        val headerText = headerBuffer.toString(StandardCharsets.ISO_8859_1.name())
        val lines = headerText.split("\r\n")
        val requestParts = lines.firstOrNull()?.split(' ') ?: emptyList()
        if (requestParts.size != 3 || !requestParts[2].startsWith("HTTP/1.")) {
            throw HttpRequestException(HTTP_BAD_REQUEST, "请求行无效")
        }
        val headers = lines.drop(1)
            .takeWhile { it.isNotEmpty() }
            .mapNotNull { line ->
                val separator = line.indexOf(':')
                if (separator <= 0) null else {
                    line.substring(0, separator).trim().lowercase() to
                        line.substring(separator + 1).trim()
                }
            }
            .toMap()
        val contentLength = headers["content-length"]?.toIntOrNull() ?: 0
        if (contentLength !in 0..MAX_BODY_BYTES) {
            throw HttpRequestException(HTTP_REQUEST_TOO_LARGE, "请求体过大")
        }
        val bodyBytes = ByteArray(contentLength)
        var offset = 0
        while (offset < bodyBytes.size) {
            val count = input.read(bodyBytes, offset, bodyBytes.size - offset)
            if (count <= 0) throw HttpRequestException(HTTP_BAD_REQUEST, "请求体不完整")
            offset += count
        }
        val target = requestParts[1]
        return HttpRequest(
            method = requestParts[0].uppercase(),
            path = target.substringBefore('?'),
            headers = headers,
            body = String(bodyBytes, StandardCharsets.UTF_8)
        )
    }

    private fun writeJson(output: OutputStream, status: Int, body: JSONObject) {
        val bytes = body.toString().toByteArray(StandardCharsets.UTF_8)
        val reason = when (status) {
            HTTP_OK -> "OK"
            HTTP_BAD_REQUEST -> "Bad Request"
            HTTP_FORBIDDEN -> "Forbidden"
            HTTP_NOT_FOUND -> "Not Found"
            HTTP_REQUEST_TOO_LARGE -> "Payload Too Large"
            HTTP_UNSUPPORTED_MEDIA_TYPE -> "Unsupported Media Type"
            else -> "Internal Server Error"
        }
        val headers = buildString {
            append("HTTP/1.1 $status $reason\r\n")
            append("Content-Type: application/json; charset=utf-8\r\n")
            append("Content-Length: ${bytes.size}\r\n")
            append("Cache-Control: no-store\r\n")
            append("X-Content-Type-Options: nosniff\r\n")
            append("Connection: close\r\n\r\n")
        }.toByteArray(StandardCharsets.ISO_8859_1)
        output.write(headers)
        output.write(bytes)
        output.flush()
    }

    private fun observedAt(): String =
        SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss.SSS'Z'", Locale.US).apply {
            timeZone = TimeZone.getTimeZone("UTC")
        }.format(Date())

    private data class HttpRequest(
        val method: String,
        val path: String,
        val headers: Map<String, String>,
        val body: String
    ) {
        fun hasJsonContentType(): Boolean = headers["content-type"]
            ?.substringBefore(';')
            ?.trim()
            ?.equals("application/json", ignoreCase = true) == true
    }

    private data class HttpResponse(val status: Int, val body: JSONObject)

    private class HttpRequestException(val status: Int, override val message: String) :
        Exception(message)

    companion object {
        const val PORT = 17_890
        private const val TAG = "USBridgeControl"
        private const val BACKLOG = 16
        private const val SOCKET_READ_TIMEOUT_MILLIS = 5_000
        private const val MAX_HEADER_BYTES = 16 * 1024
        private const val MAX_BODY_BYTES = 32 * 1024
        private const val HTTP_OK = 200
        private const val HTTP_BAD_REQUEST = 400
        private const val HTTP_FORBIDDEN = 403
        private const val HTTP_NOT_FOUND = 404
        private const val HTTP_REQUEST_TOO_LARGE = 413
        private const val HTTP_UNSUPPORTED_MEDIA_TYPE = 415
        private const val HTTP_INTERNAL_ERROR = 500
        private val MUTATING_METHODS = setOf("POST", "PUT")
    }
}

internal fun isDesktopControlRequest(headers: Map<String, String>): Boolean =
    headers["x-usbridge-client"]?.equals("desktop", ignoreCase = true) == true &&
        headers["origin"].isNullOrBlank()
