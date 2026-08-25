// Gate: importers=live Maps proof + Mac maps-envelope-seal.py; callers=
// instrumentation / import path; API=Keystore ECDH offer + envelope import into
// MapsApiKeyVault; schemas=offer JSON + sealed envelope JSON; user: "Import key
// from Mac .env onto Android via the allowed envelope path only"
package app.codexlauncher.runtime.broker.maps.vault

import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.broker.maps.envelope.MapsCredentialEnvelope
import app.codexlauncher.runtime.broker.maps.envelope.MapsEnvelopeMeta
import app.codexlauncher.runtime.broker.maps.envelope.SealedMapsEnvelope
import app.codexlauncher.runtime.broker.openai.vault.OpenAiApiKeyVault
import org.json.JSONObject
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.PrivateKey
import java.security.cert.X509Certificate
import java.security.spec.ECGenParameterSpec
import java.util.Base64
import java.util.UUID

data class MapsImportOffer(
    val importId: String,
    val provider: String,
    val expiresAtUnix: Long,
    val deviceSerial: String,
    val packageName: String,
    val signingDigestSha256: String,
    val devicePublicKeySpkiB64: String,
)

object MapsImportSession {
    private const val ALIAS_PREFIX = "maps-import-ecdh-"

    fun createOffer(
        context: Context,
        deviceSerial: String,
        provider: String = "maps",
        nowUnix: Long = System.currentTimeMillis() / 1000,
        ttlSeconds: Long = 600,
    ): MapsImportOffer {
        val importId = "imp-" + UUID.randomUUID().toString()
        val alias = ALIAS_PREFIX + importId
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        if (ks.containsAlias(alias)) ks.deleteEntry(alias)
        val kpg = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore")
        kpg.initialize(
            KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_AGREE_KEY)
                .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                .setDigests(KeyProperties.DIGEST_SHA256)
                .build(),
        )
        val pair = kpg.generateKeyPair()
        val digest = signingCertSha256Hex(context)
        val offer =
            MapsImportOffer(
                importId = importId,
                provider = provider,
                expiresAtUnix = nowUnix + ttlSeconds,
                deviceSerial = deviceSerial,
                packageName = context.packageName,
                signingDigestSha256 = digest,
                devicePublicKeySpkiB64 =
                    Base64.getUrlEncoder().withoutPadding().encodeToString(pair.public.encoded),
            )
        val saved =
            context.getSharedPreferences(MapsApiKeyVault.PREFS, Context.MODE_PRIVATE)
                .edit()
                .putString("pending_import_id", importId)
                .putString("pending_import_alias", alias)
                .putString("pending_provider", provider)
                .putLong("pending_import_expires", offer.expiresAtUnix)
                .putString("pending_device_serial", deviceSerial)
                .putString("pending_signing_digest", digest)
                .commit()
        if (!saved) error("maps_import_offer_prefs_not_persisted")
        AppLog.info(
            feature = "maps-broker",
            message = "maps import offer created",
            fields = mapOf("import_id" to importId, "provider" to provider, "expires_at" to offer.expiresAtUnix.toString()),
        )
        return offer
    }

    fun offerToJson(offer: MapsImportOffer): String =
        JSONObject()
            .put("v", 1)
            .put("provider", offer.provider)
            .put("importId", offer.importId)
            .put("expiresAtUnix", offer.expiresAtUnix)
            .put("deviceSerial", offer.deviceSerial)
            .put("packageName", offer.packageName)
            .put("signingDigestSha256", offer.signingDigestSha256)
            .put("devicePublicKeySpkiB64", offer.devicePublicKeySpkiB64)
            .toString()

    fun importSealedJson(
        context: Context,
        sealedJson: String,
        nowUnix: Long = System.currentTimeMillis() / 1000,
    ) {
        val root = JSONObject(sealedJson)
        val prefs = context.getSharedPreferences(MapsApiKeyVault.PREFS, Context.MODE_PRIVATE)
        val importId = root.getString("importId")
        val pendingId = prefs.getString("pending_import_id", null)
        val alias = prefs.getString("pending_import_alias", null)
        if (pendingId == null || alias == null || pendingId != importId) {
            error("envelope_import_id_mismatch")
        }
        val meta =
            MapsEnvelopeMeta(
                importId = importId,
                provider = root.getString("provider"),
                expiresAtUnix = root.getLong("expiresAtUnix"),
                deviceSerial = root.getString("deviceSerial"),
                packageName = root.getString("packageName"),
                signingDigestSha256 = root.getString("signingDigestSha256"),
            )
        // The provider to expect (and to route the decrypted key into) comes
        // from THIS device's own pending-offer state, never from the sealed
        // JSON's own claim -- otherwise a forged envelope could pick its own
        // provider and land wherever it wants.
        val pendingProvider = prefs.getString("pending_provider", "maps") ?: "maps"
        val expected =
            MapsEnvelopeMeta(
                importId = importId,
                provider = pendingProvider,
                expiresAtUnix = meta.expiresAtUnix,
                deviceSerial = prefs.getString("pending_device_serial", "") ?: "",
                packageName = context.packageName,
                signingDigestSha256 = prefs.getString("pending_signing_digest", "") ?: "",
            )
        val sealed =
            SealedMapsEnvelope(
                meta = meta,
                helperEphemeralPublicKey =
                    Base64.getUrlDecoder().decode(root.getString("helperEphemeralPublicKeyB64")),
                nonce = Base64.getUrlDecoder().decode(root.getString("nonceB64")),
                ciphertext = Base64.getUrlDecoder().decode(root.getString("ciphertextB64")),
            )
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        val privateKey = ks.getKey(alias, null) as? PrivateKey ?: error("import_private_key_missing")
        val apiKeyBytes = MapsCredentialEnvelope.open(privateKey, sealed, expected, nowUnix)
        val apiKey = String(apiKeyBytes, Charsets.UTF_8)
        apiKeyBytes.fill(0)
        when (pendingProvider) {
            "maps" -> MapsApiKeyVault.android(context).putApiKey(apiKey)
            "openai" -> OpenAiApiKeyVault.android(context).putApiKey(apiKey)
            else -> error("unsupported_import_provider")
        }
        prefs.edit()
            .remove("pending_import_id")
            .remove("pending_import_alias")
            .remove("pending_import_expires")
            .remove("pending_provider")
            .remove("pending_device_serial")
            .remove("pending_signing_digest")
            .commit()
        runCatching { ks.deleteEntry(alias) }
        AppLog.info(
            feature = "maps-broker",
            message = "envelope imported into vault",
            fields = mapOf("import_id" to importId, "provider" to pendingProvider, "has_key" to "true"),
        )
    }

    fun signingCertSha1Hex(context: Context): String {
        val sigs = signingCerts(context)
        val md = java.security.MessageDigest.getInstance("SHA-1")
        return md.digest(sigs[0].encoded).joinToString("") { "%02x".format(it) }
    }

    fun signingCertSha256Hex(context: Context): String {
        val sigs = signingCerts(context)
        val md = java.security.MessageDigest.getInstance("SHA-256")
        return md.digest(sigs[0].encoded).joinToString("") { "%02x".format(it) }
    }

    private fun signingCerts(context: Context): Array<X509Certificate> {
        val pm = context.packageManager
        val info =
            if (Build.VERSION.SDK_INT >= 33) {
                pm.getPackageInfo(
                    context.packageName,
                    PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES.toLong()),
                )
            } else {
                @Suppress("DEPRECATION")
                pm.getPackageInfo(context.packageName, PackageManager.GET_SIGNING_CERTIFICATES)
            }
        val signers = info.signingInfo?.apkContentsSigners ?: error("missing_signing_info")
        return Array(signers.size) { i ->
            val factory = java.security.cert.CertificateFactory.getInstance("X.509")
            factory.generateCertificate(signers[i].toByteArray().inputStream()) as X509Certificate
        }
    }
}
