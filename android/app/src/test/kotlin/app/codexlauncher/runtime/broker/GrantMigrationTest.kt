package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class GrantMigrationTest {
    @Test
    fun googleDesktopGrantNeverImportedAskPlayServices() {
        val decision =
            GrantMigration.decide(
                SourceGrantHealth(
                    provider = "google",
                    accountOrWorkspace = "aadivya.raushan@gmail.com",
                    clientId = "desktop-client",
                    scopes = setOf("calendar.events", "drive.file"),
                    tokenClass = "desktop_refresh",
                    expiryUnix = 9_999L,
                    healthPass = true,
                ),
            )
        assertEquals(GrantDisposition.DoNotImportAskPlatform, decision.disposition)
        assertTrue(decision.preserveSource)
        assertTrue(decision.notes.none { it.contains("import") && it.contains("refresh") })
    }

    @Test
    fun slackPortablePublicTokenMayEnvelopeImport() {
        val decision =
            GrantMigration.decide(
                SourceGrantHealth(
                    provider = "slack",
                    accountOrWorkspace = "ssdear",
                    clientId = "android-public-app",
                    scopes = setOf("chat:write", "im:write"),
                    tokenClass = "user_token_non_rotating_public",
                    expiryUnix = 0L,
                    healthPass = true,
                    androidCompatibleWithoutSecret = true,
                ),
            )
        assertEquals(GrantDisposition.EnvelopeImport, decision.disposition)
    }

    @Test
    fun slackIncompatiblePreservedWithSeparateAndroidConsent() {
        val decision =
            GrantMigration.decide(
                SourceGrantHealth(
                    provider = "slack",
                    accountOrWorkspace = "ssdear",
                    clientId = "legacy-secret-app",
                    scopes = setOf("chat:write"),
                    tokenClass = "user_token_needs_client_secret",
                    expiryUnix = 0L,
                    healthPass = true,
                    androidCompatibleWithoutSecret = false,
                ),
            )
        assertEquals(GrantDisposition.PreserveAndCreateAndroidGrant, decision.disposition)
        assertTrue(decision.preserveSource)
    }

    @Test
    fun failedHealthNeverImportsOrRefreshes() {
        val decision =
            GrantMigration.decide(
                SourceGrantHealth(
                    provider = "todoist",
                    accountOrWorkspace = "personal",
                    clientId = "pub",
                    scopes = setOf("data:read_write"),
                    tokenClass = "rotating_pair",
                    expiryUnix = 1L,
                    healthPass = false,
                    androidCompatibleWithoutSecret = true,
                ),
            )
        assertEquals(GrantDisposition.PreserveAndCreateAndroidGrant, decision.disposition)
        assertTrue(decision.preserveSource)
    }

    @Test
    fun healthCheckRecordOmitsSecrets() {
        val record =
            SourceGrantHealth(
                provider = "todoist",
                accountOrWorkspace = "personal",
                clientId = "pub-client",
                scopes = setOf("data:read_write"),
                tokenClass = "rotating_pair",
                expiryUnix = 100L,
                healthPass = true,
            ).toAuditRecord()
        assertTrue(!record.contains("token="))
        assertTrue(!record.contains("secret"))
        assertTrue(record.contains("provider=todoist"))
        assertTrue(record.contains("pass=true"))
    }
}
