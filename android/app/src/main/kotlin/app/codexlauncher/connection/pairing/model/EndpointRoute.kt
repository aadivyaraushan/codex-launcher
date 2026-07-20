package app.codexlauncher.connection.pairing.model

import java.net.Inet4Address
import java.net.Inet6Address
import java.net.InetAddress

/**
 * Classifies every companion address at the pairing and stored-record boundary.
 * A Tailscale route is always an already-parsed literal, so it can later be
 * dialed without DNS. Relay routes retain the public-DNS path.
 */
sealed interface EndpointRoute {
    enum class Kind { PUBLIC, TAILSCALE }

    val host: String
    val kind: Kind

    data class PublicEndpoint(override val host: String) : EndpointRoute {
        override val kind: Kind = Kind.PUBLIC
    }

    data class TailscaleLiteral(
        override val host: String,
        val address: InetAddress,
    ) : EndpointRoute {
        override val kind: Kind = Kind.TAILSCALE
    }

    companion object {
        fun classify(host: String): EndpointRoute? {
            val literal = parseLiteral(host)
            if (literal != null && isTailscale(literal)) return TailscaleLiteral(host, literal)
            if (PairingValidation.isSafePublicEndpoint(host)) return PublicEndpoint(host)
            return null
        }

        private fun parseLiteral(host: String): InetAddress? {
            if (host.isEmpty() || host.contains('%')) return null
            if (host.contains(':')) {
                if (host.any { it !in "0123456789abcdefABCDEF:." }) return null
            } else if (host.any { it !in '0'..'9' && it != '.' }) {
                return null
            }
            return runCatching { InetAddress.getByName(host) }.getOrNull()
        }

        private fun isTailscale(address: InetAddress): Boolean =
            when (address) {
                is Inet4Address -> {
                    val bytes = address.address
                    bytes[0].toInt() and 0xff == 100 && (bytes[1].toInt() and 0xff) in 64..127
                }
                is Inet6Address -> {
                    val bytes = address.address
                    bytes[0].toInt() and 0xff == 0xfd &&
                        bytes[1].toInt() and 0xff == 0x7a &&
                        bytes[2].toInt() and 0xff == 0x11 &&
                        bytes[3].toInt() and 0xff == 0x5c &&
                        bytes[4].toInt() and 0xff == 0xa1 &&
                        bytes[5].toInt() and 0xff == 0xe0
                }
                else -> false
            }
    }
}
