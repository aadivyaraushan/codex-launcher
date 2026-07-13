package app.codexlauncher.connection.security

import okhttp3.ConnectionSpec
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.TlsVersion
import java.security.SecureRandom
import java.security.cert.X509Certificate
import javax.net.ssl.SSLContext

class PinnedTlsClientFactory {
    fun builder(pin: HostIdentityPin): OkHttpClient.Builder {
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
    }
}
