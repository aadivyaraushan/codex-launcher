package app.codexlauncher.connection.security

import com.google.crypto.tink.PublicKeyVerify
import com.google.crypto.tink.subtle.Ed25519Verify
import java.security.MessageDigest
import java.security.PublicKey
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import java.util.Base64
import javax.net.ssl.X509TrustManager

class HostIdentityPin private constructor(
    private val expectedSubjectPublicKeyInfo: ByteArray,
    private val verifier: PublicKeyVerify,
) {
    fun matches(publicKey: PublicKey): Boolean =
        MessageDigest.isEqual(expectedSubjectPublicKeyInfo, publicKey.encoded)

    fun verifies(message: ByteArray, signature: ByteArray): Boolean =
        runCatching {
            verifier.verify(signature, message)
            true
        }.getOrDefault(false)

    companion object {
        fun parse(encoded: String): HostIdentityPin {
            val subjectPublicKeyInfo =
                try {
                    Base64.getUrlDecoder().decode(encoded)
                } catch (error: IllegalArgumentException) {
                    throw IllegalArgumentException("Invalid host identity pin", error)
                }
            require(subjectPublicKeyInfo.size == ED25519_SPKI_BYTES) { "Invalid host identity pin" }
            require(subjectPublicKeyInfo.copyOfRange(0, ED25519_SPKI_PREFIX.size).contentEquals(ED25519_SPKI_PREFIX)) {
                "Host identity must be Ed25519"
            }
            val rawPublicKey = subjectPublicKeyInfo.copyOfRange(ED25519_SPKI_PREFIX.size, subjectPublicKeyInfo.size)
            val verifier =
                try {
                    Ed25519Verify(rawPublicKey)
                } catch (error: Exception) {
                    throw IllegalArgumentException("Invalid host identity pin", error)
                }
            return HostIdentityPin(subjectPublicKeyInfo.copyOf(), verifier)
        }

        private const val ED25519_SPKI_BYTES = 44
        private val ED25519_SPKI_PREFIX =
            byteArrayOf(0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00)
    }
}

class TlsIdentityPin private constructor(
    private val expectedSubjectPublicKeyInfo: ByteArray,
) {
    fun matches(publicKey: PublicKey): Boolean =
        MessageDigest.isEqual(expectedSubjectPublicKeyInfo, publicKey.encoded)

    fun trustManager(): X509TrustManager = PinnedTlsTrustManager(this)

    companion object {
        fun parse(encoded: String): TlsIdentityPin {
            val subjectPublicKeyInfo =
                try {
                    Base64.getUrlDecoder().decode(encoded)
                } catch (error: IllegalArgumentException) {
                    throw IllegalArgumentException("Invalid TLS identity pin", error)
                }
            require(subjectPublicKeyInfo.size == P256_SPKI_BYTES) { "Invalid TLS identity pin" }
            require(subjectPublicKeyInfo.copyOfRange(0, P256_SPKI_PREFIX.size).contentEquals(P256_SPKI_PREFIX)) {
                "TLS identity must be P-256"
            }
            return TlsIdentityPin(subjectPublicKeyInfo.copyOf())
        }

        private const val P256_SPKI_BYTES = 91
        private val P256_SPKI_PREFIX =
            byteArrayOf(
                0x30, 0x59, 0x30, 0x13, 0x06, 0x07, 0x2a, 0x86.toByte(), 0x48, 0xce.toByte(), 0x3d, 0x02, 0x01,
                0x06, 0x08, 0x2a, 0x86.toByte(), 0x48, 0xce.toByte(), 0x3d, 0x03, 0x01, 0x07, 0x03, 0x42, 0x00,
            )
    }
}

private class PinnedTlsTrustManager(
    private val pin: TlsIdentityPin,
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
