package app.codexlauncher.connection.pairing.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class EndpointRouteTest {
    @Test
    fun classifiesOnlyOfficialTailscaleLiteralsAsPrivateRoutes() {
        assertTrue(EndpointRoute.classify("100.64.0.10") is EndpointRoute.TailscaleLiteral)
        assertTrue(EndpointRoute.classify("fd7a:115c:a1e0::10") is EndpointRoute.TailscaleLiteral)
        assertEquals(EndpointRoute.Kind.PUBLIC, EndpointRoute.classify("relay.example.com")?.kind)
        assertEquals(EndpointRoute.Kind.PUBLIC, EndpointRoute.classify("100.63.255.255")?.kind)
        assertNull(EndpointRoute.classify("192.168.1.10"))
        assertNull(EndpointRoute.classify("fd7a:115c:a1e1::10"))
    }
}
