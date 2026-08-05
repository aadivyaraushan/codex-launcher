package app.codexlauncher.runtime.localpair.attestation

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class AttestationClaimsTest {
    private val expected =
        AttestationClaims.Expected(
            packageName = "app.codexlauncher",
            signingCertSha256 = "aa11",
            offerId = "offer-1",
            runtimeIdentity = "rid",
            ephemeralPublicKey = "epk",
            tlsSpki = "pin",
            transcriptNonce = "nonce",
            pinnedRootFingerprints = setOf("root-avd"),
        )

    private val good =
        AttestationClaims.Observed(
            packageName = "app.codexlauncher",
            signingCertSha256 = "AA11",
            challenge =
                AttestationClaims.challengeBytes(
                    offerId = "offer-1",
                    runtimeIdentity = "rid",
                    ephemeralPublicKey = "epk",
                    tlsSpki = "pin",
                    transcriptNonce = "nonce",
                ),
            securityLevel = AttestationClaims.SecurityLevel.TrustedEnvironment,
            rootFingerprint = "root-avd",
            revoked = false,
        )

    @Test
    fun acceptsMatchingAttestationClaims() {
        assertNull(AttestationClaims.reject(expected, good))
    }

    @Test
    fun rejectsWrongPackage() {
        assertEquals(
            AttestationClaims.Reject.WrongApplicationId,
            AttestationClaims.reject(expected, good.copy(packageName = "com.evil")),
        )
    }

    @Test
    fun rejectsUntrustedRoot() {
        assertEquals(
            AttestationClaims.Reject.UntrustedRoot,
            AttestationClaims.reject(expected, good.copy(rootFingerprint = "attacker-root")),
        )
    }

    @Test
    fun rejectsWrongChallenge() {
        assertEquals(
            AttestationClaims.Reject.WrongChallenge,
            AttestationClaims.reject(
                expected,
                good.copy(challenge = ByteArray(32) { 7 }),
            ),
        )
    }

    @Test
    fun rejectsRevokedChain() {
        assertEquals(
            AttestationClaims.Reject.Revoked,
            AttestationClaims.reject(expected, good.copy(revoked = true)),
        )
    }
}
