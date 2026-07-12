package app.codexlauncher

import android.app.Activity
import android.os.Bundle
import android.widget.TextView
import app.codexlauncher.diagnostics.AppLog

class LauncherActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        AppLog.info(
            feature = "launcher",
            message = "activity created",
            fields = mapOf("input_shape" to "saved_state=${savedInstanceState != null}"),
        )
        setContentView(
            TextView(this).apply {
                text = "Codex Launcher"
                contentDescription = "Codex Launcher bootstrap screen"
            },
        )
    }
}
