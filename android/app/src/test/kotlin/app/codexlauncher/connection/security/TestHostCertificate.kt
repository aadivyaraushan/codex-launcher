package app.codexlauncher.connection.security

import okhttp3.tls.HeldCertificate
import java.io.ByteArrayInputStream
import java.security.KeyFactory
import java.security.KeyPair
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.security.spec.PKCS8EncodedKeySpec
import java.util.Base64

internal object TestHostCertificate {
    private val tlsCertificate: HeldCertificate by lazy {
        HeldCertificate.Builder()
            .commonName("Codex Launcher Companion")
            .build()
    }

    fun keyPair(): KeyPair {
        val privateKey =
            KeyFactory.getInstance("Ed25519").generatePrivate(
                PKCS8EncodedKeySpec(Base64.getDecoder().decode(TEST_PRIVATE_KEY_DER)),
            )
        return KeyPair(certificate().publicKey, privateKey)
    }

    fun held(): HeldCertificate = tlsCertificate

    fun tlsIdentity(): String =
        Base64.getUrlEncoder().withoutPadding().encodeToString(tlsCertificate.certificate.publicKey.encoded)

    private fun certificate(): X509Certificate =
        CertificateFactory.getInstance("X.509").generateCertificate(
            ByteArrayInputStream(Base64.getDecoder().decode(TEST_CERTIFICATE_DER)),
        ) as X509Certificate

    private const val TEST_PRIVATE_KEY_DER = "MC4CAQAwBQYDK2VwBCIEIEsdeNgriS3kUQIUcNgWcYeRIr1ATQWD03QbdAixnaJ/"
    private const val TEST_CERTIFICATE_DER =
        "MIHyMIGloAMCAQICAQEwBQYDK2VwMCMxITAfBgNVBAMMGENvZGV4IExhdW5jaGVyIENvbXBhbmlvbjAe" +
            "Fw0yNTAxMDEwMDAwMDBaFw0zNTAxMDEwMDAwMDBaMCMxITAfBgNVBAMMGENvZGV4IExhdW5jaGVyIENv" +
            "bXBhbmlvbjAqMAUGAytlcAMhAJPONiAfzsyisg6QTg+FnhOzqvyBMnp+8IH7necYtVgPMAUGAytlcANB" +
            "AE7KUgByKxkaAbGGTKOIEsTTKGD4jTStjQdg/fR6nSndPxPpY67EwxwdfMDzjrxdNTZGKQ0dIZlDvKnh" +
            "cWJ9Kwg="
}
