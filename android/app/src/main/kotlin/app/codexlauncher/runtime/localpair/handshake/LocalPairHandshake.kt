package app.codexlauncher.runtime.localpair.handshake

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import app.codexlauncher.connection.pairing.model.PairingOffer
import app.codexlauncher.connection.pairing.network.AndroidDevicePairingSigner
import app.codexlauncher.connection.pairing.network.PairingClient
import app.codexlauncher.connection.pairing.network.PinnedPairingTransport
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.localpair.PublicLocalPairOffer
import app.codexlauncher.runtime.localpair.attestation.AttestationClaims
import app.codexlauncher.runtime.standalone.LocalRuntimeEndpoint
import app.codexlauncher.storage.secrets.PairingKeyStore
import org.json.JSONArray
import org.json.JSONObject
import java.io.BufferedReader
import java.io.InputStreamReader
import java.io.OutputStreamWriter
import java.net.URL
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.MessageDigest
import java.security.SecureRandom
import java.security.cert.X509Certificate
import java.security.spec.ECGenParameterSpec
import javax.net.ssl.HostnameVerifier
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager

object LocalPairHandshake {
    data class Result(
        val offerId: String,
        val acked: Boolean,
        val error: String? = null,
    )

    fun run(
        context: Context,
        offer: PublicLocalPairOffer,
    ): Result {
        val nonceBytes = ByteArray(16)
        SecureRandom().nextBytes(nonceBytes)
        val nonce = Base64.encodeToString(nonceBytes, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING)
        val challenge =
            AttestationClaims.challengeBytes(
                offerId = offer.offerId,
                runtimeIdentity = offer.runtimeIdentity,
                ephemeralPublicKey = offer.ephemeralPublicKey,
                tlsSpki = offer.tlsSpki,
                transcriptNonce = nonce,
            )
        val alias = "local-runtime-auth-${offer.offerId}"
        return try {
            val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
            if (keyStore.containsAlias(alias)) {
                keyStore.deleteEntry(alias)
            }
            val kpg = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore")
            val spec =
                KeyGenParameterSpec.Builder(
                    alias,
                    KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY,
                )
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256)
                    .setAttestationChallenge(challenge)
                    .build()
            kpg.initialize(spec)
            val pair = kpg.generateKeyPair()
            val pubB64 =
                Base64.encodeToString(pair.public.encoded, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING)
            val chain = keyStore.getCertificateChain(alias) ?: error("missing attestation chain")
            val chainB64 =
                chain.map { cert ->
                    Base64.encodeToString(cert.encoded, Base64.NO_WRAP)
                }
            AppLog.info(
                feature = "local-pair",
                message = "local pair attestation chain ready",
                fields =
                    mapOf(
                        "offer_id" to offer.offerId,
                        "chain_len" to chain.size.toString(),
                    ),
            )
            // Callers: LocalPairImportActivity.kt:100. Affected: attest→/v1/pair enroll→ack.
            // Attest JSON may include sessionSecret,hostPublicKey,tlsPublicKey,host,port,protocol.
            // User: "open a real session/transport to phone-runtime on loopback" via existing pairing auth.
            val attest =
                postAttest(
                    offer = offer,
                    nonce = nonce,
                    chainB64 = chainB64,
                    androidAuthPublicKey = pubB64,
                )
            if (attest.secret.isEmpty()) {
                return Result(offer.offerId, false, "empty_secret")
            }
            enrollLocalSession(context, attest)
            postAck(offer = offer, androidAuthPublicKey = pubB64)
            persistAck(context, offer)
            AppLog.info(
                feature = "local-pair",
                message = "local pair durable ack complete",
                fields = mapOf("offer_id" to offer.offerId, "acked" to "true"),
            )
            Result(offer.offerId, true)
        } catch (err: Exception) {
            AppLog.info(
                feature = "local-pair",
                message = "local pair handshake failed",
                fields =
                    mapOf(
                        "offer_id" to offer.offerId,
                        "error" to (err.message ?: err::class.simpleName.orEmpty()),
                    ),
            )
            Result(offer.offerId, false, err.message ?: err::class.simpleName)
        }
    }

    private data class AttestResult(
        val secret: String,
        val sessionSecret: String,
        val hostPublicKey: String,
        val tlsPublicKey: String,
        val host: String,
        val port: Int,
        val protocol: Int,
    )

    private fun postAttest(
        offer: PublicLocalPairOffer,
        nonce: String,
        chainB64: List<String>,
        androidAuthPublicKey: String,
    ): AttestResult {
        val body =
            JSONObject()
                .put("nonce", nonce)
                .put("androidAuthPublicKey", androidAuthPublicKey)
                .put("chainDerBase64", JSONArray(chainB64))
                .toString()
        val raw = pinnedPost(offer, "/v1/local-pair/attest", body)
        val json = JSONObject(raw)
        return AttestResult(
            secret = json.optString("secret"),
            sessionSecret = json.optString("sessionSecret"),
            hostPublicKey = json.optString("hostPublicKey"),
            tlsPublicKey = json.optString("tlsPublicKey"),
            host = json.optString("host", "127.0.0.1"),
            port = json.optInt("port", offer.port),
            protocol = json.optInt("protocol", 1),
        )
    }

    private fun enrollLocalSession(
        context: Context,
        attest: AttestResult,
    ) {
        if (
            attest.sessionSecret.isBlank() ||
            attest.hostPublicKey.isBlank() ||
            attest.tlsPublicKey.isBlank()
        ) {
            AppLog.info(
                feature = "local-pair",
                message = "local pair attest missing session enrollment fields",
                fields = mapOf("decision" to "skip_session_enroll"),
            )
            return
        }
        val offer =
            PairingOffer.forLocalRuntime(
                secret = attest.sessionSecret,
                hostIdentity = attest.hostPublicKey,
                tlsIdentity = attest.tlsPublicKey,
                host = attest.host,
                port = attest.port,
                protocol = attest.protocol,
            )
        val client =
            PairingClient(
                AndroidDevicePairingSigner(PairingKeyStore()),
                PinnedPairingTransport(localLoopback = true),
            )
        val paired =
            client.pair(
                offer = offer,
                deviceId = "local-android",
                deviceName = android.os.Build.MODEL.ifBlank { "Android device" },
            )
        LocalRuntimeEndpoint.save(context, paired)
        AppLog.info(
            feature = "local-pair",
            message = "local runtime session enrollment complete",
            fields = mapOf("device_id" to paired.deviceId, "port" to paired.port),
        )
    }

    private fun postAck(
        offer: PublicLocalPairOffer,
        androidAuthPublicKey: String,
    ) {
        val body =
            JSONObject()
                .put("offerId", offer.offerId)
                .put("androidAuthPublicKey", androidAuthPublicKey)
                .toString()
        pinnedPost(offer, "/v1/local-pair/ack", body)
    }

    private fun persistAck(
        context: Context,
        offer: PublicLocalPairOffer,
    ) {
        context
            .getSharedPreferences("local_pair_runtime", Context.MODE_PRIVATE)
            .edit()
            .putString("runtimeIdentity", offer.runtimeIdentity)
            .putString("tlsSpki", offer.tlsSpki)
            .putString("offerId", offer.offerId)
            .putInt("epoch", 1)
            .apply()
    }

    private fun pinnedPost(
        offer: PublicLocalPairOffer,
        path: String,
        body: String,
    ): String {
        val wantSpki = Base64.decode(offer.tlsSpki, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING)
        val tm =
            object : X509TrustManager {
                override fun checkClientTrusted(
                    chain: Array<out X509Certificate>?,
                    authType: String?,
                ) = Unit

                override fun checkServerTrusted(
                    chain: Array<out X509Certificate>?,
                    authType: String?,
                ) {
                    val leaf = chain?.firstOrNull() ?: error("empty server chain")
                    val leafSpki = MessageDigest.getInstance("SHA-256").digest(leaf.publicKey.encoded)
                    if (!leafSpki.contentEquals(wantSpki)) {
                        error("tls_spki_mismatch")
                    }
                }

                override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
            }
        val ctx = SSLContext.getInstance("TLS")
        ctx.init(null, arrayOf<TrustManager>(tm), SecureRandom())
        val url = URL("https://127.0.0.1:${offer.port}$path")
        val conn = (url.openConnection() as HttpsURLConnection)
        conn.sslSocketFactory = ctx.socketFactory
        conn.hostnameVerifier = HostnameVerifier { _, _ -> true }
        conn.requestMethod = "POST"
        conn.setRequestProperty("Content-Type", "application/json")
        conn.doOutput = true
        conn.connectTimeout = 8000
        conn.readTimeout = 8000
        OutputStreamWriter(conn.outputStream).use { it.write(body) }
        val code = conn.responseCode
        val stream = if (code in 200..299) conn.inputStream else conn.errorStream
        val text = BufferedReader(InputStreamReader(stream)).use { it.readText() }
        if (code !in 200..299) {
            error("http_$code:$text")
        }
        return text
    }
}
