package app.codexlauncher.connection.session

import app.codexlauncher.connection.pairing.model.PairingWire
import app.codexlauncher.connection.pairing.network.DevicePairingSigner
import app.codexlauncher.connection.pairing.network.PairedComputer
import app.codexlauncher.connection.security.HostIdentityPin
import app.codexlauncher.diagnostics.AppLog
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.add
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put
import java.security.MessageDigest
import java.util.Base64
import java.util.UUID

class SessionHandshakeException(
    message: String,
    cause: Throwable? = null,
) : Exception(message, cause)

class SessionHandshake(
    private val paired: PairedComputer,
    private val sessionId: String,
    private val signer: DevicePairingSigner,
    private val nowSeconds: () -> Long = { System.currentTimeMillis() / 1_000 },
    private val messageId: () -> String = { UUID.randomUUID().toString() },
) {
    sealed interface Output {
        data class Proof(val json: String) : Output

        data class Ready(
            val helloJson: String,
            val attachmentKey: ByteArray,
        ) : Output
    }

    private enum class Stage {
        CHALLENGE,
        AUTHENTICATED,
        READY,
        CLOSED,
    }

    private val hostPin = HostIdentityPin.parse(paired.hostIdentity)
    private var stage = Stage.CHALLENGE
    private var attachmentKey: ByteArray? = null

    init {
        require(validIdentifier(sessionId)) { "Invalid session identifier" }
    }

    fun receive(encoded: String): Output {
        if (stage == Stage.CLOSED || stage == Stage.READY) throw SessionHandshakeException("Session handshake is closed")
        try {
            return when (stage) {
                Stage.CHALLENGE -> receiveChallenge(encoded)
                Stage.AUTHENTICATED -> receiveAuthenticated(encoded)
                Stage.READY, Stage.CLOSED -> throw SessionHandshakeException("Session handshake is closed")
            }
        } catch (error: Exception) {
            stage = Stage.CLOSED
            AppLog.error(
                feature = "session-handshake",
                message = "session handshake rejected",
                error = error,
                fields = mapOf("device_id" to paired.deviceId, "session_id" to sessionId, "decision" to "fail_closed"),
            )
            if (error is SessionHandshakeException) throw error
            throw SessionHandshakeException("Invalid session handshake", error)
        }
    }

    private fun receiveChallenge(encoded: String): Output.Proof {
        val challenge = parseObject(encoded)
        if (challenge.keys != CHALLENGE_KEYS) throw SessionHandshakeException("Invalid session challenge")
        val deviceId = challenge.string("deviceId")
        val receivedSessionId = challenge.string("sessionId")
        val generation = challenge.string("pairingGeneration")
        val protocol = challenge.int("protocol")
        val hostIdentity = challenge.string("hostPublicKey")
        val nonce = challenge.string("nonce")
        val expiresAt = challenge.long("expiresAt")
        val hostSignature = challenge.string("hostSignature")
        val currentTime = nowSeconds()
        if (
            deviceId != paired.deviceId ||
            receivedSessionId != sessionId ||
            generation != paired.pairingGeneration ||
            protocol != paired.protocol ||
            hostIdentity != paired.hostIdentity ||
            expiresAt !in (currentTime + 1)..(currentTime + MAX_CHALLENGE_LIFETIME_SECONDS)
        ) {
            throw SessionHandshakeException("Session challenge binding changed")
        }
        val nonceBytes = decodeCanonical(nonce, 32)
        val hostSignatureBytes = decodeCanonical(hostSignature, 64)
        if (nonceBytes.size != 32) throw SessionHandshakeException("Invalid session challenge nonce")
        val challengeMessage =
            PairingWire.sessionChallengeMessage(
                deviceId = deviceId,
                sessionId = receivedSessionId,
                pairingGeneration = generation,
                protocol = protocol,
                hostIdentity = hostIdentity,
                nonce = nonce,
                expiresAt = expiresAt,
            )
        if (!hostPin.verifies(challengeMessage, hostSignatureBytes)) {
            throw SessionHandshakeException("Companion host challenge signature is invalid")
        }
        val proofMessage =
            PairingWire.sessionProofMessage(
                deviceId = deviceId,
                sessionId = receivedSessionId,
                pairingGeneration = generation,
                protocol = protocol,
                hostIdentity = hostIdentity,
                nonce = nonce,
                expiresAt = expiresAt,
                hostSignature = hostSignature,
            )
        val phoneSignature = signer.sign(proofMessage)
        attachmentKey = MessageDigest.getInstance("SHA-256").digest(hostSignatureBytes + phoneSignature)
        stage = Stage.AUTHENTICATED
        AppLog.info(
            feature = "session-handshake",
            message = "host challenge verified",
            fields = mapOf("device_id" to paired.deviceId, "session_id" to sessionId, "output_shape" to "p256_session_proof"),
        )
        return Output.Proof(
            buildJsonObject {
                put("deviceId", deviceId)
                put("sessionId", receivedSessionId)
                put("pairingGeneration", generation)
                put("protocol", protocol)
                put("hostPublicKey", hostIdentity)
                put("nonce", nonce)
                put("expiresAt", expiresAt)
                put("hostSignature", hostSignature)
                put("signature", encode(phoneSignature))
            }.toString(),
        )
    }

    private fun receiveAuthenticated(encoded: String): Output.Ready {
        val authenticated = parseObject(encoded)
        if (
            authenticated.keys != AUTHENTICATED_KEYS ||
            authenticated.string("type") != "authenticated" ||
            authenticated.string("deviceId") != paired.deviceId ||
            authenticated.string("sessionId") != sessionId
        ) {
            throw SessionHandshakeException("Session authentication acknowledgement changed")
        }
        val key = requireNotNull(attachmentKey).copyOf()
        stage = Stage.READY
        val hello =
            buildJsonObject {
                put("version", buildJsonObject { put("major", paired.protocol); put("minor", 0) })
                put("messageId", messageId())
                put("sender", "phone")
                put("type", "hello")
                put(
                    "body",
                    buildJsonObject {
                        put("clientInstanceId", paired.deviceId)
                        put("supportedMajors", buildJsonArray { add(paired.protocol) })
                        put("resume", buildJsonObject { put("mode", "no_local_state") })
                    },
                )
            }.toString()
        AppLog.info(
            feature = "session-handshake",
            message = "session authenticated",
            fields = mapOf("device_id" to paired.deviceId, "session_id" to sessionId, "output_shape" to "cold_hello,attachment_key"),
        )
        return Output.Ready(helloJson = hello, attachmentKey = key)
    }

    private fun parseObject(encoded: String): JsonObject =
        if (encoded.encodeToByteArray().size > MAX_HANDSHAKE_FRAME_BYTES) {
            throw SessionHandshakeException("Session handshake frame is too large")
        } else {
            runCatching { JSON.parseToJsonElement(encoded).jsonObject }
                .getOrElse { throw SessionHandshakeException("Invalid session handshake JSON", it) }
        }

    private fun JsonObject.string(name: String): String {
        val primitive = this[name]?.jsonPrimitive ?: throw SessionHandshakeException("Missing session handshake field")
        if (!primitive.isString) throw SessionHandshakeException("Invalid session handshake field")
        return primitive.content
    }

    private fun JsonObject.int(name: String): Int =
        this[name]?.jsonPrimitive?.intOrNull ?: throw SessionHandshakeException("Invalid session handshake field")

    private fun JsonObject.long(name: String): Long =
        this[name]?.jsonPrimitive?.longOrNull ?: throw SessionHandshakeException("Invalid session handshake field")

    private fun decodeCanonical(value: String, expectedSize: Int): ByteArray {
        val decoded = runCatching { Base64.getUrlDecoder().decode(value) }.getOrNull()
            ?: throw SessionHandshakeException("Invalid session handshake encoding")
        if (decoded.size != expectedSize || encode(decoded) != value) {
            throw SessionHandshakeException("Invalid session handshake encoding")
        }
        return decoded
    }

    private fun encode(value: ByteArray): String =
        Base64.getUrlEncoder().withoutPadding().encodeToString(value)

    private fun validIdentifier(value: String): Boolean =
        value.length in 1..128 && value.first().isAsciiLetterOrDigit() && value.drop(1).all { it.isAsciiLetterOrDigit() || it in "._:-" }

    private fun Char.isAsciiLetterOrDigit(): Boolean =
        this in 'A'..'Z' || this in 'a'..'z' || this in '0'..'9'

    private companion object {
        const val MAX_HANDSHAKE_FRAME_BYTES = 16 * 1024
        const val MAX_CHALLENGE_LIFETIME_SECONDS = 120L
        val JSON = Json { isLenient = false }
        val CHALLENGE_KEYS =
            setOf("deviceId", "sessionId", "pairingGeneration", "protocol", "hostPublicKey", "nonce", "expiresAt", "hostSignature")
        val AUTHENTICATED_KEYS = setOf("type", "deviceId", "sessionId")
    }
}
