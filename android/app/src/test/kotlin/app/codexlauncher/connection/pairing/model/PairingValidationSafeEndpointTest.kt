package app.codexlauncher.connection.pairing.model

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Spec for [PairingValidation.isSafePublicEndpoint], the rule that replaces the
 * old Tailscale allowlist now that the phone dials a public relay box instead of
 * a Tailscale IP.
 *
 * The threat this rule exists to stop: a malicious pairing link that points the
 * phone at an internal service (loopback, private LAN, cloud metadata) and turns
 * the phone into a blind network probe (SSRF). Content can never leak — the phone
 * only ever speaks pinned, sealed TLS — but the *probing* is real, so the phone
 * must refuse any address that resolves inside the network.
 *
 * This function is pure (no DNS): it decides IP literals outright and accepts a
 * hostname only on shape. The resolved-IP check for hostnames happens at dial
 * time in the custom OkHttp Dns; that is tested separately.
 */
class PairingValidationSafeEndpointTest {
    @Test
    fun acceptsPublicIpv4() {
        val publicHosts =
            listOf(
                "1.1.1.1", // Cloudflare — 1/8 is public
                "8.8.8.8", // Google DNS
                "93.184.216.34", // example.com
                // Range edges that sit just OUTSIDE a denied block are public:
                "11.0.0.1", // just above 10/8
                "126.255.255.255", // just below 127/8
                "128.0.0.1", // just above 127/8
                "100.63.255.255", // just below 100.64/10 CGNAT
                "100.128.0.1", // just above 100.64/10 CGNAT (was rejected under Tailscale rule)
                "172.15.255.255", // just below 172.16/12
                "172.32.0.0", // just above 172.16/12
                "192.167.255.255", // just below 192.168/16
                "192.169.0.0", // just above 192.168/16
                "192.0.1.0", // just above 192.0.0/24
            )
        publicHosts.forEach { host ->
            assertTrue("expected public: $host", PairingValidation.isSafePublicEndpoint(host))
        }
    }

    @Test
    fun rejectsPrivateLoopbackLinkLocalAndMetadataIpv4() {
        val deniedHosts =
            listOf(
                "0.0.0.0", // 0/8
                "0.1.2.3", // 0/8
                "10.0.0.1", // 10/8
                "10.255.255.255", // 10/8 top
                "100.64.0.10", // CGNAT — the old "valid" fixture is now denied
                "100.127.255.255", // CGNAT top
                "127.0.0.1", // loopback
                "127.1.2.3", // loopback (whole /8)
                "169.254.0.1", // link-local
                "169.254.169.254", // cloud metadata
                "172.16.0.1", // 172.16/12
                "172.31.255.255", // 172.16/12 top
                "192.0.0.1", // 192.0.0/24
                "192.168.1.10", // 192.168/16
            )
        deniedHosts.forEach { host ->
            assertFalse("expected denied: $host", PairingValidation.isSafePublicEndpoint(host))
        }
    }

    @Test
    fun rejectsDisguisedIpv4Encodings() {
        // Octal, hex, integer, and short forms all resolve to a private/loopback
        // address on a permissive parser. We refuse them outright: they are
        // neither clean dotted-decimal nor a valid hostname.
        val disguised =
            listOf(
                "0177.0.0.1", // octal 127.0.0.1
                "0x7f.0.0.1", // hex 127.0.0.1
                "0x7f000001", // hex integer 127.0.0.1
                "2130706433", // decimal integer 127.0.0.1
                "127.1", // short form 127.0.0.1
                "127.0.1", // short form
                "010.0.0.1", // leading-zero octal-ish
                "1.2.3.4.5", // too many octets
                "256.1.1.1", // octet out of range
                "1.2.3.256", // octet out of range
            )
        disguised.forEach { host ->
            assertFalse("expected rejected: $host", PairingValidation.isSafePublicEndpoint(host))
        }
    }

    @Test
    fun acceptsPublicIpv6() {
        val publicHosts =
            listOf(
                "2606:4700:4700::1111", // Cloudflare
                "2001:4860:4860::8888", // Google DNS (2001: but not db8 documentation)
                "2400:cb00::1",
            )
        publicHosts.forEach { host ->
            assertTrue("expected public: $host", PairingValidation.isSafePublicEndpoint(host))
        }
    }

