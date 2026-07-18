package app.codexlauncher.connection.pairing.model

import org.junit.Assert.assertEquals
import org.junit.Test

class PairingWireTest {
    @Test
    fun pairingProofBytesMatchTheGoLengthPrefixedContract() {
        val encoded =
            PairingWire.pairingProofMessage(
                secret = "secret",
                // Arbitrary wire-encoding payload — this golden-bytes test pins the
                // length-prefixed byte contract against Go, not address validation.
                host = "100.64.0.10",
                port = 9443,
                protocol = 1,
                hostIdentity = "identity",
                deviceId = "pixel-9",
                deviceName = "Pixel 9",
                devicePublicKey = "public",
            )

        assertEquals(
            "00000007706169722d7631000000067365637265740000000b3130302e36342e302e3130" +
                "00000004393434330000000131000000086964656e7469747900000007706978656c2d39" +
                "00000007506978656c2039000000067075626c6963",
            encoded.toHex(),
        )
    }

    private fun ByteArray.toHex(): String = joinToString("") { "%02x".format(it) }
}
