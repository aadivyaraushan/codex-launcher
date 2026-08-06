package app.codexlauncher.connection.pairing.network

import app.codexlauncher.connection.pairing.model.PairingOffer
import app.codexlauncher.connection.pairing.model.PairingValidation
import app.codexlauncher.connection.pairing.model.PairingWire
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.storage.secrets.PairingKeyProtection
import app.codexlauncher.storage.secrets.PairingKeyStore
import app.codexlauncher.storage.secrets.PairingPublicKey
import java.util.Base64

interface DevicePairingSigner {
    fun loadOrCreate(): PairingPublicKey

    fun sign(message: ByteArray): ByteArray
}

class AndroidDevicePairingSigner(
    private val keyStore: PairingKeyStore,
) : DevicePairingSigner {
    override fun loadOrCreate(): PairingPublicKey = keyStore.loadOrCreate()

    override fun sign(message: ByteArray): ByteArray = keyStore.sign(message)
}

interface PairingTransport {
    fun pair(offer: PairingOffer, request: PairingRequest): PairingResponse
}

class PairingRequest(
    val secret: String,
    val host: String,
    val port: Int,
    val protocol: Int,
    val hostIdentity: String,
    val deviceId: String,
    val deviceName: String,
    val devicePublicKey: String,
    val signature: String,
) {
    override fun toString(): String =
        "PairingRequest(host=$host, port=$port, protocol=$protocol, deviceId=$deviceId, secret=[redacted], key=[redacted], signature=[redacted])"
}

data class PairingResponse(
    val deviceId: String,
    val pairingGeneration: String,
)

data class PairedComputer(
    val host: String,
    val port: Int,
    val protocol: Int,
    val hostIdentity: String,
    val tlsIdentity: String,
    val deviceId: String,
    val deviceName: String,
    val pairingGeneration: String,
    val keyProtection: PairingKeyProtection,
)

class PairingException(
    message: String,
    cause: Throwable? = null,
) : Exception(message, cause)

class PairingClient(
    private val signer: DevicePairingSigner,
    private val transport: PairingTransport,
) {
    // Callers: LauncherActivity PairingViewModel (encoded); LocalPairHandshake (forLocalRuntime offer).
    // Affected API: pair(PairingOffer) overload for loopback enrollment without public QR parse.
    // User: "Reuse existing local-pair TLS/auth ... do not invent a second auth scheme."
    fun pair(
        encodedOffer: String,
        deviceId: String,
        deviceName: String,
    ): PairedComputer = pair(PairingOffer.parse(encodedOffer), deviceId, deviceName)

    fun pair(
        offer: PairingOffer,
        deviceId: String,
        deviceName: String,
    ): PairedComputer {
        require(PairingValidation.isSafeIdentifier(deviceId)) { "Invalid device identifier" }
        require(PairingValidation.isSafeDeviceName(deviceName)) { "Invalid device name" }
        AppLog.info(
            feature = "pairing",
            message = "pairing requested",
            fields = mapOf("input_shape" to "validated_offer,p256_device_key", "device_id" to deviceId),
        )
        try {
            val publicKey = signer.loadOrCreate()
            if (publicKey.algorithm != "EC") throw PairingException("Pairing key algorithm is unavailable")
            val encodedPublicKey = Base64.getUrlEncoder().withoutPadding().encodeToString(publicKey.publicKey)
            val proof =
                PairingWire.pairingProofMessage(
                    secret = offer.secret,
                    host = offer.host,
                    port = offer.port,
                    protocol = offer.protocol,
                    hostIdentity = offer.hostIdentity,
                    deviceId = deviceId,
                    deviceName = deviceName,
                    devicePublicKey = encodedPublicKey,
                )
            val request =
                PairingRequest(
                    secret = offer.secret,
                    host = offer.host,
                    port = offer.port,
                    protocol = offer.protocol,
                    hostIdentity = offer.hostIdentity,
                    deviceId = deviceId,
                    deviceName = deviceName,
                    devicePublicKey = encodedPublicKey,
                    signature = Base64.getUrlEncoder().withoutPadding().encodeToString(signer.sign(proof)),
                )
            val response = transport.pair(offer, request)
            if (response.deviceId != deviceId || !PairingValidation.isCanonicalBase64Url(response.pairingGeneration, 16)) {
                throw PairingException("Pairing response did not match the device")
            }
            AppLog.info(
                feature = "pairing",
                message = "pairing completed",
                fields =
                    mapOf(
                        "device_id" to deviceId,
                        "output_shape" to "paired_computer",
                        "protection" to publicKey.protection,
                    ),
            )
            return PairedComputer(
                host = offer.host,
                port = offer.port,
                protocol = offer.protocol,
                hostIdentity = offer.hostIdentity,
                tlsIdentity = offer.tlsIdentity,
                deviceId = deviceId,
                deviceName = deviceName,
                pairingGeneration = response.pairingGeneration,
                keyProtection = publicKey.protection,
            )
        } catch (error: Exception) {
            AppLog.error(
                feature = "pairing",
                message = "pairing failed",
                error = error,
                fields = mapOf("device_id" to deviceId, "decision" to "fail_closed"),
            )
            if (error is PairingException) throw error
            throw PairingException("Pairing failed", error)
        }
    }

}
