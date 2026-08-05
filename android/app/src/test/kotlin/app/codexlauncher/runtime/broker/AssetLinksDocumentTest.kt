package app.codexlauncher.runtime.broker

import org.junit.Assert.assertTrue
import org.junit.Test

class AssetLinksDocumentTest {
    @Test
    fun buildsStaticAssociationWithoutCallbackHandlers() {
        val json =
            AssetLinksDocument.build(
                packageName = "app.codexlauncher",
                sha256Fingerprints = listOf("AA:BB:CC:DD"),
            )
        assertTrue(json.contains("\"package_name\":\"app.codexlauncher\""))
        assertTrue(json.contains("AA:BB:CC:DD"))
        assertTrue(json.contains("delegate_permission/common.handle_all_urls"))
        assertTrue(!json.contains("oauth"))
        assertTrue(!json.contains("token"))
        assertTrue(!json.contains("redirect"))
    }
}
