// Gate facts: importers=adb instrument after outlook_broker granted;
// callers=wave3 Outlook Graph proof; API=MSAL silent + Graph /me messages draft;
// schemas=live-outlook-graph-proof.txt; user verbatim: "On granted → Graph live → Slack."
package app.codexlauncher.runtime.broker.outlook.live

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import app.codexlauncher.R
import app.codexlauncher.runtime.broker.outlook.OutlookAuthScopes
import app.codexlauncher.runtime.broker.outlook.OutlookAuthorizeActivity
import com.microsoft.identity.client.AcquireTokenSilentParameters
import com.microsoft.identity.client.IAccount
import com.microsoft.identity.client.IAuthenticationResult
import com.microsoft.identity.client.IPublicClientApplication
import com.microsoft.identity.client.ISingleAccountPublicClientApplication
import com.microsoft.identity.client.PublicClientApplication
import com.microsoft.identity.client.SilentAuthenticationCallback
import com.microsoft.identity.client.exception.MsalException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

@RunWith(AndroidJUnit4::class)
class LiveOutlookGraphProofTest {
    @Test
    fun silentTokenThenGraphReadAndDraft() {
        val ctx = InstrumentationRegistry.getInstrumentation().targetContext
        val prefs = ctx.getSharedPreferences(OutlookAuthorizeActivity.PREFS, 0)
        assertEquals("outlook_broker must already be granted", "granted", prefs.getString("status", "missing"))
        assertTrue("access_token_len must be > 0", prefs.getInt("access_token_len", 0) > 0)

        val token = acquireSilentAccessToken()
        assertFalse("silent MSAL must return access token", token.isNullOrBlank())

        val meCode = graphGet("https://graph.microsoft.com/v1.0/me?\$select=id,displayName,mail", token!!)
        val messagesCode =
            graphGet(
                "https://graph.microsoft.com/v1.0/me/messages?\$top=1&\$select=id,subject",
                token,
            )
        val (draftCode, draftId) = graphCreateDraft(token)
        val deleteCode =
            if (draftId.isNotBlank()) {
                graphDelete("https://graph.microsoft.com/v1.0/me/messages/$draftId", token)
            } else {
                -1
            }

        assertTrue("GET /me HTTP must be 200, got $meCode", meCode == 200)
        assertTrue("GET /me/messages HTTP must be 200, got $messagesCode", messagesCode == 200)
        assertTrue("POST draft HTTP must be 201, got $draftCode", draftCode == 201)
        assertTrue("DELETE draft HTTP must be 204, got $deleteCode", deleteCode == 204)

        val text =
            buildString {
                appendLine("status=granted")
                appendLine("account=${prefs.getString("account_hint", "")}")
                appendLine("scopes=${prefs.getString("scopes", "")}")
                appendLine("access_token_len=${token.length}")
                appendLine("me_http=$meCode")
                appendLine("messages_http=$messagesCode")
                appendLine("draft_http=$draftCode")
                appendLine("draft_delete_http=$deleteCode")
                appendLine("verdict=PASS")
            }
        File(ctx.filesDir, "live-outlook-graph-proof.txt").writeText(text)
        runCatching { File("/data/local/tmp/live-outlook-graph-proof.txt").writeText(text) }
    }

