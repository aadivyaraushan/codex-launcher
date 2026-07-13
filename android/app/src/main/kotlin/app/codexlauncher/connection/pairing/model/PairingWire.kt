package app.codexlauncher.connection.pairing.model

import java.nio.ByteBuffer

object PairingWire {
    fun pairingProofMessage(
        secret: String,
        host: String,
        port: Int,
        protocol: Int,
        hostIdentity: String,
        deviceId: String,
        deviceName: String,
        devicePublicKey: String,
    ): ByteArray =
        signedMessage(
            "pair-v1",
            secret,
            host,
            port.toString(),
            protocol.toString(),
            hostIdentity,
            deviceId,
            deviceName,
            devicePublicKey,
        )

    fun sessionChallengeMessage(
        deviceId: String,
        sessionId: String,
        pairingGeneration: String,
        protocol: Int,
        hostIdentity: String,
        nonce: String,
        expiresAt: Long,
    ): ByteArray =
        signedMessage(
            "session-challenge-v1",
            deviceId,
            sessionId,
            pairingGeneration,
            protocol.toString(),
            hostIdentity,
            nonce,
            expiresAt.toString(),
        )

    fun sessionProofMessage(
        deviceId: String,
        sessionId: String,
        pairingGeneration: String,
        protocol: Int,
        hostIdentity: String,
        nonce: String,
        expiresAt: Long,
        hostSignature: String,
    ): ByteArray =
        signedMessage(
            "session-v1",
            deviceId,
            sessionId,
            pairingGeneration,
            protocol.toString(),
            hostIdentity,
            nonce,
            expiresAt.toString(),
            hostSignature,
        )

    private fun signedMessage(vararg values: String): ByteArray {
        val fields = values.map(String::encodeToByteArray)
        val totalSize = fields.sumOf { 4L + it.size }
        require(totalSize <= Int.MAX_VALUE) { "Signed message is too large" }
        return ByteBuffer.allocate(totalSize.toInt()).apply {
            fields.forEach { field ->
                putInt(field.size)
                put(field)
            }
        }.array()
    }
}
