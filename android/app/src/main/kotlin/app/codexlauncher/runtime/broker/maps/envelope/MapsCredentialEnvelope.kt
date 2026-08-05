// Gate: importers=MapsApiKeyVault + Mac maps-envelope-seal helper; callers=
// Android envelope import + JVM unit tests; API=ECDH P-256 + HKDF-SHA256 +
// AES-256-GCM; schemas=MapsEnvelopeMeta/SealedMapsEnvelope; user: "Import key
// from Mac .env onto Android via the allowed envelope path only"
package app.codexlauncher.runtime.broker.maps.envelope

import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.MessageDigest
import java.security.PrivateKey
import java.security.PublicKey
import java.security.SecureRandom
import java.security.spec.ECGenParameterSpec
import java.security.spec.X509EncodedKeySpec
import javax.crypto.Cipher
import javax.crypto.KeyAgreement
import javax.crypto.Mac
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

data class MapsEnvelopeMeta(
    val importId: String,
    val provider: String,
    val expiresAtUnix: Long,
    val deviceSerial: String,
    val packageName: String,
    val signingDigestSha256: String,
) {
    fun aadUtf8(): ByteArray =
        listOf(
            "v=1",
            "importId=$importId",
            "provider=$provider",
            "expiresAtUnix=$expiresAtUnix",
            "deviceSerial=$deviceSerial",
            "packageName=$packageName",
            "signingDigestSha256=$signingDigestSha256",
        ).joinToString("\n").toByteArray(Charsets.UTF_8)
}

data class SealedMapsEnvelope(
    val meta: MapsEnvelopeMeta,
    val helperEphemeralPublicKey: ByteArray,
    val nonce: ByteArray,
    val ciphertext: ByteArray,
)

object MapsCredentialEnvelope {
    private const val INFO = "operator-maps-key-import-v1"

    fun seal(
        devicePublicKey: PublicKey,
        meta: MapsEnvelopeMeta,
        apiKeyUtf8: ByteArray,
    ): SealedMapsEnvelope {
        val helper =
            KeyPairGenerator.getInstance("EC").apply {
                initialize(ECGenParameterSpec("secp256r1"))
            }.generateKeyPair()
        val shared = ecdh(helper.private, devicePublicKey)
        val aesKey = hkdfSha256(shared, meta.importId.toByteArray(Charsets.UTF_8), 32)
        val nonce = ByteArray(12).also { SecureRandom().nextBytes(it) }
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(aesKey, "AES"), GCMParameterSpec(128, nonce))
        cipher.updateAAD(meta.aadUtf8())
        val ciphertext = cipher.doFinal(apiKeyUtf8)
        shared.fill(0)
        aesKey.fill(0)
        return SealedMapsEnvelope(
            meta = meta,
            helperEphemeralPublicKey = helper.public.encoded,
            nonce = nonce,
            ciphertext = ciphertext,
        )
    }

    fun open(
        devicePrivateKey: PrivateKey,
        sealed: SealedMapsEnvelope,
        expected: MapsEnvelopeMeta,
        nowUnix: Long,
    ): ByteArray {
        if (nowUnix > expected.expiresAtUnix) {
            error("envelope_expired")
        }
        if (sealed.meta != expected) {
            error("envelope_aad_mismatch")
        }
        val helperPublic =
            KeyFactory.getInstance("EC").generatePublic(X509EncodedKeySpec(sealed.helperEphemeralPublicKey))
        val shared = ecdh(devicePrivateKey, helperPublic)
        val aesKey = hkdfSha256(shared, expected.importId.toByteArray(Charsets.UTF_8), 32)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(aesKey, "AES"), GCMParameterSpec(128, sealed.nonce))
        cipher.updateAAD(expected.aadUtf8())
        return try {
            cipher.doFinal(sealed.ciphertext)
        } catch (_: Exception) {
            error("envelope_decrypt_failed")
        } finally {
            shared.fill(0)
            aesKey.fill(0)
        }
    }

    private fun ecdh(privateKey: PrivateKey, publicKey: PublicKey): ByteArray {
        val ka = KeyAgreement.getInstance("ECDH")
        ka.init(privateKey)
        ka.doPhase(publicKey, true)
        return ka.generateSecret()
    }

    private fun hkdfSha256(ikm: ByteArray, salt: ByteArray, length: Int): ByteArray {
        val mac = Mac.getInstance("HmacSHA256")
        val zeroSalt = if (salt.isEmpty()) ByteArray(32) else salt
        mac.init(SecretKeySpec(zeroSalt, "HmacSHA256"))
        val prk = mac.doFinal(ikm)
        mac.init(SecretKeySpec(prk, "HmacSHA256"))
        mac.update(INFO.toByteArray(Charsets.UTF_8))
        mac.update(0x01)
        val okm = mac.doFinal()
        prk.fill(0)
        return okm.copyOf(length).also { okm.fill(0) }
    }

    fun sha256Hex(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { b ->
            "%02x".format(b)
        }
}
