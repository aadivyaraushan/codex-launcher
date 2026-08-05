package app.codexlauncher.runtime

import android.app.Activity
import android.os.Bundle
import android.widget.TextView
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.localpair.LocalPairAwaitingStore

/**
 * User-opened "Link local runtime" screen. Holds the import latch open while
 * visible so Termux can share a public local-pair offer.
 */
class LocalPairAwaitActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        LocalPairAwaitingStore.beginWaiting()
        AppLog.info(
            feature = "local-pair",
            message = "local pair awaiting offer",
            fields = mapOf("waiting" to "true"),
        )
        val label = TextView(this)
        label.text = "Waiting for Termux local-pair offer…"
        label.setPadding(48, 48, 48, 48)
        setContentView(label)
    }

    override fun onDestroy() {
        LocalPairAwaitingStore.clearWaiting()
        AppLog.info(
            feature = "local-pair",
            message = "local pair await closed",
            fields = mapOf("waiting" to "false"),
        )
        super.onDestroy()
    }
}
