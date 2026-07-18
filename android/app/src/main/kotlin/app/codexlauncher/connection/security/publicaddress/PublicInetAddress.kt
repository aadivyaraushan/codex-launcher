package app.codexlauncher.connection.security.publicaddress

import java.net.InetAddress

/**
 * Single source of truth for "is this a public, routable address we may dial?"
 * Used both by the pure link/record validator (on IP literals) and by the
 * connection-time DNS filter (on resolved addresses). Deny-list mirrors the
 * relay-box plan Section 6.
 */
internal object PublicInetAddress {
    fun isPublic(address: InetAddress): Boolean {
        val b = address.address
        if (b.size == 4) return isPublicIpv4(b[0].u(), b[1].u(), b[2].u(), b[3].u())
        if (b.size != 16) return false
        // Everything in ::/80 (first 10 bytes zero) carries an IPv4 in its last 4
        // bytes: this covers :: (unspecified), ::1 (loopback), ::ffff:a.b.c.d
        // (IPv4-mapped) AND ::a.b.c.d (IPv4-compatible). No global-unicast IPv6
        // lives here (that starts at 2000::/3), so apply the IPv4 deny-list to the
        // embedded address — which closes the ::a.b.c.d metadata/loopback bypass
        // that a marker-only ("::ffff:") check would miss.
        if ((0..9).all { b[it] == 0.toByte() }) {
            return isPublicIpv4(b[12].u(), b[13].u(), b[14].u(), b[15].u())
        }
        if (b[0] == 0xff.toByte()) return false // ff00::/8 multicast
        if ((b[0].toInt() and 0xfe) == 0xfc) return false // fc00::/7 ULA
        if (b[0] == 0xfe.toByte() && (b[1].toInt() and 0xc0) == 0x80) return false // fe80::/10 link-local
        if (b[0] == 0xfe.toByte() && (b[1].toInt() and 0xc0) == 0xc0) return false // fec0::/10 deprecated site-local
        if (b[0] == 0x20.toByte() && b[1] == 0x01.toByte() && b[2] == 0x0d.toByte() && b[3] == 0xb8.toByte()) return false // 2001:db8::/32 doc
        if (b[0] == 0x00.toByte() && b[1] == 0x64.toByte() && b[2] == 0xff.toByte() && b[3] == 0x9b.toByte() && (4..11).all { b[it] == 0.toByte() }) return false // 64:ff9b::/96 NAT64
        if (b[0] == 0x20.toByte() && b[1] == 0x02.toByte()) return false // 2002::/16 6to4
        return true
    }

    fun isPublicIpv4(octet0: Int, octet1: Int, octet2: Int, octet3: Int): Boolean {
        if (octet0 == 0) return false
        if (octet0 == 10) return false
        if (octet0 == 100 && octet1 in 64..127) return false
        if (octet0 == 127) return false
        if (octet0 == 169 && octet1 == 254) return false
        if (octet0 == 172 && octet1 in 16..31) return false
        if (octet0 == 192 && octet1 == 0 && octet2 == 0) return false
        if (octet0 == 192 && octet1 == 88 && octet2 == 99) return false // 6to4 relay anycast
        if (octet0 == 192 && octet1 == 168) return false
        if (octet0 == 198 && octet1 in 18..19) return false // 198.18/15 benchmarking
        if (octet0 >= 224) return false // 224/4 multicast, 240/4 reserved, 255.255.255.255 broadcast
        return true
    }

    private fun Byte.u(): Int = this.toInt() and 0xff
}
