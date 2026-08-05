package app.codexlauncher.runtime.localpair.attestation

import java.nio.charset.StandardCharsets
import java.security.MessageDigest

object AttestationClaims {
    enum class SecurityLevel {
        Software,
        TrustedEnvironment,
        StrongBox,
    }

    data class Expected(
        val packageName: String,
        val signingCertSha256: String,
        val offerId: String,
        val runtimeIdentity: String,
        val ephemeralPublicKey: String,
        val tlsSpki: String,
        val transcriptNonce: String,
        val pinnedRootFingerprints: Set<String>,
    )

    data class Observed(
        val packageName: String,
        val signingCertSha256: String,
        val challenge: ByteArray,
        val securityLevel: SecurityLevel,
        val rootFingerprint: String,
        val revoked: Boolean,
    ) {
        override fun equals(other: Any?): Boolean {
            if (this === other) return true
            if (other !is Observed) return false
            return packageName == other.packageName &&
                signingCertSha256 == other.signingCertSha256 &&
                challenge.contentEquals(other.challenge) &&
                securityLevel == other.securityLevel &&
                rootFingerprint == other.rootFingerprint &&
                revoked == other.revoked
        }

        override fun hashCode(): Int {
            var result = packageName.hashCode()
            result = 31 * result + signingCertSha256.hashCode()
            result = 31 * result + challenge.contentHashCode()
            result = 31 * result + securityLevel.hashCode()
            result = 31 * result + rootFingerprint.hashCode()
            result = 31 * result + revoked.hashCode()
            return result
        }
    }

    sealed class Reject {
        data object WrongApplicationId : Reject()
        data object WrongSigner : Reject()
        data object UntrustedRoot : Reject()
        data object WrongChallenge : Reject()
        data object Revoked : Reject()
        data object WeakSecurityLevel : Reject()
    }

    fun challengeBytes(
        offerId: String,
        runtimeIdentity: String,
        ephemeralPublicKey: String,
        tlsSpki: String,
        transcriptNonce: String,
    ): ByteArray {
        val material =
            listOf(offerId, runtimeIdentity, ephemeralPublicKey, tlsSpki, transcriptNonce)
                .joinToString("\u0000")
        return MessageDigest.getInstance("SHA-256").digest(material.toByteArray(StandardCharsets.UTF_8))
    }

    fun reject(expected: Expected, observed: Observed): Reject? {
        if (observed.revoked) return Reject.Revoked
        if (observed.packageName != expected.packageName) return Reject.WrongApplicationId
        if (!observed.signingCertSha256.equals(expected.signingCertSha256, ignoreCase = true)) {
            return Reject.WrongSigner
        }
        if (observed.rootFingerprint !in expected.pinnedRootFingerprints) return Reject.UntrustedRoot
        val want =
            challengeBytes(
                offerId = expected.offerId,
                runtimeIdentity = expected.runtimeIdentity,
                ephemeralPublicKey = expected.ephemeralPublicKey,
                tlsSpki = expected.tlsSpki,
                transcriptNonce = expected.transcriptNonce,
            )
        if (!observed.challenge.contentEquals(want)) return Reject.WrongChallenge
        if (observed.securityLevel == SecurityLevel.Software) return Reject.WeakSecurityLevel
        return null
    }
}
