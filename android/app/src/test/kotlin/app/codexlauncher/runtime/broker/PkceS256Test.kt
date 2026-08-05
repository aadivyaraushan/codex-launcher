package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class PkceS256Test {
    @Test
    fun challengeIsUrlSafeBase64Sha256OfVerifier() {
        // RFC 7636 appendix B vector
        val verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
        val challenge = PkceS256.challengeS256(verifier)
        assertEquals("E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", challenge)
    }

    @Test
    fun generatedVerifierLengthAndAlphabet() {
        val v = PkceS256.newVerifier()
        assertTrue(v.length in 43..128)
        assertTrue(v.all { it.isLetterOrDigit() || it == '-' || it == '_' })
        assertTrue(PkceS256.challengeS256(v).isNotBlank())
    }
}
