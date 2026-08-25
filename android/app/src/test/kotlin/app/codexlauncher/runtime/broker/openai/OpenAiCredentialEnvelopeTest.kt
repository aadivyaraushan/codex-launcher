// Gate: importers=MapsCredentialEnvelope (shared, provider-agnostic seal/open);
// callers=unit test only; API=same ECDH+HKDF+AES-GCM path proven generic across
// providers; schemas=MapsEnvelopeMeta with provider="openai"; user: "Workstream A1
// on-device OpenAI loopback broker, mirror the maps broker one-for-one"
package app.codexlauncher.runtime.broker.openai

import app.codexlauncher.runtime.broker.maps.envelope.MapsCredentialEnvelope
import app.codexlauncher.runtime.broker.maps.envelope.MapsEnvelopeMeta
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Test
import java.security.KeyPairGenerator
import java.security.spec.ECGenParameterSpec

class OpenAiCredentialEnvelopeTest {
    @Test
    fun roundTripsApiKeyThroughEnvelopeForOpenAiProvider() {
        val device = KeyPairGenerator.getInstance("EC").apply {
            initialize(ECGenParameterSpec("secp256r1"))
        }.generateKeyPair()
        val meta =
            MapsEnvelopeMeta(
                importId = "imp-openai-1",
                provider = "openai",
                expiresAtUnix = 4_102_444_800L,
                deviceSerial = "4B230DLAQ001Z5",
                packageName = "app.codexlauncher",
                signingDigestSha256 = "c613e6607c404042e6913517278638946b29c18dc1cd81e92c9d3699a580c25d",
            )
        val apiKey = "sk-CanaryOpenAiKeyForUnitTestOnly0001".toByteArray(Charsets.UTF_8)
        val sealed =
            MapsCredentialEnvelope.seal(
                devicePublicKey = device.public,
                meta = meta,
                apiKeyUtf8 = apiKey,
            )
        val opened =
            MapsCredentialEnvelope.open(
                devicePrivateKey = device.private,
                sealed = sealed,
                expected = meta,
                nowUnix = 1_775_000_000L,
            )
        assertArrayEquals(apiKey, opened)
    }

    @Test
    fun rejectsEnvelopeSealedForWrongProvider() {
        val device = KeyPairGenerator.getInstance("EC").apply {
            initialize(ECGenParameterSpec("secp256r1"))
        }.generateKeyPair()
        val meta =
            MapsEnvelopeMeta(
                importId = "imp-openai-2",
                provider = "openai",
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
        val expectedMaps = meta.copy(provider = "maps")
        val err =
            runCatching {
                MapsCredentialEnvelope.open(
                    devicePrivateKey = device.private,
                    sealed = sealed,
                    expected = expectedMaps,
                    nowUnix = 1L,
                )
            }.exceptionOrNull()
        assertEquals("envelope_aad_mismatch", err?.message)
    }
}
