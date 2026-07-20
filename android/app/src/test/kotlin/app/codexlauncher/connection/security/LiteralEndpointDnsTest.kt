package app.codexlauncher.connection.security

import java.net.InetAddress
import java.net.UnknownHostException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class LiteralEndpointDnsTest {
    @Test
    fun returnsOnlyTheClassifiedTailscaleAddressForItsExactHost() {
        val address = InetAddress.getByName("100.64.0.10")
        val dns = LiteralEndpointDns("100.64.0.10", address)

        assertEquals(listOf(address), dns.lookup("100.64.0.10"))
        assertThrows(UnknownHostException::class.java) { dns.lookup("relay.example.com") }
    }
}
