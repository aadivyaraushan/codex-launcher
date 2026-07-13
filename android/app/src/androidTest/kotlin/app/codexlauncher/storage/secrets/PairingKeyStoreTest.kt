package app.codexlauncher.storage.secrets

import android.os.Build
import android.security.keystore.KeyInfo
import android.security.keystore.KeyProperties
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.security.KeyFactory
import java.security.KeyStore
import java.security.Signature
import java.security.spec.X509EncodedKeySpec

@RunWith(AndroidJUnit4::class)
class PairingKeyStoreTest {
    private val store = PairingKeyStore()

    @Before
    fun clearBeforeTest() {
        store.delete()
    }

    @After
    fun clearAfterTest() {
        store.delete()
    }

    @Test
    fun generatesAStableNonExportableP256SigningKeyAndReportsItsProtection() {
        val first = store.loadOrCreate()
        val second = store.loadOrCreate()
        assertArrayEquals(first.publicKey, second.publicKey)
        assertEquals("EC", first.algorithm)
        assertTrue(store.exists())

        val message = "pairing proof".encodeToByteArray()
        val signature = store.sign(message)
        val publicKey = KeyFactory.getInstance("EC").generatePublic(X509EncodedKeySpec(first.publicKey))
        val verifier = Signature.getInstance("SHA256withECDSA")
        verifier.initVerify(publicKey)
        verifier.update(message)
        assertTrue(verifier.verify(signature))

        val androidKeyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        val privateKey = androidKeyStore.getKey(PAIRING_KEY_ALIAS, null)
        assertNull("Android Keystore private key material must not be exportable", privateKey.encoded)
        val keyInfo =
            KeyFactory
                .getInstance(privateKey.algorithm, "AndroidKeyStore")
                .getKeySpec(privateKey, KeyInfo::class.java)
        assertEquals(keyInfo.securityLevel, first.securityLevel)
        assertEquals(
            keyInfo.securityLevel in
                setOf(
                    KeyProperties.SECURITY_LEVEL_TRUSTED_ENVIRONMENT,
                    KeyProperties.SECURITY_LEVEL_STRONGBOX,
                    KeyProperties.SECURITY_LEVEL_UNKNOWN_SECURE,
                ),
            first.protection == PairingKeyProtection.HARDWARE_BACKED,
        )

        val isPhysicalPixel9 = Build.MODEL.contains("Pixel 9") && !Build.FINGERPRINT.contains("generic")
        if (isPhysicalPixel9) {
            assertEquals(PairingKeyProtection.HARDWARE_BACKED, first.protection)
        }
    }

    @Test
    fun deletingTheKeyMakesThePairingIdentityUnavailable() {
        store.loadOrCreate()
        store.delete()

        assertFalse(store.exists())
        assertThrows(IllegalStateException::class.java) {
            store.sign("session proof".encodeToByteArray())
        }
    }
}
