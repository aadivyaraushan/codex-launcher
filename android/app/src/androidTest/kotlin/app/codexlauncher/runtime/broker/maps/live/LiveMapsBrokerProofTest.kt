// Gate: importers=adb instrument live Maps proof on Pixel; callers=maps live
// orchestration; API=MapsImportSession + MapsPlatformClient Places/Routes;
// schemas=live-maps-broker-proof.txt; user: "Prove place + directions live on
// phone with durable evidence."
package app.codexlauncher.runtime.broker.maps.live

import android.content.Context
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
        val provider = args.getString("provider") ?: MapsImportSession.PROVIDER_MAPS
        when (mode) {
            "offer" -> {
                val offer =
                    MapsImportSession.createOffer(
                        ctx,
                        deviceSerial = serial,
                        provider = provider,
                    )
                val json = MapsImportSession.offerToJson(offer)
                val offerName =
                    if (provider == MapsImportSession.PROVIDER_OPENAI) {
                        "openai-import-offer.json"
                    } else {
                        "maps-import-offer.json"
                    }
                File(ctx.filesDir, offerName).writeText(json)
                runCatching { File("/sdcard/Download/$offerName").writeText(json) }
                File(ctx.filesDir, "live-maps-broker-proof.txt").writeText(
                    "mode=offer\nprovider=$provider\nimportId=${offer.importId}\n",
                )
            }
            "import_only" -> {
                // Prefer sealed_b64 / sealed_json args — release apps cannot
                // read adb-pushed /sdcard/Download files (scoped storage EACCES).
                val sealed =
                    args.getString("sealed_b64")?.let { b64 ->
                        String(android.util.Base64.decode(b64, android.util.Base64.DEFAULT), Charsets.UTF_8)
                    }
                        ?: args.getString("sealed_json")
                        ?: run {
                            val envelopePath =
                                args.getString("envelope_path")
                                    ?: File(ctx.filesDir, "openai-import-envelope.json").absolutePath
                            File(envelopePath).readText()
                        }
                MapsImportSession.importSealedJson(ctx, sealed)
                val vault =
                    if (provider == MapsImportSession.PROVIDER_OPENAI) {
                        MapsApiKeyVault.androidOpenAi(ctx)
                    } else {
                        MapsApiKeyVault.android(ctx)
                    }
                assertTrue("vault must hold $provider key after import", vault.hasKey())
                File(ctx.filesDir, "live-maps-broker-proof.txt").writeText(
                    "mode=import_only\nprovider=$provider\nhas_key=true\nverdict=PASS\n",
                )
                runCatching {
                    File("/sdcard/Download/live-maps-broker-proof.txt").writeText(
                        "mode=import_only\nprovider=$provider\nhas_key=true\nverdict=PASS\n",
                    )
                }
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
            "clear_local_runtime" -> {
                // Dogfood: drop loopback session endpoint prefs so silent
                // local-pair can re-enroll after phone-runtime paired_devices reset.
                val cleared =
                    ctx.getSharedPreferences("local_pair_runtime", Context.MODE_PRIVATE)
                        .edit()
                        .clear()
                        .commit()
                assertTrue("local_pair_runtime prefs clear", cleared)
                File(ctx.filesDir, "live-maps-broker-proof.txt").writeText(
                    "mode=clear_local_runtime\ncleared=true\nverdict=PASS\n",
                )
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
