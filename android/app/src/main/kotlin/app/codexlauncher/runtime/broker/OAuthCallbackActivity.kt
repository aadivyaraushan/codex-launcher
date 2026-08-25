package app.codexlauncher.runtime.broker

import android.app.Activity
import android.content.Context
import android.os.Bundle
import android.util.Log
import org.json.JSONObject
import java.io.BufferedReader
import java.io.InputStreamReader
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.Executors

/**
 * App Link OAuth callback. Parses provider/code/state, then exchanges a pending
 * PKCE verifier for Todoist tokens when PendingOAuthStore is seeded.
 */
class OAuthCallbackActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val uri = intent?.data
        if (uri == null) {
            Log.i(TAG, "[broker] oauth_callback missing_uri")
            finish()
            return
        }
        val parsed =
            runCatching { OAuthCallbackParse.parse(uri.toString()) }
                .onFailure { err ->
                    Log.w(TAG, "[broker] oauth_callback rejected reason=${err.message}")
                }.getOrNull()
        if (parsed == null) {
            finish()
            return
        }
        Log.i(TAG, "[broker] oauth_callback ${parsed.toAuditRecord()}")
        lastCallback = parsed
        val appContext = applicationContext
        Executors.newSingleThreadExecutor().execute {
            try {
                runCatching { exchangeIfPending(appContext, parsed) }
                    .onFailure { err ->
                        Log.w(TAG, "[broker] oauth_token_exchange_failed reason=${err.message}")
                    }
            } finally {
                // B0-015: the raw code is single-use and consumed above -- do not
                // let it linger in a process-wide static after this point.
                lastCallback = null
                lastTokenAdapter = null
            }
        }
        finish()
    }

    companion object {
        private const val TAG = "OAuthCallback"

        // B4-008: fully private. Nothing outside this class reads these, so a
        // public getter only widened the window in which the raw single-use code
        // and state were readable process-wide during the async token exchange.
        // Kept solely as the internal hand-off/clearing holder below.
        @Volatile
        private var lastCallback: OAuthCallback? = null

        @Volatile
        private var lastTokenAdapter: String? = null

        private fun exchangeIfPending(
            context: Context,
            parsed: OAuthCallback,
        ) {
            val pending = PendingOAuthStore.load(context) ?: error("no_pending_oauth")
            require(pending.provider == parsed.provider) { "provider_mismatch" }
            require(pending.state == parsed.state) { "state_mismatch" }
            when (parsed.provider) {
                "todoist" -> {
                    val form =
                        TodoistPublicClientRegistration.tokenExchangeForm(
                            clientId = pending.clientId,
                            code = parsed.code,
                            redirectUri = pending.redirectUri,
                            codeVerifier = pending.codeVerifier,
                        )
                    val raw = postForm("https://api.todoist.com/oauth/access_token", form)
                    val json = JSONObject(raw)
                    val access = json.optString("access_token")
                    require(access.isNotEmpty()) { "missing_access_token" }
                    context
                        .getSharedPreferences("credential_broker", Context.MODE_PRIVATE)
                        .edit()
                        .putString("todoist_access_token", access)
                        .putString("todoist_token_type", json.optString("token_type", "Bearer"))
                        .apply()
                    PendingOAuthStore.clear(context)
                    lastTokenAdapter = "todoist"
                    Log.i(TAG, "[broker] oauth_token_stored provider=todoist")
                }
                else -> error("unsupported_provider")
            }
        }

        private fun postForm(
            url: String,
            form: String,
        ): String {
            val conn = URL(url).openConnection() as HttpURLConnection
            conn.requestMethod = "POST"
            conn.setRequestProperty("Content-Type", "application/x-www-form-urlencoded")
            conn.doOutput = true
            conn.connectTimeout = 15000
            conn.readTimeout = 15000
            OutputStreamWriter(conn.outputStream).use { it.write(form) }
            val code = conn.responseCode
            val stream = if (code in 200..299) conn.inputStream else conn.errorStream
            val text = BufferedReader(InputStreamReader(stream)).use { it.readText() }
            require(code in 200..299) { "http_$code" }
            return text
        }
    }
}
