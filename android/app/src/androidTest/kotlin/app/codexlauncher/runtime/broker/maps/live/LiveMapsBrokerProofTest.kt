// Gate: importers=adb instrument live Maps proof on Pixel; callers=maps live
// orchestration; API=MapsImportSession + MapsPlatformClient Places/Routes;
// schemas=live-maps-broker-proof.txt; user: "Prove place + directions live on
// phone with durable evidence."
package app.codexlauncher.runtime.broker.maps.live

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import app.codexlauncher.runtime.broker.maps.http.MapsAppIdentity
import app.codexlauncher.runtime.broker.maps.http.MapsPlatformClient
import app.codexlauncher.runtime.broker.maps.vault.MapsApiKeyVault
import app.codexlauncher.runtime.broker.maps.vault.MapsImportSession
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File

@RunWith(AndroidJUnit4::class)
class LiveMapsBrokerProofTest {
    @Test
    fun importEnvelopeThenPlacesAndRoutes() {
        val ctx = InstrumentationRegistry.getInstrumentation().targetContext
        val args = InstrumentationRegistry.getArguments()
        val mode = args.getString("maps_mode") ?: "offer"
        val serial = args.getString("device_serial") ?: "unknown"
        when (mode) {
            "offer" -> {
                val offer = MapsImportSession.createOffer(ctx, deviceSerial = serial)
                val json = MapsImportSession.offerToJson(offer)
                File(ctx.filesDir, "maps-import-offer.json").writeText(json)
                runCatching { File("/sdcard/Download/maps-import-offer.json").writeText(json) }
                File(ctx.filesDir, "live-maps-broker-proof.txt").writeText("mode=offer\nimportId=${offer.importId}\n")
            }
            "import_and_call" -> {
                val envelopePath = args.getString("envelope_path")
                    ?: "/sdcard/Download/maps-import-envelope.json"
                val sealed = File(envelopePath).readText()
                MapsImportSession.importSealedJson(ctx, sealed)
                val vault = MapsApiKeyVault.android(ctx)
                assertTrue("vault must hold maps key after import", vault.hasKey())
                val identity =
                    MapsAppIdentity(
                        packageName = ctx.packageName,
                        certSha1Hex = MapsImportSession.signingCertSha1Hex(ctx),
                    )
                val client =
                    MapsPlatformClient(
                        identity = identity,
                        apiKeyProvider = {
                            vault.withKey { String(it, Charsets.UTF_8) }
                        },
                    )
                val place = client.searchPlace("Ferry Building San Francisco")
                assertTrue("place id required", place.id.isNotBlank())
                assertTrue("place name required", place.name.isNotBlank())
                val route =
                    client.computeRoute(
                        "Ferry Building, San Francisco, CA",
                        "Golden Gate Bridge, San Francisco, CA",
                    )
                assertTrue("route distance required", route.distance.isNotBlank())
                val text =
                    buildString {
                        appendLine("mode=import_and_call")
                        appendLine("place_id=${place.id}")
                        appendLine("place_name=${place.name}")
                        appendLine("place_address=${place.address}")
                        appendLine("route_summary=${route.summary}")
                        appendLine("route_distance=${route.distance}")
                        appendLine("route_duration=${route.duration}")
                        appendLine("verdict=PASS")
                    }
                File(ctx.filesDir, "live-maps-broker-proof.txt").writeText(text)
                runCatching { File("/sdcard/Download/live-maps-broker-proof.txt").writeText(text) }
            }
            "reject_wrong_package" -> {
                val vault = MapsApiKeyVault.android(ctx)
                assertTrue("vault must already hold maps key", vault.hasKey())
                val client =
                    MapsPlatformClient(
                        identity =
                            MapsAppIdentity(
                                packageName = "com.evil.spoof",
                                certSha1Hex = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
                            ),
                        apiKeyProvider = {
                            vault.withKey { String(it, Charsets.UTF_8) }
                        },
                    )
                val err = runCatching { client.searchPlace("Ferry Building San Francisco") }.exceptionOrNull()
                if (err == null) {
                    fail("expected Places reject for wrong package/cert")
                }
                val msg = err!!.message.orEmpty()
                assertTrue("expected places_status_* got $msg", msg.startsWith("places_status_"))
                val text =
                    "mode=reject_wrong_package\nerror=$msg\nverdict=PASS\n"
                File(ctx.filesDir, "live-maps-wrong-package-proof.txt").writeText(text)
                runCatching { File("/sdcard/Download/live-maps-wrong-package-proof.txt").writeText(text) }
            }
            else -> error("unknown maps_mode=$mode")
        }
    }
}