    private fun acquireSilentAccessToken(): String? {
        val ctx = InstrumentationRegistry.getInstrumentation().targetContext
        val created = CountDownLatch(1)
        val appRef = AtomicReference<ISingleAccountPublicClientApplication?>()
        val createErr = AtomicReference<String?>()
        PublicClientApplication.createSingleAccountPublicClientApplication(
            ctx,
            R.raw.msal_auth_config,
            object : IPublicClientApplication.ISingleAccountApplicationCreatedListener {
                override fun onCreated(application: ISingleAccountPublicClientApplication) {
                    appRef.set(application)
                    created.countDown()
                }

                override fun onError(exception: MsalException) {
                    createErr.set(exception.message)
                    created.countDown()
                }
            },
        )
        assertTrue("msal create timed out: ${createErr.get()}", created.await(30, TimeUnit.SECONDS))
        val application = appRef.get()
        assertTrue("msal create failed: ${createErr.get()}", application != null)

        val accountLatch = CountDownLatch(1)
        val accountRef = AtomicReference<IAccount?>()
        val accountErr = AtomicReference<String?>()
        application!!.getCurrentAccountAsync(
            object : ISingleAccountPublicClientApplication.CurrentAccountCallback {
                override fun onAccountLoaded(activeAccount: IAccount?) {
                    accountRef.set(activeAccount)
                    accountLatch.countDown()
                }

                override fun onAccountChanged(
                    priorAccount: IAccount?,
                    currentAccount: IAccount?,
                ) {
                    // unused
                }

                override fun onError(exception: MsalException) {
                    accountErr.set(exception.message)
                    accountLatch.countDown()
                }
            },
        )
        assertTrue("getCurrentAccount timed out: ${accountErr.get()}", accountLatch.await(30, TimeUnit.SECONDS))
        val account = accountRef.get()
        assertTrue("no MSAL account for silent acquire: ${accountErr.get()}", account != null)

        val tokenLatch = CountDownLatch(1)
        val tokenRef = AtomicReference<String?>()
        val tokenErr = AtomicReference<String?>()
        val params =
            AcquireTokenSilentParameters.Builder()
                .forAccount(account)
                .fromAuthority(account!!.authority)
                .withScopes(OutlookAuthScopes.operatorMail)
                .withCallback(
                    object : SilentAuthenticationCallback {
                        override fun onSuccess(authenticationResult: IAuthenticationResult) {
                            tokenRef.set(authenticationResult.accessToken)
                            tokenLatch.countDown()
                        }

                        override fun onError(exception: MsalException) {
                            tokenErr.set("${exception.javaClass.simpleName}:${exception.message}")
                            tokenLatch.countDown()
                        }
                    },
                )
                .build()
        application.acquireTokenSilentAsync(params)
        assertTrue("silent acquire timed out: ${tokenErr.get()}", tokenLatch.await(45, TimeUnit.SECONDS))
        assertTrue("silent acquire failed: ${tokenErr.get()}", tokenErr.get() == null)
        return tokenRef.get()
    }

    private fun graphGet(
        url: String,
        accessToken: String,
    ): Int {
        val conn = URL(url).openConnection() as HttpURLConnection
        conn.requestMethod = "GET"
        conn.setRequestProperty("Authorization", "Bearer $accessToken")
        conn.connectTimeout = 20_000
        conn.readTimeout = 20_000
        return try {
            conn.responseCode
        } finally {
            conn.disconnect()
        }
    }

    private fun graphCreateDraft(accessToken: String): Pair<Int, String> {
        val body =
            """
            {"subject":"Operator Graph proof draft (delete me)","body":{"contentType":"Text","content":"live proof"},"toRecipients":[{"emailAddress":{"address":"ssdear@gmail.com"}}]}
            """.trimIndent()
        val conn = URL("https://graph.microsoft.com/v1.0/me/messages").openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        conn.setRequestProperty("Authorization", "Bearer $accessToken")
        conn.setRequestProperty("Content-Type", "application/json")
        conn.connectTimeout = 20_000
        conn.readTimeout = 20_000
        return try {
            conn.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }
            val code = conn.responseCode
            val raw =
                try {
                    (if (code in 200..299) conn.inputStream else conn.errorStream)
                        ?.bufferedReader()
                        ?.readText()
                        .orEmpty()
                } catch (_: Exception) {
                    ""
                }
            val id =
                Regex("\"id\"\\s*:\\s*\"([^\"]+)\"").find(raw)?.groupValues?.get(1).orEmpty()
            code to id
        } finally {
            conn.disconnect()
        }
    }

    private fun graphDelete(
        url: String,
        accessToken: String,
    ): Int {
        val conn = URL(url).openConnection() as HttpURLConnection
        conn.requestMethod = "DELETE"
        conn.setRequestProperty("Authorization", "Bearer $accessToken")
        conn.connectTimeout = 20_000
        conn.readTimeout = 20_000
        return try {
            conn.responseCode
        } finally {
            conn.disconnect()
        }
    }
}
