// Gate: importers=MapsPlatformClient; callers=unit test + live Maps broker;
// API=Places searchText + Routes computeRoutes with X-Android-Package/Cert;
// schemas=MapsPlace + MapsRoute; user: "Implement the minimal Android Maps
// broker path... Places/Routes calls"
package app.codexlauncher.runtime.broker.maps

import app.codexlauncher.runtime.broker.maps.http.MapsAppIdentity
import app.codexlauncher.runtime.broker.maps.http.MapsPlatformClient
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class MapsPlatformClientTest {
    private val servers = mutableListOf<MockWebServer>()

    @After
    fun tearDown() {
        servers.forEach(MockWebServer::close)
    }

    @Test
    fun searchPlaceSendsAndroidRestrictedHeadersAndParsesTopMatch() {
        val server = MockWebServer().also { servers += it; it.start() }
        server.enqueue(
            MockResponse.Builder()
                .code(200)
                .body(
                    """
                    {"places":[{"id":"places/ChIJ_test","displayName":{"text":"Ferry Building"},
                    "formattedAddress":"1 Ferry Building, San Francisco, CA"}]}
                    """.trimIndent(),
                )
                .build(),
        )
        val client =
            MapsPlatformClient(
                placesBaseUrl = server.url("/").toString().trimEnd('/'),
                routesBaseUrl = server.url("/").toString().trimEnd('/'),
                http = OkHttpClient(),
                identity =
                    MapsAppIdentity(
                        packageName = "app.codexlauncher",
                        certSha1Hex = "f7a85af51de481aa5c1d1211c0f18d1b0683c57f",
                    ),
                apiKeyProvider = { "test-api-key-not-for-prod" },
            )
        val place = client.searchPlace("Ferry Building San Francisco")
        assertEquals("places/ChIJ_test", place.id)
        assertEquals("Ferry Building", place.name)
        assertTrue(place.address.contains("San Francisco"))
        val recorded = server.takeRequest()
        assertEquals("POST", recorded.method)
        assertTrue(recorded.url.encodedPath.contains("places:searchText"))
        assertEquals("test-api-key-not-for-prod", recorded.headers["X-Goog-Api-Key"])
        assertEquals("app.codexlauncher", recorded.headers["X-Android-Package"])
        assertEquals("f7a85af51de481aa5c1d1211c0f18d1b0683c57f", recorded.headers["X-Android-Cert"])
        assertTrue(recorded.headers["X-Goog-FieldMask"]!!.contains("places.id"))
    }

    @Test
    fun searchPlaceUsesCallerSuppliedIdentityEvenWhenWrong() {
        val server = MockWebServer().also { servers += it; it.start() }
        server.enqueue(MockResponse.Builder().code(403).body("""{"error":{"status":"PERMISSION_DENIED"}}""").build())
        val client =
            MapsPlatformClient(
                placesBaseUrl = server.url("/").toString().trimEnd('/'),
                routesBaseUrl = server.url("/").toString().trimEnd('/'),
                http = OkHttpClient(),
                identity =
                    MapsAppIdentity(
                        packageName = "com.evil.spoof",
                        certSha1Hex = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
                    ),
                apiKeyProvider = { "test-api-key-not-for-prod" },
            )
        val err =
            runCatching { client.searchPlace("nowhere") }.exceptionOrNull()
        assertTrue(err is IllegalStateException)
        assertEquals("places_status_403", err?.message)
        val recorded = server.takeRequest()
        assertEquals("com.evil.spoof", recorded.headers["X-Android-Package"])
        assertEquals("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", recorded.headers["X-Android-Cert"])
    }

    @Test
    fun computeRouteParsesDistanceAndDuration() {
        val server = MockWebServer().also { servers += it; it.start() }
        server.enqueue(
            MockResponse.Builder()
                .code(200)
                .body(
                    """
                    {"routes":[{"distanceMeters":5400,"duration":"900s","description":"via US-101"}]}
                    """.trimIndent(),
                )
                .build(),
        )
        val client =
            MapsPlatformClient(
                placesBaseUrl = server.url("/").toString().trimEnd('/'),
                routesBaseUrl = server.url("/").toString().trimEnd('/'),
                http = OkHttpClient(),
                identity =
                    MapsAppIdentity(
                        packageName = "app.codexlauncher",
                        certSha1Hex = "f7a85af51de481aa5c1d1211c0f18d1b0683c57f",
                    ),
                apiKeyProvider = { "test-api-key-not-for-prod" },
            )
        val route = client.computeRoute("Origin", "Destination")
        assertEquals("via US-101", route.summary)
        assertEquals("5.4 km", route.distance)
        assertEquals("15 mins", route.duration)
        val recorded = server.takeRequest()
        assertTrue(recorded.url.encodedPath.contains("computeRoutes"))
        assertEquals("app.codexlauncher", recorded.headers["X-Android-Package"])
    }
}
