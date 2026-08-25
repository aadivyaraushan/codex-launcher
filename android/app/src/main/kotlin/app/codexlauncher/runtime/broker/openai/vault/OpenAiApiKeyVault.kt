// Gate: importers=OpenAiBrokerLoopback wiring + MapsImportSession (openai import
// path); callers=Android envelope import + JVM unit tests; API=put/hasKey/withKey,
// same shape as MapsApiKeyVault; schemas=AES-GCM sealed key in prefs (no plaintext);
// user: "Workstream A1 on-device OpenAI loopback broker, mirror the maps broker
// one-for-one"
package app.codexlauncher.runtime.broker.openai.vault

import android.content.Context
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.broker.maps.vault.AndroidKeystoreAesGcmBox
import app.codexlauncher.runtime.broker.maps.vault.MapsApiKeyVault
import app.codexlauncher.runtime.broker.maps.vault.SecretBox
import java.util.Base64

class OpenAiApiKeyVault(
    private val box: SecretBox,
    private val read: (String) -> String?,
    private val write: (String, String) -> Unit,
    private val clear: (String) -> Unit,
) {
    fun hasKey(): Boolean = !read(PREF_KEY).isNullOrBlank()

    fun putApiKey(apiKey: String) {
        val plain = apiKey.toByteArray(Charsets.UTF_8)
        val sealed = box.seal(plain)
        plain.fill(0)
        write(PREF_KEY, Base64.getEncoder().encodeToString(sealed))
        AppLog.info(
            feature = "openai-broker",
            message = "openai api key sealed into vault",
            fields = mapOf("sealed_len" to sealed.size.toString()),
        )
    }

    fun clearKey() {
        clear(PREF_KEY)
    }

    fun <T> withKey(block: (ByteArray) -> T): T {
        val encoded = read(PREF_KEY) ?: error("openai_key_missing")
        val sealed = Base64.getDecoder().decode(encoded)
        val plain = box.open(sealed)
        return try {
            block(plain)
        } finally {
            plain.fill(0)
        }
    }

    companion object {
        // Same prefs file as maps: this vault only adds one more sealed-blob
        // entry under its own key, not a second store to keep consistent.
        const val PREF_KEY = "openai_api_key_sealed"
        private const val KEYSTORE_ALIAS = "openai-api-key-wrap-v1"

        fun android(context: Context): OpenAiApiKeyVault {
            val prefs = context.applicationContext.getSharedPreferences(MapsApiKeyVault.PREFS, Context.MODE_PRIVATE)
            return OpenAiApiKeyVault(
                box = AndroidKeystoreAesGcmBox(KEYSTORE_ALIAS),
                read = { prefs.getString(it, null) },
                write = { k, v ->
                    if (!prefs.edit().putString(k, v).commit()) {
                        error("openai_vault_prefs_not_persisted")
                    }
                },
                clear = {
                    if (!prefs.edit().remove(it).commit()) {
                        error("openai_vault_prefs_clear_failed")
                    }
                },
            )
        }
    }
}
