package app.codexlauncher.connection.pairing.network

import app.codexlauncher.connection.pairing.model.PairingOffer
import app.codexlauncher.connection.security.TestHostCertificate
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.tls.HandshakeCertificates
import okhttp3.TlsVersion
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import java.security.KeyPairGenerator
import java.util.Base64

class PinnedPairingTransportTest {
    private val servers = mutableListOf<MockWebServer>()

    @After
    fun closeServers() {
        servers.forEach(MockWebServer::close)
    }

    @Test
    fun postsOverTls13OnlyWhenTheServerCertificateHasTheExactPinnedTlsKey() {
        val hostKey = TestHostCertificate.keyPair()
        val server = tlsServer().apply {
            enqueue(
                MockResponse.Builder()
                    .code(201)
                    .body("""{"deviceId":"pixel-9","pairingGeneration":"AQIDBAUGBwgJCgsMDQ4PEA"}""")
                    .build(),
            )
            start()
        }
        val offer = PairingOffer.parse(offerUri(server.port, hostKey.public.encoded))
        val request = testRequest(offer)

        val response = PinnedPairingTransport(server.url("/v1/pair").toString()).pair(offer, request)

        assertEquals("pixel-9", response.deviceId)
        val recorded = server.takeRequest()
        assertEquals("/v1/pair", recorded.url.encodedPath)
        assertEquals("POST", recorded.method)
        assertEquals(TlsVersion.TLS_1_3, recorded.handshake?.tlsVersion)
        assertEquals("application/json; charset=utf-8", recorded.headers["Content-Type"])
        assertEquals(offer.secret, recorded.body?.utf8()?.let { body -> Regex("\\\"secret\\\":\\\"([^\\\"]+)\\\"").find(body)?.groupValues?.get(1) })
    }

    @Test
    fun rejectsAValidCertificateWhoseTlsKeyDoesNotMatchTheOffer() {
        val offeredTlsKey = KeyPairGenerator.getInstance("EC").apply { initialize(256) }.generateKeyPair()
        val server = tlsServer().apply {
            enqueue(MockResponse.Builder().code(201).body("{}").build())
            start()
        }
        val offer = PairingOffer.parse(
            offerUri(
                port = server.port,
                hostPublicKey = TestHostCertificate.keyPair().public.encoded,
                tlsPublicKey = offeredTlsKey.public.encoded,
            ),
        )

        assertThrows(PairingException::class.java) {
            PinnedPairingTransport(server.url("/v1/pair").toString()).pair(offer, testRequest(offer))
        }
    }

    private fun tlsServer(): MockWebServer {
        val certificates = HandshakeCertificates.Builder().heldCertificate(TestHostCertificate.held()).build()
        return MockWebServer().also { server ->
            server.useHttps(certificates.sslSocketFactory())
            servers += server
        }
    }

    private fun offerUri(
        port: Int,
        hostPublicKey: ByteArray,
        tlsPublicKey: ByteArray = TestHostCertificate.held().certificate.publicKey.encoded,
    ): String {
        val identity = Base64.getUrlEncoder().withoutPadding().encodeToString(hostPublicKey)
        val tlsIdentity = Base64.getUrlEncoder().withoutPadding().encodeToString(tlsPublicKey)
        val secret = Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(16) { it.toByte() })
        return "codex-launcher://pair?host=203.0.113.5&port=$port&v=1&identity=$identity&tls_identity=$tlsIdentity&secret=$secret"
    }

    private fun testRequest(offer: PairingOffer): PairingRequest =
        PairingRequest(
            secret = offer.secret,
            host = offer.host,
            port = offer.port,
            protocol = offer.protocol,
            hostIdentity = offer.hostIdentity,
            deviceId = "pixel-9",
            deviceName = "Pixel 9",
            devicePublicKey = "public-key",
            signature = "signature",
        )

}
