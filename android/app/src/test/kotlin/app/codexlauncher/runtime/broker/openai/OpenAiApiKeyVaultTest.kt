// Gate: importers=OpenAiApiKeyVault; callers=unit test + envelope import path;
// API=put/has/withKey never exposing key via prefs dump, mirrors MapsApiKeyVaultTest;
// schemas=sealed key bytes; user: "Workstream A1 on-device OpenAI loopback broker"
package app.codexlauncher.runtime.broker.openai

import app.codexlauncher.runtime.broker.openai.vault.OpenAiApiKeyVault
import app.codexlauncher.runtime.broker.maps.vault.SecretBox
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

class OpenAiApiKeyVaultTest {
    @Test
    fun storesAndReleasesKeyOnlyInsideWithKey() {
        val memory = linkedMapOf<String, String>()
        val vault =
            OpenAiApiKeyVault(
                box = AesGcmTestBox(),
                read = { memory[it] },
                write = { k, v -> memory[k] = v },
                clear = { memory.remove(it) },
            )
        assertFalse(vault.hasKey())
        vault.putApiKey("sk-OpenAiVaultUnitTestKey000000000001")
        assertTrue(vault.hasKey())
        val seen = vault.withKey { String(it, Charsets.UTF_8) }
        assertEquals("sk-OpenAiVaultUnitTestKey000000000001", seen)
        assertFalse(memory.values.any { it.contains("sk-OpenAi") })
    }

    private class AesGcmTestBox : SecretBox {
        private val key = ByteArray(32).also { SecureRandom().nextBytes(it) }

        override fun seal(plaintext: ByteArray): ByteArray {
            val iv = ByteArray(12).also { SecureRandom().nextBytes(it) }
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, iv))
            return iv + cipher.doFinal(plaintext)
        }

        override fun open(blob: ByteArray): ByteArray {
            val iv = blob.copyOfRange(0, 12)
            val ct = blob.copyOfRange(12, blob.size)
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, iv))
            return cipher.doFinal(ct)
        }
    }
}
