package app.codexlauncher.connection.security

import okhttp3.Dns
import java.net.InetAddress
import java.net.UnknownHostException

/** A literal-only resolver for a previously classified Tailscale route. */
internal class LiteralEndpointDns(
    private val expectedHost: String,
    private val address: InetAddress,
) : Dns {
    override fun lookup(hostname: String): List<InetAddress> {
        if (hostname != expectedHost) throw UnknownHostException("unexpected Tailscale hostname: $hostname")
        return listOf(address)
    }
}
