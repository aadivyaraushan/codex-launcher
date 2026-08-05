package app.codexlauncher.runtime.localpair.handshake

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.localpair.PublicLocalPairOffer
import app.codexlauncher.runtime.localpair.attestation.AttestationClaims
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
            val secret =
                postAttest(
                    offer = offer,
                    nonce = nonce,
                    chainB64 = chainB64,
                    androidAuthPublicKey = pubB64,
                )
            if (secret.isEmpty()) {
                return Result(offer.offerId, false, "empty_secret")
            }
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

    private fun postAttest(
        offer: PublicLocalPairOffer,
        nonce: String,
        chainB64: List<String>,
        androidAuthPublicKey: String,
    ): String {
        val body =
            JSONObject()
                .put("nonce", nonce)
                .put("androidAuthPublicKey", androidAuthPublicKey)
                .put("chainDerBase64", JSONArray(chainB64))
                .toString()
        val raw = pinnedPost(offer, "/v1/local-pair/attest", body)
        val json = JSONObject(raw)
        return json.optString("secret")
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
