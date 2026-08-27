package com.usbridge

import android.app.Application
import com.topjohnwu.superuser.Shell

class USBridgeApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        Shell.setDefaultBuilder(
            Shell.Builder.create()
                .setFlags(Shell.FLAG_REDIRECT_STDERR)
                .setTimeout(ROOT_SHELL_TIMEOUT_SECONDS)
        )
    }

    private companion object {
        const val ROOT_SHELL_TIMEOUT_SECONDS = 15L
    }
}
