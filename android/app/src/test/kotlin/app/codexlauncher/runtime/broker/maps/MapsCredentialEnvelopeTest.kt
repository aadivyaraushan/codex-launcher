// Gate: importers=MapsCredentialEnvelope (Android maps broker); callers=unit test +
// MapsApiKeyVault.importEnvelope; API=ECDH+HKDF+AES-GCM seal/open;
// schemas=MapsEnvelopeMeta + SealedMapsEnvelope; user: "Implement the minimal
// Android Maps broker path... Keystore-backed key / envelope import + Places/Routes"
package app.codexlauncher.runtime.broker.maps

import app.codexlauncher.runtime.broker.maps.envelope.MapsCredentialEnvelope
import app.codexlauncher.runtime.broker.maps.envelope.MapsEnvelopeMeta
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.KeyPairGenerator
import java.security.spec.ECGenParameterSpec

class MapsCredentialEnvelopeTest {
    @Test
    fun roundTripsApiKeyThroughEcdhHkdfAesGcm() {
        val device = KeyPairGenerator.getInstance("EC").apply {
            initialize(ECGenParameterSpec("secp256r1"))
        }.generateKeyPair()
        val meta =
            MapsEnvelopeMeta(
                importId = "imp-test-1",
                provider = "maps",
                expiresAtUnix = 4_102_444_800L,
                deviceSerial = "4B230DLAQ001Z5",
                packageName = "app.codexlauncher",
                signingDigestSha256 = "c613e6607c404042e6913517278638946b29c18dc1cd81e92c9d3699a580c25d",
            )
        val apiKey = "AIzaSyCanaryMapsKeyForUnitTestOnly0001".toByteArray(Charsets.UTF_8)
        val sealed =
            MapsCredentialEnvelope.seal(
                devicePublicKey = device.public,
                meta = meta,
                apiKeyUtf8 = apiKey,
            )
        assertTrue(sealed.ciphertext.isNotEmpty())
        assertTrue(sealed.helperEphemeralPublicKey.isNotEmpty())
        val opened =
            MapsCredentialEnvelope.open(
                devicePrivateKey = device.private,
                sealed = sealed,
                expected =
                    MapsEnvelopeMeta(
                        importId = meta.importId,
                        provider = "maps",
                        expiresAtUnix = meta.expiresAtUnix,
                        deviceSerial = meta.deviceSerial,
                        packageName = meta.packageName,
                        signingDigestSha256 = meta.signingDigestSha256,
                    ),
                nowUnix = 1_775_000_000L,
            )
        assertArrayEquals(apiKey, opened)
    }

    @Test
    fun rejectsExpiredEnvelope() {
        val device = KeyPairGenerator.getInstance("EC").apply {
            initialize(ECGenParameterSpec("secp256r1"))
        }.generateKeyPair()
        val meta =
            MapsEnvelopeMeta(
                importId = "imp-expired",
                provider = "maps",
                expiresAtUnix = 100L,
                deviceSerial = "4B230DLAQ001Z5",
                packageName = "app.codexlauncher",
                signingDigestSha256 = "aa",
            )
        val sealed =
            MapsCredentialEnvelope.seal(
                devicePublicKey = device.public,
                meta = meta,
                apiKeyUtf8 = "x".toByteArray(),
            )
        val err =
            runCatching {
                MapsCredentialEnvelope.open(
                    devicePrivateKey = device.private,
                    sealed = sealed,
                    expected = meta,
                    nowUnix = 200L,
                )
            }.exceptionOrNull()
        assertEquals("envelope_expired", err?.message)
    }

    @Test
    fun rejectsWrongProviderBinding() {
        val device = KeyPairGenerator.getInstance("EC").apply {
            initialize(ECGenParameterSpec("secp256r1"))
        }.generateKeyPair()
        val meta =
            MapsEnvelopeMeta(
                importId = "imp-provider",
                provider = "maps",
                expiresAtUnix = 4_102_444_800L,
                deviceSerial = "4B230DLAQ001Z5",
                packageName = "app.codexlauncher",
                signingDigestSha256 = "aa",
            )
        val sealed =
            MapsCredentialEnvelope.seal(
                devicePublicKey = device.public,
                meta = meta,
                apiKeyUtf8 = "x".toByteArray(),
            )
        val wrong = meta.copy(provider = "youtube")
        val err =
            runCatching {
                MapsCredentialEnvelope.open(
                    devicePrivateKey = device.private,
                    sealed = sealed,
                    expected = wrong,
                    nowUnix = 1L,
                )
            }.exceptionOrNull()
        assertEquals("envelope_aad_mismatch", err?.message)
    }
}
