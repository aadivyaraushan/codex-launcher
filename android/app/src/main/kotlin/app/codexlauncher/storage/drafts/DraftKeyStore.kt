package app.codexlauncher.storage.drafts

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

internal const val DRAFT_KEY_ALIAS = "codex_launcher_unfinished_draft_aes_v1"

internal data class EncryptedDraftPayload(
    val initializationVector: ByteArray,
    val ciphertext: ByteArray,
)

class DraftKeyStore {
    private val keyStore: KeyStore
        get() = KeyStore.getInstance(ANDROID_KEY_STORE).apply { load(null) }

    fun exists(): Boolean = keyStore.containsAlias(DRAFT_KEY_ALIAS)

    internal fun encrypt(plaintext: ByteArray, associatedData: ByteArray): EncryptedDraftPayload {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, loadOrCreate())
        cipher.updateAAD(associatedData)
        return EncryptedDraftPayload(
            initializationVector = cipher.iv.copyOf(),
            ciphertext = cipher.doFinal(plaintext),
        )
    }

    internal fun decrypt(payload: EncryptedDraftPayload, associatedData: ByteArray): ByteArray {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.DECRYPT_MODE, requireKey(), GCMParameterSpec(GCM_TAG_BITS, payload.initializationVector))
        cipher.updateAAD(associatedData)
        return cipher.doFinal(payload.ciphertext)
    }

    fun delete() {
        val store = keyStore
        if (store.containsAlias(DRAFT_KEY_ALIAS)) store.deleteEntry(DRAFT_KEY_ALIAS)
    }

    @Synchronized
    private fun loadOrCreate(): SecretKey {
        (keyStore.getKey(DRAFT_KEY_ALIAS, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEY_STORE)
        generator.init(
            KeyGenParameterSpec
                .Builder(
                    DRAFT_KEY_ALIAS,
                    KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
                ).setKeySize(AES_KEY_BITS)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setRandomizedEncryptionRequired(true)
                .build(),
        )
        return generator.generateKey()
    }

    private fun requireKey(): SecretKey =
        (keyStore.getKey(DRAFT_KEY_ALIAS, null) as? SecretKey)
            ?: throw IllegalStateException("Draft encryption key is missing")

    private companion object {
        const val ANDROID_KEY_STORE = "AndroidKeyStore"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val AES_KEY_BITS = 256
        const val GCM_TAG_BITS = 128
    }
}
