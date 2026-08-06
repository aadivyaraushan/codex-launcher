// Gate: importers=MapsPlatformClient + envelope import activity; callers=
// live Maps broker path; API=putApiKey/hasKey/withKey + SecretBox; schemas=
// AES-GCM sealed key in prefs (no plaintext); user: "Keystore-backed key /
// envelope import"
package app.codexlauncher.runtime.broker.maps.vault

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import app.codexlauncher.diagnostics.AppLog
import java.security.KeyStore
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

interface SecretBox {
    fun seal(plaintext: ByteArray): ByteArray

    fun open(blob: ByteArray): ByteArray
}

// Fact-force (edit):
// 1) Callers: MapsBrokerLoopback (maps+openai vaults), MapsImportSession.importSealedJson,
//    LiveMapsBrokerProofTest, MapsApiKeyVaultTest. New androidOpenAi() for OpenAI broker.
// 2) Search: androidOpenAi / openai_api_key_sealed → none; only maps PREF_KEY today.
// 3) Prefs file maps_broker; keys maps_api_key_sealed + openai_api_key_sealed (Base64 AES-GCM).
// 4) User: "Continue implementing the PASSed plan at planning/openai-beeper-phone-runtime-plan.md"
class MapsApiKeyVault(
    private val box: SecretBox,
    private val read: (String) -> String?,
    private val write: (String, String) -> Unit,
    private val clear: (String) -> Unit,
    private val prefKey: String = PREF_KEY,
    private val logFeature: String = "maps-broker",
) {
    fun hasKey(): Boolean = !read(prefKey).isNullOrBlank()

    fun putApiKey(apiKey: String) {
        val plain = apiKey.toByteArray(Charsets.UTF_8)
        val sealed = box.seal(plain)
        plain.fill(0)
        write(prefKey, Base64.getEncoder().encodeToString(sealed))
        AppLog.info(
            feature = logFeature,
            message = "api key sealed into vault",
            fields = mapOf("sealed_len" to sealed.size.toString(), "pref_key" to prefKey),
        )
    }

    fun clearKey() {
        clear(prefKey)
    }

    fun <T> withKey(block: (ByteArray) -> T): T {
        val encoded = read(prefKey) ?: error("api_key_missing")
        val sealed = Base64.getDecoder().decode(encoded)
        val plain = box.open(sealed)
        return try {
            block(plain)
        } finally {
            plain.fill(0)
        }
    }

    companion object {
        const val PREFS = "maps_broker"
        const val PREF_KEY = "maps_api_key_sealed"
        const val OPENAI_PREF_KEY = "openai_api_key_sealed"
        private const val KEYSTORE_ALIAS = "maps-api-key-wrap-v1"
        private const val OPENAI_KEYSTORE_ALIAS = "openai-api-key-wrap-v1"

        fun android(context: Context): MapsApiKeyVault = androidForProvider(context, Provider.MAPS)

        fun androidOpenAi(context: Context): MapsApiKeyVault = androidForProvider(context, Provider.OPENAI)

        private enum class Provider {
            MAPS,
            OPENAI,
        }

        private fun androidForProvider(context: Context, provider: Provider): MapsApiKeyVault {
            val prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            val (alias, prefKey, feature) =
                when (provider) {
                    Provider.MAPS -> Triple(KEYSTORE_ALIAS, PREF_KEY, "maps-broker")
                    Provider.OPENAI -> Triple(OPENAI_KEYSTORE_ALIAS, OPENAI_PREF_KEY, "openai-broker")
                }
            return MapsApiKeyVault(
                box = AndroidKeystoreAesGcmBox(alias),
                read = { prefs.getString(it, null) },
                write = { k, v ->
                    if (!prefs.edit().putString(k, v).commit()) {
                        error("vault_prefs_not_persisted")
                    }
                },
                clear = {
                    if (!prefs.edit().remove(it).commit()) {
                        error("vault_prefs_clear_failed")
                    }
                },
                prefKey = prefKey,
                logFeature = feature,
            )
        }
    }
}

class AndroidKeystoreAesGcmBox(
    private val alias: String,
) : SecretBox {
    override fun seal(plaintext: ByteArray): ByteArray {
        val key = secretKey()
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key)
        val iv = cipher.iv
        return iv + cipher.doFinal(plaintext)
    }

    override fun open(blob: ByteArray): ByteArray {
        val key = secretKey()
        val iv = blob.copyOfRange(0, 12)
        val ct = blob.copyOfRange(12, blob.size)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, iv))
        return cipher.doFinal(ct)
    }

    private fun secretKey(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        val existing = ks.getKey(alias, null) as? SecretKey
        if (existing != null) return existing
        val kg = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        kg.init(
            KeyGenParameterSpec.Builder(
                alias,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return kg.generateKey()
    }
}
