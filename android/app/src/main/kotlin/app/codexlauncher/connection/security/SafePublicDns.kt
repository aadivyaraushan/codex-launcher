package app.codexlauncher.connection.security

import app.codexlauncher.connection.security.publicaddress.PublicInetAddress
import okhttp3.Dns
import java.net.InetAddress
import java.net.UnknownHostException

/**
 * Resolves a hostname exactly once and returns only the public addresses, so
 * OkHttp can never dial a private/loopback/metadata IP and never re-resolves to a
 * fresh (poisoned) answer. Fails closed: if nothing public survives, the dial
 * fails rather than falling back to the system resolver. See relay-box plan §6.
 */
internal class SafePublicDns(private val resolver: Dns = Dns.SYSTEM) : Dns {
    override fun lookup(hostname: String): List<InetAddress> {
        val safe = resolver.lookup(hostname).filter(PublicInetAddress::isPublic)
        if (safe.isEmpty()) {
            throw UnknownHostException("refusing non-public address(es) for host: $hostname")
        }
        return safe
    }
}
