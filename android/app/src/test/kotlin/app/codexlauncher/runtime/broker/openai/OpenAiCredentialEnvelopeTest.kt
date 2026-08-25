// Fact-force:
// 1) Callers: JUnit (this file); production importers=MapsImportSession once
//    provider=openai is accepted; mirrors MapsCredentialEnvelopeTest.kt.
// 2) find android -iname '*openai*Credential*' → none; only OpenAIAccountLock/
//    UsageCeiling (unrelated OAuth ceiling). Envelope crypto already shared.
// 3) No data files; synthetic meta + apiKey bytes only (no production secrets).
// 4) User: "Implement the judge-PASSed plan at planning/openai-beeper-phone-runtime-plan.md"
//    (A1: envelope round-trip with provider openai, wrong-provider rejection)
package app.codexlauncher.runtime.broker.openai

import app.codexlauncher.runtime.broker.maps.envelope.MapsCredentialEnvelope
import app.codexlauncher.runtime.broker.maps.envelope.MapsEnvelopeMeta
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.KeyPairGenerator
import java.security.spec.ECGenParameterSpec

class OpenAiCredentialEnvelopeTest {
    @Test
    fun roundTripsOpenAiApiKeyThroughEnvelope() {
        val device =
            KeyPairGenerator.getInstance("EC").apply {
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
        val apiKey = "sk-test-openai-key-for-unit-only-0001".toByteArray(Charsets.UTF_8)
        val sealed =
            MapsCredentialEnvelope.seal(
                devicePublicKey = device.public,
                meta = meta,
                apiKeyUtf8 = apiKey,
            )
        assertTrue(sealed.ciphertext.isNotEmpty())
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
    fun rejectsMapsEnvelopeWhenOpenAiProviderExpected() {
        val device =
            KeyPairGenerator.getInstance("EC").apply {
                initialize(ECGenParameterSpec("secp256r1"))
            }.generateKeyPair()
        val sealedMeta =
            MapsEnvelopeMeta(
                importId = "imp-openai-wrong",
                provider = "maps",
                expiresAtUnix = 4_102_444_800L,
                deviceSerial = "4B230DLAQ001Z5",
                packageName = "app.codexlauncher",
                signingDigestSha256 = "aa",
            )
        val sealed =
            MapsCredentialEnvelope.seal(
                devicePublicKey = device.public,
                meta = sealedMeta,
                apiKeyUtf8 = "x".toByteArray(),
            )
        val expected = sealedMeta.copy(provider = "openai")
        val err =
            runCatching {
                MapsCredentialEnvelope.open(
                    devicePrivateKey = device.private,
                    sealed = sealed,
                    expected = expected,
                    nowUnix = 1L,
                )
            }.exceptionOrNull()
        assertEquals("envelope_aad_mismatch", err?.message)
    }
}
