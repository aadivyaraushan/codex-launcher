// Gate: importers=adb am start + HUMAN-UNBLOCK Outlook; callers=Pixel user;
// API=MSAL SingleAccountPublicClientApplication acquireToken/Silent; schemas=
// outlook_broker prefs; user: "Open Outlook consent next (MSAL)"
package app.codexlauncher.runtime.broker.outlook

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.widget.TextView
import androidx.activity.ComponentActivity
import app.codexlauncher.R
import app.codexlauncher.diagnostics.AppLog
import com.microsoft.identity.client.AcquireTokenParameters
import com.microsoft.identity.client.AcquireTokenSilentParameters
import com.microsoft.identity.client.AuthenticationCallback
import com.microsoft.identity.client.IAccount
import com.microsoft.identity.client.IAuthenticationResult
import com.microsoft.identity.client.IPublicClientApplication
import com.microsoft.identity.client.ISingleAccountPublicClientApplication
import com.microsoft.identity.client.PublicClientApplication
import com.microsoft.identity.client.SilentAuthenticationCallback
import com.microsoft.identity.client.exception.MsalException

class OutlookAuthorizeActivity : ComponentActivity() {
    private lateinit var label: TextView
    private var app: ISingleAccountPublicClientApplication? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        label = TextView(this)
        label.text = "Requesting Outlook mail access…"
        label.setPadding(48, 48, 48, 48)
        setContentView(label)
        persist("pending", "msal_init")
        PublicClientApplication.createSingleAccountPublicClientApplication(
            applicationContext,
            R.raw.msal_auth_config,
            object : IPublicClientApplication.ISingleAccountApplicationCreatedListener {
                override fun onCreated(application: ISingleAccountPublicClientApplication) {
                    app = application
                    AppLog.info(
                        feature = "outlook-auth",
                        message = "msal single-account app created",
                        fields = mapOf("redirect" to OutlookAuthScopes.redirectUri),
                    )
                    beginAcquire(application)
                }

                override fun onError(exception: MsalException) {
                    persist("error", "msal_init_${exception.javaClass.simpleName}")
                    label.text = "Could not start Outlook sign-in: ${exception.message}"
                    AppLog.error(
                        feature = "outlook-auth",
                        message = "msal init failed",
                        error = exception,
                    )
                }
            },
        )
    }

    private fun beginAcquire(application: ISingleAccountPublicClientApplication) {
        application.getCurrentAccountAsync(
            object : ISingleAccountPublicClientApplication.CurrentAccountCallback {
                override fun onAccountLoaded(activeAccount: IAccount?) {
                    if (activeAccount != null) {
                        acquireSilent(application, activeAccount)
                    } else {
                        acquireInteractive(application)
                    }
                }

                override fun onAccountChanged(
                    priorAccount: IAccount?,
                    currentAccount: IAccount?,
                ) {
                    // no-op for consent launch
                }

                override fun onError(exception: MsalException) {
                    AppLog.error(
                        feature = "outlook-auth",
                        message = "getCurrentAccount failed; falling back to interactive",
                        error = exception,
                    )
                    acquireInteractive(application)
                }
            },
        )
    }

    private fun acquireSilent(
        application: ISingleAccountPublicClientApplication,
        account: IAccount,
    ) {
        persist("pending", "silent_started")
        val params =
            AcquireTokenSilentParameters.Builder()
                .forAccount(account)
                .fromAuthority(account.authority)
                .withScopes(OutlookAuthScopes.operatorMail)
                .withCallback(
                    object : SilentAuthenticationCallback {
                        override fun onSuccess(authenticationResult: IAuthenticationResult) {
                            onToken(authenticationResult, "silent")
                        }

                        override fun onError(exception: MsalException) {
                            AppLog.info(
                                feature = "outlook-auth",
                                message = "silent failed; launching interactive",
                                fields = mapOf("error_type" to exception.javaClass.simpleName),
                            )
                            acquireInteractive(application)
                        }
                    },
                )
                .build()
        application.acquireTokenSilentAsync(params)
    }

    private fun acquireInteractive(application: ISingleAccountPublicClientApplication) {
        persist("awaiting_consent", "interactive_launched")
        label.text =
            "Allow Operator to access Outlook mail.\n\n" +
                "Sign in as ${OutlookAuthScopes.loginHint}, then Accept."
        val params =
            AcquireTokenParameters.Builder()
                .startAuthorizationFromActivity(this)
                .withScopes(OutlookAuthScopes.operatorMail)
                .withLoginHint(OutlookAuthScopes.loginHint)
                .withCallback(
                    object : AuthenticationCallback {
                        override fun onSuccess(authenticationResult: IAuthenticationResult) {
                            onToken(authenticationResult, "interactive")
                        }

                        override fun onError(exception: MsalException) {
                            persist("error", "msal_${exception.javaClass.simpleName}")
                            label.text = "Outlook authorization failed: ${exception.message}"
                            AppLog.error(
                                feature = "outlook-auth",
                                message = "interactive acquireToken failed",
                                error = exception,
                            )
                        }

                        override fun onCancel() {
                            persist("denied", "user_cancelled")
                            label.text = "Outlook access was not granted. Close and try again."
                            AppLog.info(feature = "outlook-auth", message = "interactive cancelled")
                        }
                    },
                )
                .build()
        application.acquireToken(params)
    }

    private fun onToken(
        result: IAuthenticationResult,
        path: String,
    ) {
        val token = result.accessToken
        persist(
            status = "granted",
            detail = path,
            accessTokenLen = token.length,
            accountHint = result.account.username ?: OutlookAuthScopes.loginHint,
        )
        label.text =
            "Outlook mail access granted for Operator.\nYou can return to chat."
        AppLog.info(
            feature = "outlook-auth",
            message = "authorization granted",
            fields =
                mapOf(
                    "path" to path,
                    "access_len" to token.length.toString(),
                    "account_present" to (!result.account.username.isNullOrBlank()).toString(),
                ),
        )
    }

    private fun persist(
        status: String,
        detail: String,
        accessTokenLen: Int = 0,
        accountHint: String = "",
    ) {
        getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            .edit()
            .putString("status", status)
            .putString("detail", detail)
            .putLong("updated_at_unix", System.currentTimeMillis() / 1000)
            .putString("scopes", OutlookAuthScopes.operatorMail.joinToString(" "))
            .putInt("access_token_len", accessTokenLen)
            .putString("account_hint", accountHint)
            .commit()
    }

    companion object {
        const val PREFS = "outlook_broker"

        fun launchIntent(context: Context): Intent =
            Intent(context, OutlookAuthorizeActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    }
}
