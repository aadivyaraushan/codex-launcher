package app.codexlauncher.runtime.broker

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class CredentialBrokerTest {
    @Test
    fun returnsReadyAccessWithoutLoggingSecret() {
        val store = InMemoryCredentialStore()
        store.put(
            CredentialRecord(
                adapterId = "todoist",
                scopes = setOf("data:read_write"),
                refreshToken = "refresh-secret",
                accessToken = "access-secret",
                accessExpiresAtUnix = 2_000L,
            ),
        )
        val broker = CredentialBroker(store)
        val logs = mutableListOf<String>()
        val response =
            broker.handle(
                CredentialRequest(
                    requestId = "r1",
                    adapterId = "todoist",
                    scopes = setOf("data:read_write"),
                    nowUnix = 1_500L,
                ),
                log = { logs += it },
            )
        assertTrue(response is CredentialResponse.Ready)
        val ready = response as CredentialResponse.Ready
        assertEquals("access-secret", ready.accessToken)
        assertEquals("r1", ready.requestId)
        assertTrue(logs.none { it.contains("access-secret") || it.contains("refresh-secret") })
        assertTrue(logs.any { it.contains("r1") && it.contains("todoist") })
    }

    @Test
    fun unavailableWhenAdapterMissing() {
        val broker = CredentialBroker(InMemoryCredentialStore())
        val response =
            broker.handle(
                CredentialRequest(
                    requestId = "r2",
                    adapterId = "todoist",
                    scopes = setOf("data:read_write"),
                    nowUnix = 1L,
                ),
            )
        assertTrue(response is CredentialResponse.Unavailable)
        assertEquals("missing_grant", (response as CredentialResponse.Unavailable).reason)
    }

    @Test
    fun refusesScopeEscalation() {
        val store = InMemoryCredentialStore()
        store.put(
            CredentialRecord(
                adapterId = "todoist",
                scopes = setOf("data:read"),
                refreshToken = "refresh-secret",
                accessToken = "access-secret",
                accessExpiresAtUnix = 2_000L,
            ),
        )
        val response =
            CredentialBroker(store).handle(
                CredentialRequest(
                    requestId = "r3",
                    adapterId = "todoist",
                    scopes = setOf("data:read_write"),
                    nowUnix = 1L,
                ),
            )
        assertTrue(response is CredentialResponse.Unavailable)
        assertEquals("scope_not_granted", (response as CredentialResponse.Unavailable).reason)
    }

    @Test
    fun expiredAccessMarkedRecoverableWithoutReturningValue() {
        val store = InMemoryCredentialStore()
        store.put(
            CredentialRecord(
                adapterId = "todoist",
                scopes = setOf("data:read_write"),
                refreshToken = "refresh-secret",
                accessToken = "old-access",
                accessExpiresAtUnix = 100L,
            ),
        )
        val response =
            CredentialBroker(store).handle(
                CredentialRequest(
                    requestId = "r4",
                    adapterId = "todoist",
                    scopes = setOf("data:read_write"),
                    nowUnix = 101L,
                ),
            )
        val unavailable = response as CredentialResponse.Unavailable
        assertEquals("access_expired", unavailable.reason)
        assertTrue(unavailable.recoverable)
        assertNull(store.debugDumpSecrets().firstOrNull { it.contains("old-access") && it.startsWith("returned:") })
    }


    @Test
    fun refreshesExpiredAccessWithoutLoggingRefreshToken() {
        val store = InMemoryCredentialStore()
        store.put(
            CredentialRecord(
                adapterId = "todoist",
                scopes = setOf("data:read_write"),
                refreshToken = "refresh-secret",
                accessToken = "old-access",
                accessExpiresAtUnix = 100L,
            ),
        )
        val logs = mutableListOf<String>()
        val broker =
            CredentialBroker(store) { record ->
                record.copy(accessToken = "new-access", accessExpiresAtUnix = 2_000L)
            }
        val response =
            broker.handle(
                CredentialRequest(
                    requestId = "r5",
                    adapterId = "todoist",
                    scopes = setOf("data:read_write"),
                    nowUnix = 101L,
                ),
                log = { logs += it },
            )
        val ready = response as CredentialResponse.Ready
        assertEquals("new-access", ready.accessToken)
        assertTrue(logs.none { it.contains("refresh-secret") })
        assertTrue(logs.any { it.contains("credential_refreshed") })
    }
}
