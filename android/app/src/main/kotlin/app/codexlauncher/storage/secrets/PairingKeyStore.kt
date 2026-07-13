package app.codexlauncher.storage.secrets

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyInfo
import android.security.keystore.KeyProperties
import app.codexlauncher.diagnostics.AppLog
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.PrivateKey
import java.security.Signature
import java.security.spec.ECGenParameterSpec

internal const val PAIRING_KEY_ALIAS = "codex_launcher_pairing_signing_v1"

enum class PairingKeyProtection {
    HARDWARE_BACKED,
    SOFTWARE_BACKED,
}

data class PairingPublicKey(
    val algorithm: String,
    val publicKey: ByteArray,
    val protection: PairingKeyProtection,
    val securityLevel: Int,
)

class PairingKeyStore {
    private val keyStore: KeyStore
        get() = KeyStore.getInstance(ANDROID_KEY_STORE).apply { load(null) }

    fun exists(): Boolean = keyStore.containsAlias(PAIRING_KEY_ALIAS)

    fun loadOrCreate(): PairingPublicKey {
        val existed = exists()
        AppLog.info(
            feature = "pairing-key",
            message = "pairing signing key requested",
            fields = mapOf("input_shape" to "existing=$existed,algorithm=EC,curve=P-256"),
        )
        try {
            if (!existed) generate()
            val entry = requireEntry()
            val keyInfo = keyInfo(entry.privateKey)
            val protection = protectionFor(keyInfo.securityLevel)
            AppLog.info(
                feature = "pairing-key",
                message = "pairing signing key ready",
                fields =
                    mapOf(
                        "decision" to if (existed) "reuse" else "generate",
                        "output_shape" to "x509_public_key",
                        "protection" to protection,
                        "security_level" to keyInfo.securityLevel,
                    ),
            )
            return PairingPublicKey(
                algorithm = entry.certificate.publicKey.algorithm,
                publicKey = entry.certificate.publicKey.encoded.copyOf(),
                protection = protection,
                securityLevel = keyInfo.securityLevel,
            )
        } catch (error: Exception) {
            AppLog.error(
                feature = "pairing-key",
                message = "pairing signing key unavailable",
                error = error,
                fields = mapOf("decision" to "fail_closed"),
            )
            throw error
        }
    }

    fun sign(message: ByteArray): ByteArray {
        AppLog.info(
            feature = "pairing-key",
            message = "pairing proof signing requested",
            fields = mapOf("input_shape" to "message_bytes=${message.size}"),
        )
        try {
            val signer = Signature.getInstance(SIGNATURE_ALGORITHM)
            signer.initSign(requireEntry().privateKey)
            signer.update(message)
            val signature = signer.sign()
            AppLog.info(
                feature = "pairing-key",
                message = "pairing proof signed",
                fields = mapOf("output_shape" to "ecdsa_asn1", "signature_bytes" to signature.size),
            )
            return signature
        } catch (error: Exception) {
            AppLog.error(
                feature = "pairing-key",
                message = "pairing proof signing failed",
                error = error,
                fields = mapOf("decision" to "fail_closed"),
            )
            throw error
        }
    }

    fun delete() {
        val store = keyStore
        if (store.containsAlias(PAIRING_KEY_ALIAS)) {
            store.deleteEntry(PAIRING_KEY_ALIAS)
            AppLog.info(
                feature = "pairing-key",
                message = "pairing signing key deleted",
                fields = mapOf("output_shape" to "alias_absent"),
            )
        }
    }

    private fun generate() {
        val generator = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, ANDROID_KEY_STORE)
        val parameters =
            KeyGenParameterSpec
                .Builder(
                    PAIRING_KEY_ALIAS,
                    KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY,
                ).setAlgorithmParameterSpec(ECGenParameterSpec(P256_CURVE))
                .setDigests(KeyProperties.DIGEST_SHA256)
                .build()
        generator.initialize(parameters)
        generator.generateKeyPair()
    }

    private fun requireEntry(): KeyStore.PrivateKeyEntry =
        (keyStore.getEntry(PAIRING_KEY_ALIAS, null) as? KeyStore.PrivateKeyEntry)
            ?: throw IllegalStateException("Pairing signing key is missing")

    private fun keyInfo(privateKey: PrivateKey): KeyInfo =
        KeyFactory
            .getInstance(privateKey.algorithm, ANDROID_KEY_STORE)
            .getKeySpec(privateKey, KeyInfo::class.java)

    private fun protectionFor(securityLevel: Int): PairingKeyProtection =
        when (securityLevel) {
            KeyProperties.SECURITY_LEVEL_TRUSTED_ENVIRONMENT,
            KeyProperties.SECURITY_LEVEL_STRONGBOX,
            KeyProperties.SECURITY_LEVEL_UNKNOWN_SECURE,
            -> PairingKeyProtection.HARDWARE_BACKED
            else -> PairingKeyProtection.SOFTWARE_BACKED
        }

    private companion object {
        const val ANDROID_KEY_STORE = "AndroidKeyStore"
        const val P256_CURVE = "secp256r1"
        const val SIGNATURE_ALGORITHM = "SHA256withECDSA"
    }
}
