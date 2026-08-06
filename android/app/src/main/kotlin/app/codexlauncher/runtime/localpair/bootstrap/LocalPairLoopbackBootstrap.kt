package app.codexlauncher.runtime.localpair.bootstrap

import android.content.Context
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.localpair.PublicLocalPairOffer
import app.codexlauncher.runtime.localpair.handshake.LocalPairHandshake
import app.codexlauncher.runtime.localpair.offer.PublicOfferJson
import app.codexlauncher.runtime.standalone.LocalRuntimeEndpoint
import app.codexlauncher.runtime.standalone.StandaloneRuntimeStatusReader
import java.io.BufferedReader
import java.io.InputStreamReader
import java.io.OutputStreamWriter
import java.net.InetSocketAddress
import java.net.Socket
import java.net.URL
import java.security.SecureRandom
import java.security.cert.X509Certificate
import javax.net.ssl.HostnameVerifier
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager

/**
 * Silent phone-runtime local-pair over loopback — no Termux share / AwaitActivity.
 *
 * Callers: LauncherActivity (READY unpaired auto-link + Home "Link local runtime").
 * Affected API: POST https://127.0.0.1:9443/v1/local-pair/offer → LocalPairHandshake.run.
 * Offer JSON fields: protocolVersion, offerId, expiresAt, port, runtimeIdentity, tlsSpki,
 * ephemeralPublicKey, challenge (no secret).
 *
 * Trust-all TLS is only used for the initial offer fetch on 127.0.0.1; subsequent
 * attest/ack/pair use SPKI pinning inside LocalPairHandshake / PinnedPairingTransport.
 *
 * User: "Implement silent auto-link of phone-runtime on Operator app load so users
 * never need Termux UI / Link local runtime / Termux session wanting to connect."
 */
object LocalPairLoopbackBootstrap {
    private const val DEFAULT_PORT = 9443

    sealed class Outcome {
        data object AlreadyReady : Outcome()

        data object NotReachable : Outcome()

        data class Linked(
            val offerId: String,
        ) : Outcome()

        data class Failed(
            val error: String,
        ) : Outcome()
    }

    fun run(
        localPairAcked: Boolean,
        hasEndpoint: Boolean,
        probe: () -> Boolean,
        fetchOffer: () -> Result<PublicLocalPairOffer>,
        handshake: (PublicLocalPairOffer) -> LocalPairHandshake.Result,
    ): Outcome {
        if (localPairAcked) {
            AppLog.info(
                feature = "local-pair",
                message = "loopback auto-link skipped",
                fields =
                    mapOf(
                        "decision" to "already_ready",
                        "has_endpoint" to hasEndpoint,
                    ),
            )
            return Outcome.AlreadyReady
        }
        if (!probe()) {
            AppLog.info(
                feature = "standalone",
                message = "loopback auto-link skipped",
                fields = mapOf("decision" to "not_reachable"),
            )
            return Outcome.NotReachable
        }
        AppLog.info(
            feature = "local-pair",
            message = "loopback auto-link fetching offer",
            fields = mapOf("decision" to "fetch_offer", "port" to DEFAULT_PORT),
        )
        val offer =
            fetchOffer().getOrElse { err ->
                val message = err.message ?: err::class.simpleName.orEmpty()
                AppLog.info(
                    feature = "local-pair",
                    message = "loopback auto-link offer failed",
                    fields = mapOf("error" to message, "decision" to "fail_closed"),
                )
                return Outcome.Failed(message)
            }
        val result = handshake(offer)
        return if (result.acked) {
            AppLog.info(
                feature = "local-pair",
                message = "loopback auto-link linked",
                fields = mapOf("offer_id" to result.offerId, "decision" to "linked"),
            )
            Outcome.Linked(offerId = result.offerId)
        } else {
            val message = result.error ?: "handshake_failed"
            AppLog.info(
                feature = "local-pair",
                message = "loopback auto-link handshake failed",
                fields =
                    mapOf(
                        "offer_id" to result.offerId,
                        "error" to message,
                        "decision" to "fail_closed",
                    ),
            )
            Outcome.Failed(message)
        }
    }

    fun run(context: Context): Outcome {
        val status = StandaloneRuntimeStatusReader.read(context)
        return run(
            localPairAcked = status.localPairAcked,
            hasEndpoint = LocalRuntimeEndpoint.load(context) != null,
            probe = { probeLoopback(DEFAULT_PORT) },
            fetchOffer = { fetchOfferOverLoopback(DEFAULT_PORT) },
            handshake = { offer -> LocalPairHandshake.run(context, offer) },
        )
    }

    fun fetchOfferOverLoopback(port: Int = DEFAULT_PORT): Result<PublicLocalPairOffer> =
        runCatching {
            require(port in 1..65535) { "invalid_port" }
            val tm =
                object : X509TrustManager {
                    override fun checkClientTrusted(
                        chain: Array<out X509Certificate>?,
                        authType: String?,
                    ) = Unit

                    override fun checkServerTrusted(
                        chain: Array<out X509Certificate>?,
                        authType: String?,
                    ) = Unit

                    override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
                }
            val ssl = SSLContext.getInstance("TLS")
            ssl.init(null, arrayOf<TrustManager>(tm), SecureRandom())
            // Loopback-only offer bootstrap; pin after PublicLocalPairOffer.tlsSpki is known.
            val url = URL("https://127.0.0.1:$port/v1/local-pair/offer")
            val conn = (url.openConnection() as HttpsURLConnection)
            conn.sslSocketFactory = ssl.socketFactory
            conn.hostnameVerifier = HostnameVerifier { hostname, _ -> hostname == "127.0.0.1" }
            conn.requestMethod = "POST"
            conn.setRequestProperty("Content-Type", "application/json")
            conn.doOutput = true
            conn.connectTimeout = 8000
            conn.readTimeout = 8000
            OutputStreamWriter(conn.outputStream).use { it.write("{}") }
            val code = conn.responseCode
            val stream = if (code in 200..299) conn.inputStream else conn.errorStream
            val text = BufferedReader(InputStreamReader(stream)).use { it.readText() }
            if (code !in 200..299) {
                error("offer_http_$code:$text")
            }
            PublicOfferJson.parse(text).getOrThrow()
        }

    private fun probeLoopback(port: Int): Boolean =
        try {
            Socket().use { socket ->
                socket.connect(InetSocketAddress("127.0.0.1", port), 250)
                true
            }
        } catch (_: Exception) {
            false
        }
}
