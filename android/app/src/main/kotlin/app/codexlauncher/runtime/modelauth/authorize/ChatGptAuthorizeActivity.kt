package app.codexlauncher.runtime.modelauth.authorize

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.widget.TextView
import androidx.activity.ComponentActivity
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.modelauth.loopback.ModelAuthClient
import kotlin.concurrent.thread

/**
 * Simple device-code ChatGPT sign-in, matching GoogleAuthorizeActivity:
 * a TextView, a verification URL via ACTION_VIEW, no Mac pairing copy.
 */
class ChatGptAuthorizeActivity : ComponentActivity() {
    private lateinit var label: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        label = TextView(this)
        label.text = "Sign in with ChatGPT to use Operator…"
        label.setPadding(48, 48, 48, 48)
        setContentView(label)
        AppLog.info(feature = "model-auth", message = "authorize activity onCreate", fields = mapOf("decision" to "start_device_login"))
        thread(name = "chatgpt-device-login") { startLogin() }
    }

    private fun startLogin() {
        try {
            val start = ModelAuthClient.start()
            runOnUiThread {
                label.text =
                    "Sign in with ChatGPT to use Operator\n\n" +
                        "Enter this code in ChatGPT:\n\n${start.userCode}"
            }
            startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(start.verificationUrl)))
            pollUntilReady()
        } catch (error: Exception) {
            AppLog.info(
                feature = "model-auth",
                message = "authorize activity start failed",
                fields = mapOf("error" to (error.message ?: error::class.simpleName.orEmpty())),
            )
            runOnUiThread {
                label.text = "Could not start ChatGPT sign-in. Close and try again."
            }
        }
    }

    private fun pollUntilReady() {
        repeat(90) {
            val health = runCatching { ModelAuthClient.health() }.getOrNull()
            if (health?.gateOpen == true) {
                AppLog.info(feature = "model-auth", message = "authorize activity complete", fields = mapOf("decision" to "oauth_ready"))
                runOnUiThread { finish() }
                return
            }
            Thread.sleep(2_000)
        }
    }

    companion object {
        fun intent(context: Context): Intent = Intent(context, ChatGptAuthorizeActivity::class.java)
    }
}
