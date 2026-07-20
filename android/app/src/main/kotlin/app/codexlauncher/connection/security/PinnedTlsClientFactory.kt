package app.codexlauncher.connection.security

import app.codexlauncher.connection.pairing.model.EndpointRoute
import okhttp3.ConnectionSpec
import okhttp3.Dns
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.TlsVersion
import java.security.SecureRandom
import java.security.cert.X509Certificate
import javax.net.ssl.SSLContext

class PinnedTlsClientFactory(private val dns: Dns = SafePublicDns()) {
    fun builder(pin: TlsIdentityPin, route: EndpointRoute): OkHttpClient.Builder =
        builder(pin, when (route) {
            is EndpointRoute.PublicEndpoint -> dns
            is EndpointRoute.TailscaleLiteral -> LiteralEndpointDns(route.host, route.address)
        })

    // Test-only callers that target an explicit local server keep their
    // injected resolver. Production callers must pass a classified route.
    internal fun builder(pin: TlsIdentityPin): OkHttpClient.Builder = builder(pin, dns)

    private fun builder(pin: TlsIdentityPin, resolver: Dns): OkHttpClient.Builder {
        val trustManager = pin.trustManager()
        val sslContext = SSLContext.getInstance("TLSv1.3").apply {
            init(null, arrayOf(trustManager), SecureRandom())
        }
        val tls13 =
            ConnectionSpec.Builder(ConnectionSpec.RESTRICTED_TLS)
                .tlsVersions(TlsVersion.TLS_1_3)
                .build()
        return OkHttpClient.Builder()
            .sslSocketFactory(sslContext.socketFactory, trustManager)
            .hostnameVerifier { _, session ->
                runCatching {
                    val certificate = session.peerCertificates.singleOrNull() as? X509Certificate
                    certificate != null && pin.matches(certificate.publicKey)
                }.getOrDefault(false)
            }.connectionSpecs(listOf(tls13))
            .protocols(listOf(Protocol.HTTP_1_1))
            .dns(resolver)
    }
}
