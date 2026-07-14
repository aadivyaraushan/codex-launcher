package app.codexlauncher.connection.pairing.model

import app.codexlauncher.connection.security.HostIdentityPin
import app.codexlauncher.connection.security.TlsIdentityPin
import java.net.URI
import java.net.URLDecoder
import java.nio.charset.StandardCharsets
import java.util.Base64

class PairingOffer private constructor(
    val host: String,
    val port: Int,
    val protocol: Int,
    val hostIdentity: String,
    val tlsIdentity: String,
    val secret: String,
) {
    val pairingEndpoint: String
        get() {
            val endpointHost = if (host.contains(':')) "[$host]" else host
            return "https://$endpointHost:$port/v1/pair"
        }

    fun tlsIdentityPin(): TlsIdentityPin = TlsIdentityPin.parse(tlsIdentity)

    override fun toString(): String =
        "PairingOffer(host=$host, port=$port, protocol=$protocol, hostIdentity=[redacted], tlsIdentity=[redacted], secret=[redacted])"

    companion object {
        private val requiredFields = setOf("host", "port", "v", "identity", "tls_identity", "secret")

        fun parse(encoded: String): PairingOffer {
            val uri = runCatching { URI(encoded) }.getOrElse { throw invalidOffer(it) }
            require(
                !uri.isOpaque &&
                    uri.scheme == "codex-launcher" &&
                    uri.host == "pair" &&
                    uri.userInfo == null &&
                    uri.port == -1 &&
                    uri.path.isNullOrEmpty() &&
                    uri.fragment == null,
            ) { "Invalid pairing offer" }
            val query = parseQuery(uri.rawQuery)
            require(query.keys == requiredFields) { "Invalid pairing offer" }
            val host = query.getValue("host")
            val port = query.getValue("port").toIntOrNull()
            val protocol = query.getValue("v").toIntOrNull()
            val identity = query.getValue("identity")
            val tlsIdentity = query.getValue("tls_identity")
            val secret = query.getValue("secret")
            require(PairingValidation.isTailscaleAddress(host) && port in 1..65535 && protocol == 1) { "Invalid pairing offer" }
            HostIdentityPin.parse(identity)
            TlsIdentityPin.parse(tlsIdentity)
            require(PairingValidation.isCanonicalBase64Url(secret, decodedBytes = 16)) { "Invalid pairing offer" }
            return PairingOffer(
                host = host,
                port = requireNotNull(port),
                protocol = requireNotNull(protocol),
                hostIdentity = identity,
                tlsIdentity = tlsIdentity,
                secret = secret,
            )
        }

        private fun parseQuery(rawQuery: String?): Map<String, String> {
            require(!rawQuery.isNullOrEmpty()) { "Invalid pairing offer" }
            val result = linkedMapOf<String, String>()
            rawQuery.split('&').forEach { field ->
                val separator = field.indexOf('=')
                require(separator > 0 && separator == field.lastIndexOf('=')) { "Invalid pairing offer" }
                val name = decode(field.substring(0, separator))
                val value = decode(field.substring(separator + 1))
                require(name !in result && value.isNotEmpty()) { "Invalid pairing offer" }
                result[name] = value
            }
            return result
        }

        private fun decode(value: String): String =
            runCatching { URLDecoder.decode(value, StandardCharsets.UTF_8.name()) }
                .getOrElse { throw invalidOffer(it) }

        private fun invalidOffer(cause: Throwable): IllegalArgumentException =
            IllegalArgumentException("Invalid pairing offer", cause)
    }
}
