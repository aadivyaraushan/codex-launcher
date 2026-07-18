package app.codexlauncher.connection.security

import okhttp3.Dns
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import java.net.InetAddress
import java.net.UnknownHostException

/**
 * Spec for [SafePublicDns], the connection-time half of the SSRF defense.
 *
 * The pure [app.codexlauncher.connection.pairing.model.PairingValidation.isSafePublicEndpoint]
 * rule accepts a hostname on shape alone — it cannot resolve DNS (that would be a
 * main-thread network call at link-scan time). So a malicious link can name a
 * hostname that *looks* public but *resolves* to a private or cloud-metadata
 * address (DNS rebinding). This is the backstop: the one resolver OkHttp actually
 * dials with. It resolves once, drops every address in the deny-list, and returns
 * only the survivors — so OkHttp can never connect to an internal IP, and never
 * gets a second (poisoned) re-resolution.
 */
class SafePublicDnsTest {
    private fun resolverReturning(vararg literals: String): Dns =
        Dns { literals.map { InetAddress.getByName(it) } }

    @Test
    fun keepsOnlyThePublicAddressesFromAMixedAnswer() {
        // A rebinding answer mixes a public decoy with the private target it wants
        // the phone to actually hit. Only the public one may survive.
        val dns = SafePublicDns(resolver = resolverReturning("203.0.113.5", "10.0.0.5"))

        val result = dns.lookup("box.example.com")

        assertEquals(listOf(InetAddress.getByName("203.0.113.5")), result)
    }

    @Test
    fun failsClosedWhenEveryResolvedAddressIsPrivate() {
        // The classic rebind: the link's hostname passed the shape check, but at
        // dial time it resolves only to an internal address. The dial must fail,
        // not silently fall through to the system resolver.
        val dns = SafePublicDns(resolver = resolverReturning("10.0.0.5"))

        assertThrows(UnknownHostException::class.java) { dns.lookup("box.example.com") }
    }

    @Test
    fun failsClosedOnCloudMetadataAddress() {
        val dns = SafePublicDns(resolver = resolverReturning("169.254.169.254"))

        assertThrows(UnknownHostException::class.java) { dns.lookup("metadata.example.com") }
    }

    @Test
    fun failsClosedOnLoopbackAndUlaAndLinkLocal() {
        listOf(
            "127.0.0.1", "::1", "fc00::1", "fe80::1",
            "::ffff:192.168.1.1", // IPv4-mapped private
            "::169.254.169.254", // IPv4-compatible metadata (rebind target)
            "::127.0.0.1", // IPv4-compatible loopback
        ).forEach { literal ->
            val dns = SafePublicDns(resolver = resolverReturning(literal))
            assertThrows("expected fail-closed for $literal", UnknownHostException::class.java) {
                dns.lookup("host.example.com")
            }
        }
    }

    @Test
    fun passesThroughAPurelyPublicAnswerUnchanged() {
        val dns = SafePublicDns(resolver = resolverReturning("203.0.113.5", "2606:4700:4700::1111"))

        val result = dns.lookup("box.example.com")

        assertEquals(
            listOf(InetAddress.getByName("203.0.113.5"), InetAddress.getByName("2606:4700:4700::1111")),
            result,
        )
    }
}
