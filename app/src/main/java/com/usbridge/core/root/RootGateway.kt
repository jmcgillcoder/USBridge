package com.usbridge.core.root

import com.topjohnwu.superuser.Shell
import com.usbridge.core.model.RootState
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

data class RootEnvironment(
    val uid: String? = null,
    val selinuxMode: String? = null,
    val rootImplementation: String? = null,
    val usbState: String? = null,
    val usbConfig: String? = null,
    val usbDeviceConnected: Boolean? = null,
    val usbConfigured: Boolean? = null,
    val usbDataRole: String? = null,
    val availableTools: Set<String> = emptySet()
)

class RootGateway {
    suspend fun requestRoot(): RootState = withContext(Dispatchers.IO) {
        try {
            if (Shell.getShell().isRoot) RootState.GRANTED else RootState.DENIED
        } catch (_: Throwable) {
            RootState.ERROR
        }
    }

    suspend fun readEnvironment(): RootEnvironment = withContext(Dispatchers.IO) {
        if (!Shell.getShell().isRoot) return@withContext RootEnvironment()

        val usbManagerState = run(RootProbe.USB_MANAGER_STATE)
        val usbDataRole = run(RootProbe.USB_DATA_ROLE)
        RootEnvironment(
            uid = run(RootProbe.UID).firstUsefulLine(),
            selinuxMode = run(RootProbe.SELINUX).firstUsefulLine(),
            rootImplementation = run(RootProbe.ROOT_IMPLEMENTATION).firstUsefulLine(),
            usbState = run(RootProbe.USB_STATE).firstUsefulLine(),
            usbConfig = run(RootProbe.USB_CONFIG).firstUsefulLine(),
            usbDeviceConnected = usbManagerState.booleanField("connected"),
            usbConfigured = usbManagerState.booleanField("configured"),
            usbDataRole = usbDataRole.stringField("data_role"),
            availableTools = run(RootProbe.AVAILABLE_TOOLS)
                .output
                .map(String::trim)
                .filter(String::isNotEmpty)
                .toSet()
        )
    }

    private fun run(probe: RootProbe): RootCommandResult {
        return try {
            val result = Shell.cmd(probe.command).exec()
            RootCommandResult(
                code = result.code,
                output = result.out,
                errors = result.err
            )
        } catch (error: Throwable) {
            RootCommandResult(code = -1, errors = listOfNotNull(error.message))
        }
    }
}

private data class RootCommandResult(
    val code: Int,
    val output: List<String> = emptyList(),
    val errors: List<String> = emptyList()
) {
    fun firstUsefulLine(): String? {
        if (code != 0) return null
        return output
            .asSequence()
            .map(String::trim)
            .firstOrNull { it.isNotEmpty() }
    }

    fun stringField(name: String): String? {
        if (code != 0) return null
        val prefix = "$name="
        return output
            .asSequence()
            .map(String::trim)
            .firstOrNull { it.startsWith(prefix) }
            ?.substringAfter('=')
            ?.trim()
            ?.takeIf(String::isNotEmpty)
    }

    fun booleanField(name: String): Boolean? = when (stringField(name)) {
        "true" -> true
        "false" -> false
        else -> null
    }
}

private enum class RootProbe(val command: String) {
    UID("id -u"),
    SELINUX("getenforce"),
    ROOT_IMPLEMENTATION(
        "if pm path me.weishu.kernelsu >/dev/null 2>&1 || [ -d /data/adb/ksu ]; then " +
            "echo KernelSU; elif pm path me.bmax.apatch >/dev/null 2>&1 || " +
            "[ -d /data/adb/ap ]; then echo APatch; " +
            "elif command -v magisk >/dev/null 2>&1 || [ -d /data/adb/magisk ]; then " +
            "echo Magisk; " +
            "else su -v 2>/dev/null || true; fi"
    ),
    USB_STATE("getprop sys.usb.state"),
    USB_CONFIG("getprop sys.usb.config"),
    USB_MANAGER_STATE("dumpsys usb | head -n 25"),
    USB_DATA_ROLE("dumpsys usb | grep -m 1 'data_role='"),
    AVAILABLE_TOOLS(
        "for tool in svc cmd ip iptables ip6tables nft; do " +
            "if command -v \"\u0024tool\" >/dev/null 2>&1; then echo \"\u0024tool\"; fi; done"
    )
}
