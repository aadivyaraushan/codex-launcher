// Gate: importers=adb am start + HUMAN-UNBLOCK Google consent; callers=Pixel user;
// API=Identity.getAuthorizationClient().authorize for calendar.events+drive.file;
// schemas=google_broker prefs status; user: "I think the Google OAuth path is buggy
// in some way. Please fix."
package app.codexlauncher.runtime.broker.google

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.widget.TextView
import androidx.activity.ComponentActivity
import androidx.activity.result.IntentSenderRequest
import androidx.activity.result.contract.ActivityResultContracts
import app.codexlauncher.diagnostics.AppLog
import com.google.android.gms.auth.api.identity.AuthorizationRequest
import com.google.android.gms.auth.api.identity.AuthorizationResult
import com.google.android.gms.auth.api.identity.Identity
import com.google.android.gms.common.api.ApiException
import com.google.android.gms.common.api.Scope

class GoogleAuthorizeActivity : ComponentActivity() {
    private lateinit var label: TextView
    private var awaitingResolution = false
    private var tokenRetryUsed = false

    private val resolutionLauncher =
        registerForActivityResult(ActivityResultContracts.StartIntentSenderForResult()) { result ->
            awaitingResolution = false
            val decision =
                GoogleAuthResultHandler.onActivityResult(
                    resultCode = result.resultCode,
                    intentDataPresent = result.data != null,
                )
            AppLog.info(
                feature = "google-auth",
                message = "authorization activity result",
                fields =
                    mapOf(
                        "result_code" to result.resultCode.toString(),
                        "has_data" to (result.data != null).toString(),
                        "next" to decision.next.name,
                    ),
            )
            when (decision.next) {
                GoogleAuthResultHandler.NextStep.MarkDenied -> {
                    persist(decision.terminalStatus ?: "denied", decision.detail ?: "user_cancelled_or_failed")
                    label.text = "Google access was not granted. Close and try again."
                }
                GoogleAuthResultHandler.NextStep.ParseIntent -> {
                    try {
                        val authResult =
                            Identity.getAuthorizationClient(this)
                                .getAuthorizationResultFromIntent(result.data)
                        applyAuthorizationResult(authResult, fromResolution = true)
                    } catch (e: ApiException) {
                        persist("error", "api_${e.statusCode}")
                        label.text = "Google authorization error (${e.statusCode})."
                        AppLog.error(
                            feature = "google-auth",
                            message = "getAuthorizationResultFromIntent failed",
                            error = e,
                            fields = mapOf("result_code" to result.resultCode.toString()),
                        )
                    }
                }
            }
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        label = TextView(this)
        label.text = "Requesting Google Calendar + Drive access…"
        label.setPadding(48, 48, 48, 48)
        setContentView(label)
        awaitingResolution = savedInstanceState?.getBoolean(STATE_AWAITING) == true
        tokenRetryUsed = savedInstanceState?.getBoolean(STATE_TOKEN_RETRY) == true
        val start =
            GoogleAuthResultHandler.shouldStartAuthorizeOnCreate(
                isRecreate = savedInstanceState != null,
                awaitingResolution = awaitingResolution,
            )
        AppLog.info(
            feature = "google-auth",
            message = "authorize activity onCreate",
            fields =
                mapOf(
                    "recreate" to (savedInstanceState != null).toString(),
                    "awaiting_resolution" to awaitingResolution.toString(),
                    "start_authorize" to start.toString(),
                ),
        )
        if (start) {
            persist("pending", "authorize_started")
            startAuthorize()
        } else {
            label.text =
                "Waiting for Google consent to finish…\n\n" +
                    "Pick aadivya.raushan@gmail.com, then Allow."
        }
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        outState.putBoolean(STATE_AWAITING, awaitingResolution)
        outState.putBoolean(STATE_TOKEN_RETRY, tokenRetryUsed)
    }

