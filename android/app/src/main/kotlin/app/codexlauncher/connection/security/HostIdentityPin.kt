package app.codexlauncher.connection.security

import java.security.KeyFactory
import java.security.MessageDigest
import java.security.PublicKey
import java.security.Signature
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import java.security.spec.X509EncodedKeySpec
import java.util.Base64
import javax.net.ssl.X509TrustManager

class HostIdentityPin private constructor(
    private val expectedSubjectPublicKeyInfo: ByteArray,
    private val hostPublicKey: PublicKey,
) {
    fun matches(publicKey: PublicKey): Boolean =
        MessageDigest.isEqual(expectedSubjectPublicKeyInfo, publicKey.encoded)

    fun trustManager(): X509TrustManager = PinnedHostTrustManager(this)

    fun verifies(message: ByteArray, signature: ByteArray): Boolean =
        runCatching {
            Signature.getInstance("Ed25519").run {
                initVerify(hostPublicKey)
                update(message)
                verify(signature)
            }
        }.getOrDefault(false)

    companion object {
        fun parse(encoded: String): HostIdentityPin {
            val subjectPublicKeyInfo =
                try {
                    Base64.getUrlDecoder().decode(encoded)
                } catch (error: IllegalArgumentException) {
                    throw IllegalArgumentException("Invalid host identity pin", error)
                }
            require(subjectPublicKeyInfo.size in 32..128) { "Invalid host identity pin" }
            val publicKey =
                try {
                    KeyFactory.getInstance("Ed25519").generatePublic(X509EncodedKeySpec(subjectPublicKeyInfo))
                } catch (error: Exception) {
                    throw IllegalArgumentException("Invalid host identity pin", error)
                }
            require(publicKey.algorithm.equals("Ed25519", ignoreCase = true) || publicKey.algorithm.equals("EdDSA", ignoreCase = true)) {
                "Host identity must be Ed25519"
            }
            return HostIdentityPin(publicKey.encoded.copyOf(), publicKey)
        }
    }
}

private class PinnedHostTrustManager(
    private val pin: HostIdentityPin,
) : X509TrustManager {
    override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        val certificate = chain?.singleOrNull() ?: throw CertificateException("Expected one self-signed companion certificate")
        try {
            certificate.checkValidity()
            if (!pin.matches(certificate.publicKey)) throw CertificateException("Companion host identity changed")
            certificate.verify(certificate.publicKey)
        } catch (error: CertificateException) {
            throw error
        } catch (error: Exception) {
            throw CertificateException("Companion certificate verification failed", error)
        }
    }

    override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        throw CertificateException("Client certificates are not accepted")
    }

    @Suppress("unused")
    fun checkServerTrusted(
        chain: Array<X509Certificate>,
        authType: String,
        host: String,
    ): List<X509Certificate> {
        checkServerTrusted(chain, authType)
        return chain.toList()
    }

    override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
}
