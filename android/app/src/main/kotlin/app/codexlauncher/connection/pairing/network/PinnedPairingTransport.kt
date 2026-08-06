package app.codexlauncher.connection.pairing.network

import app.codexlauncher.connection.pairing.model.PairingOffer
import app.codexlauncher.connection.security.PinnedTlsClientFactory
import app.codexlauncher.diagnostics.AppLog
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import okhttp3.Dns
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.ByteArrayOutputStream
import java.util.concurrent.TimeUnit

class PinnedPairingTransport private constructor(
    private val endpoint: (PairingOffer) -> String,
    private val tlsClients: PinnedTlsClientFactory,
) : PairingTransport {
    // Callers: LauncherActivity (remote); LocalPairHandshake via localLoopback=true.
    // Affected API: Dns.SYSTEM for 127.0.0.1 /v1/pair. User: loopback phone-runtime session.
    constructor() : this(endpoint = PairingOffer::pairingEndpoint, tlsClients = PinnedTlsClientFactory())

    // Test-only: targets a local MockWebServer URL, not an untrusted pairing-offer
    // host, so it uses the system resolver instead of the public-address DNS filter.
    internal constructor(testEndpoint: String) :
        this(endpoint = { testEndpoint }, tlsClients = PinnedTlsClientFactory(dns = Dns.SYSTEM))

    constructor(localLoopback: Boolean) : this(
        endpoint = PairingOffer::pairingEndpoint,
        tlsClients = PinnedTlsClientFactory(dns = Dns.SYSTEM),
    ) {
        require(localLoopback) { "use PinnedPairingTransport() for remote pairing" }
    }

    override fun pair(offer: PairingOffer, request: PairingRequest): PairingResponse {
        val pin = offer.tlsIdentityPin()
        val client =
            tlsClients.builder(pin)
                .connectTimeout(10, TimeUnit.SECONDS)
                .readTimeout(10, TimeUnit.SECONDS)
                .writeTimeout(10, TimeUnit.SECONDS)
                .callTimeout(15, TimeUnit.SECONDS)
                .build()
        val body =
            buildJsonObject {
                put("secret", request.secret)
                put("host", request.host)
                put("port", request.port)
                put("protocol", request.protocol)
                put("hostPublicKey", request.hostIdentity)
                put("deviceId", request.deviceId)
                put("deviceName", request.deviceName)
                put("devicePublicKey", request.devicePublicKey)
                put("signature", request.signature)
            }.toString()
        val httpRequest =
            Request.Builder()
                .url(endpoint(offer))
                .post(body.toRequestBody(JSON_MEDIA_TYPE))
                .build()
        AppLog.info(
            feature = "pairing-network",
            message = "pinned pairing request started",
            fields = mapOf("input_shape" to "tls13,json", "device_id" to request.deviceId),
        )
        try {
            client.newCall(httpRequest).execute().use { response ->
                if (response.code != 201) throw PairingException("Companion rejected pairing")
                val encoded = readBounded(requireNotNull(response.body))
                val root =
                    runCatching { JSON.parseToJsonElement(encoded).jsonObject }
                        .getOrElse { throw PairingException("Companion returned an invalid pairing response", it) }
                if (root.keys != setOf("deviceId", "pairingGeneration")) {
                    throw PairingException("Companion returned an invalid pairing response")
                }
                val deviceId = root["deviceId"]?.jsonPrimitive?.content
                val generation = root["pairingGeneration"]?.jsonPrimitive?.content
                if (deviceId.isNullOrEmpty() || generation.isNullOrEmpty()) {
                    throw PairingException("Companion returned an invalid pairing response")
                }
                AppLog.info(
                    feature = "pairing-network",
                    message = "pinned pairing response received",
                    fields = mapOf("device_id" to request.deviceId, "output_shape" to "device_id,pairing_generation"),
                )
                return PairingResponse(deviceId, generation)
            }
        } catch (error: Exception) {
            AppLog.error(
                feature = "pairing-network",
                message = "pinned pairing request failed",
                error = error,
                fields = mapOf("device_id" to request.deviceId, "decision" to "fail_closed"),
            )
            if (error is PairingException) throw error
            throw PairingException("Secure connection to companion failed", error)
        } finally {
            client.dispatcher.executorService.shutdown()
            client.connectionPool.evictAll()
        }
    }

    private fun readBounded(body: okhttp3.ResponseBody): String {
        val output = ByteArrayOutputStream()
        val buffer = ByteArray(1024)
        body.byteStream().use { input ->
            while (true) {
                val read = input.read(buffer)
                if (read == -1) break
                if (output.size() + read > MAX_RESPONSE_BYTES) {
                    throw PairingException("Companion returned an oversized pairing response")
                }
                output.write(buffer, 0, read)
            }
        }
        return output.toByteArray().decodeToString(throwOnInvalidSequence = true)
    }

    private companion object {
        const val MAX_RESPONSE_BYTES = 4 * 1024
        val JSON = Json { isLenient = false }
        val JSON_MEDIA_TYPE = "application/json; charset=utf-8".toMediaType()
    }
}