    private fun startAuthorize() {
        val request =
            AuthorizationRequest.builder()
                .setRequestedScopes(
                    GoogleAuthScopes.operatorCalendarDrive.map { Scope(it) },
                )
                .build()
        AppLog.info(
            feature = "google-auth",
            message = "calling AuthorizationClient.authorize",
            fields =
                mapOf(
                    "scope_count" to GoogleAuthScopes.operatorCalendarDrive.size.toString(),
                ),
        )
        Identity.getAuthorizationClient(this)
            .authorize(request)
            .addOnSuccessListener { authorizationResult ->
                applyAuthorizationResult(authorizationResult, fromResolution = false)
            }
            .addOnFailureListener { e ->
                persist("error", e.javaClass.simpleName)
                label.text = "Could not start Google authorization: ${e.message}"
                AppLog.error(
                    feature = "google-auth",
                    message = "AuthorizationClient.authorize failed",
                    error = e,
                )
            }
    }

    private fun applyAuthorizationResult(
        authorizationResult: AuthorizationResult,
        fromResolution: Boolean,
    ) {
        val token = authorizationResult.accessToken
        val scopeCount = authorizationResult.grantedScopes?.size ?: 0
        if (authorizationResult.hasResolution() && !fromResolution) {
            val pendingIntent = authorizationResult.pendingIntent
            if (pendingIntent == null) {
                persist("error", "missing_pending_intent")
                label.text = "Google consent UI unavailable."
                return
            }
            awaitingResolution = true
            persist("awaiting_consent", "resolution_launched")
            label.text =
                "Allow Operator to access Google Calendar and Drive.\n\n" +
                    "Tap Allow / Continue on the next Google screen\n" +
                    "(account: aadivya.raushan@gmail.com)."
            resolutionLauncher.launch(
                IntentSenderRequest.Builder(pendingIntent.intentSender).build(),
            )
            return
        }
        val decision =
            GoogleAuthResultHandler.afterParsedResult(
                accessTokenPresent = !token.isNullOrBlank(),
                authorizedScopeCount = scopeCount,
            )
        AppLog.info(
            feature = "google-auth",
            message = "authorization result applied",
            fields =
                mapOf(
                    "from_resolution" to fromResolution.toString(),
                    "token_present" to (!token.isNullOrBlank()).toString(),
                    "token_len" to (token?.length ?: 0).toString(),
                    "scope_count" to scopeCount.toString(),
                    "status" to (decision.terminalStatus ?: ""),
                    "retry" to decision.retryAuthorize.toString(),
                ),
        )
        if (decision.retryAuthorize && !tokenRetryUsed) {
            tokenRetryUsed = true
            persist(decision.terminalStatus ?: "awaiting_token", decision.detail ?: "retry")
            label.text = "Finishing Google authorization…"
            startAuthorize()
            return
        }
        when (decision.terminalStatus) {
            "granted" -> {
                persist(
                    "granted",
                    decision.detail ?: "play_services_cached",
                    accessTokenLen = token?.length ?: 0,
                    grantedScopeCount = scopeCount,
                )
                label.text =
                    "Google Calendar + Drive access granted for Operator.\nYou can return to chat."
            }
            else -> {
                persist(
                    decision.terminalStatus ?: "error",
                    decision.detail ?: "unknown",
                )
                label.text = "Google authorization failed (${decision.detail})."
            }
        }
    }

    private fun persist(
        status: String,
        detail: String,
        accessTokenLen: Int = 0,
        grantedScopeCount: Int = 0,
    ) {
        getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            .edit()
            .putString("status", status)
            .putString("detail", detail)
            .putLong("updated_at_unix", System.currentTimeMillis() / 1000)
            .putString("scopes", GoogleAuthScopes.operatorCalendarDrive.joinToString(" "))
            .putInt("access_token_len", accessTokenLen)
            .putInt("granted_scope_count", grantedScopeCount)
            .commit()
    }

    companion object {
        const val PREFS = "google_broker"
        private const val STATE_AWAITING = "awaiting_resolution"
        private const val STATE_TOKEN_RETRY = "token_retry_used"

        fun launchIntent(context: Context): Intent =
            Intent(context, GoogleAuthorizeActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    }
}