    @Test
    fun rejectsLoopbackLinkLocalUlaAndDocumentationIpv6() {
        val deniedHosts =
            listOf(
                "::", // unspecified
                "::1", // loopback
                "fc00::1", // ULA
                "fdff:ffff::1", // ULA (fd.. is inside fc00::/7)
                "fd7a:115c:a1e0::1", // the old Tailscale fixture — now denied (ULA)
                "fe80::1", // link-local
                "febf:ffff::1", // link-local top of fe80::/10
                "fec0::1", // deprecated site-local (fec0::/10)
                "fec0:0:0:1::1", // deprecated site-local
                "ff02::1", // multicast (ff00::/8)
                "ff05::1", // multicast
                "2001:db8::1", // documentation
                "2001:db8:dead:beef::1", // documentation
            )
        deniedHosts.forEach { host ->
            assertFalse("expected denied: $host", PairingValidation.isSafePublicEndpoint(host))
        }
    }

    @Test
    fun rejectsNat64AndSixToFourRangesEntirely() {
        // The whole NAT64 and 6to4 prefixes are denied: they can wrap a private
        // IPv4, and it is safer to refuse the range than to try to peer inside.
        val deniedHosts =
            listOf(
                "64:ff9b::7f00:1", // NAT64 wrapping 127.0.0.1
                "64:ff9b::a00:1", // NAT64 wrapping 10.0.0.1
                "64:ff9b::808:808", // NAT64 even wrapping a public IP — still denied
                "2002:7f00:1::", // 6to4 wrapping 127.0.0.1
                "2002:0a00:0001::", // 6to4 wrapping 10.0.0.1
                "2002:0808:0808::", // 6to4 wrapping 8.8.8.8 — still denied
            )
        deniedHosts.forEach { host ->
            assertFalse("expected denied: $host", PairingValidation.isSafePublicEndpoint(host))
        }
    }

    @Test
    fun rejectsIpv4MappedIpv6ThatHidesAPrivateAddress() {
        val deniedHosts =
            listOf(
                "::ffff:127.0.0.1", // loopback
                "::ffff:10.0.0.1", // private
                "::ffff:169.254.169.254", // metadata
                "::ffff:192.168.1.1", // private
            )
        deniedHosts.forEach { host ->
            assertFalse("expected denied: $host", PairingValidation.isSafePublicEndpoint(host))
        }
    }

    @Test
    fun rejectsIpv4CompatibleIpv6ThatHidesAPrivateAddress() {
        // ::a.b.c.d (no ::ffff: marker) is the sibling encoding of IPv4-mapped and
        // embeds the same v4 in the last 4 bytes. It must be denied the same way —
        // nothing in ::/80 is a real public address.
        val deniedHosts =
            listOf(
                "::127.0.0.1", // loopback
                "::10.0.0.1", // private
                "::169.254.169.254", // cloud metadata
                "::192.168.1.1", // private
            )
        deniedHosts.forEach { host ->
            assertFalse("expected denied: $host", PairingValidation.isSafePublicEndpoint(host))
        }
    }

    @Test
    fun rejectsBroadcastMulticastAndReservedIpv4() {
        val deniedHosts =
            listOf(
                "255.255.255.255", // limited broadcast
                "224.0.0.1", // multicast 224/4
                "239.255.255.255", // multicast top
                "240.0.0.1", // reserved 240/4
                "192.88.99.1", // 6to4 relay anycast
                "198.18.0.1", // benchmarking 198.18/15
                "198.19.255.255", // benchmarking top
            )
        deniedHosts.forEach { host ->
            assertFalse("expected denied: $host", PairingValidation.isSafePublicEndpoint(host))
        }
    }

    @Test
    fun rejectsIpv6WithZoneId() {
        assertFalse(PairingValidation.isSafePublicEndpoint("fe80::1%eth0"))
        assertFalse(PairingValidation.isSafePublicEndpoint("2606:4700::1111%wlan0"))
    }

    @Test
    fun acceptsPublicHostnamesOnShape() {
        val hostnames =
            listOf(
                "box.fly.dev",
                "codex-relay.fly.dev",
                "example.com",
                "a.co",
                "my-box.example.co.uk",
            )
        hostnames.forEach { host ->
            assertTrue("expected accepted hostname: $host", PairingValidation.isSafePublicEndpoint(host))
        }
    }

    @Test
    fun rejectsMalformedOrSingleLabelHostnames() {
        val bad =
            listOf(
                "", // empty
                "localhost", // single label (also loopback name)
                "box", // single label
                "-box.fly.dev", // label starts with hyphen
                "box-.fly.dev", // label ends with hyphen
                "box..fly.dev", // empty label
                "box.fly.dev.", // trailing dot -> empty last label
                "box .fly.dev", // space
                "box/../etc.fly.dev", // path-ish junk
                "box.123", // numeric TLD (looks like a broken IP)
            )
        bad.forEach { host ->
            assertFalse("expected rejected: $host", PairingValidation.isSafePublicEndpoint(host))
        }
    }
}
