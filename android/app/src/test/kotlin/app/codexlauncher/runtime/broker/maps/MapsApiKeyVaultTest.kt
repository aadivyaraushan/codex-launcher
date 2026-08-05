// Gate: importers=MapsApiKeyVault; callers=unit test + envelope import path;
// API=put/has/withKey never exposing key via prefs dump; schemas=sealed key bytes;
// user: "Implement the minimal Android Maps broker path... Keystore-backed key"
package app.codexlauncher.runtime.broker.maps

import app.codexlauncher.runtime.broker.maps.vault.MapsApiKeyVault
import app.codexlauncher.runtime.broker.maps.vault.SecretBox
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

class MapsApiKeyVaultTest {
    @Test
    fun storesAndReleasesKeyOnlyInsideWithKey() {
        val memory = linkedMapOf<String, String>()
        val vault =
            MapsApiKeyVault(
                box = AesGcmTestBox(),
                read = { memory[it] },
                write = { k, v -> memory[k] = v },
                clear = { memory.remove(it) },
            )
        assertFalse(vault.hasKey())
        vault.putApiKey("AIzaSyVaultUnitTestKey000000000000001")
        assertTrue(vault.hasKey())
        val seen = vault.withKey { String(it, Charsets.UTF_8) }
        assertEquals("AIzaSyVaultUnitTestKey000000000000001", seen)
        assertFalse(memory.values.any { it.contains("AIzaSy") })
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
