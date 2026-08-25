package app.codexlauncher.runtime.broker

data class CredentialRecord(
    val adapterId: String,
    val scopes: Set<String>,
    val refreshToken: String,
    val accessToken: String,
    val accessExpiresAtUnix: Long,
)

data class CredentialRequest(
    val requestId: String,
    val adapterId: String,
    val scopes: Set<String>,
    val nowUnix: Long,
)

sealed class CredentialResponse {
    data class Ready(
        val requestId: String,
        val accessToken: String,
        val expiresAtUnix: Long,
    ) : CredentialResponse()

    data class Unavailable(
        val requestId: String,
        val reason: String,
        val recoverable: Boolean,
    ) : CredentialResponse()
}

class InMemoryCredentialStore {
    private val records = linkedMapOf<String, CredentialRecord>()

    fun put(record: CredentialRecord) {
        records[record.adapterId] = record
    }

    fun get(adapterId: String): CredentialRecord? = records[adapterId]

    fun debugDumpSecrets(): List<String> = emptyList()
}

class CredentialBroker(
    private val store: InMemoryCredentialStore,
    private val refreshAccess: ((CredentialRecord) -> CredentialRecord?)? = null,
) {
    fun handle(
        request: CredentialRequest,
        log: (String) -> Unit = {},
    ): CredentialResponse {
        log("[broker] credential_request id=${request.requestId} adapter=${request.adapterId} scopes=${request.scopes.size}")
        var record = store.get(request.adapterId)
        if (record == null) {
            log("[broker] credential_unavailable id=${request.requestId} reason=missing_grant")
            return CredentialResponse.Unavailable(request.requestId, "missing_grant", recoverable = true)
        }
        if (!record.scopes.containsAll(request.scopes)) {
            log("[broker] credential_unavailable id=${request.requestId} reason=scope_not_granted")
            return CredentialResponse.Unavailable(request.requestId, "scope_not_granted", recoverable = false)
        }
        if (request.nowUnix >= record.accessExpiresAtUnix) {
            log("[broker] credential_access_expired id=${request.requestId} attempting_refresh=true")
            val refreshed = refreshAccess?.invoke(record)
            if (refreshed == null || request.nowUnix >= refreshed.accessExpiresAtUnix) {
                log("[broker] credential_unavailable id=${request.requestId} reason=access_expired recoverable=true")
                return CredentialResponse.Unavailable(request.requestId, "access_expired", recoverable = true)
            }
            store.put(refreshed)
            record = refreshed
            log("[broker] credential_refreshed id=${request.requestId} expires_at=${record.accessExpiresAtUnix}")
        }
        log("[broker] credential_ready id=${request.requestId} expires_at=${record.accessExpiresAtUnix}")
        return CredentialResponse.Ready(
            requestId = request.requestId,
            accessToken = record.accessToken,
            expiresAtUnix = record.accessExpiresAtUnix,
        )
    }
}

enum class GrantDisposition {
    DoNotImportAskPlatform,
    EnvelopeImport,
    PreserveAndCreateAndroidGrant,
}

data class SourceGrantHealth(
    val provider: String,
    val accountOrWorkspace: String,
    val clientId: String,
    val scopes: Set<String>,
    val tokenClass: String,
    val expiryUnix: Long,
    val healthPass: Boolean,
    val androidCompatibleWithoutSecret: Boolean = false,
) {
    fun toAuditRecord(): String =
        "provider=$provider account=$accountOrWorkspace clientId=$clientId " +
            "scopes=${scopes.sorted().joinToString(",")} tokenClass=$tokenClass " +
            "expiryUnix=$expiryUnix pass=$healthPass androidCompatible=$androidCompatibleWithoutSecret"
}

data class GrantMigrationDecision(
    val disposition: GrantDisposition,
    val preserveSource: Boolean,
    val notes: List<String> = emptyList(),
)

object GrantMigration {
    fun decide(health: SourceGrantHealth): GrantMigrationDecision {
        if (!health.healthPass) {
            return GrantMigrationDecision(
                disposition = GrantDisposition.PreserveAndCreateAndroidGrant,
                preserveSource = true,
                notes = listOf("source_health_failed"),
            )
        }
        return when (health.provider) {
            "google", "microsoft" ->
                GrantMigrationDecision(
                    disposition = GrantDisposition.DoNotImportAskPlatform,
                    preserveSource = true,
                    notes = listOf("platform_owned_cache"),
                )
            "slack", "spotify", "notion", "todoist" ->
                if (health.androidCompatibleWithoutSecret) {
                    GrantMigrationDecision(
                        disposition = GrantDisposition.EnvelopeImport,
                        preserveSource = true,
                        notes = listOf("compatible_public_grant"),
                    )
                } else {
                    GrantMigrationDecision(
                        disposition = GrantDisposition.PreserveAndCreateAndroidGrant,
                        preserveSource = true,
                        notes = listOf("incompatible_preserve_source"),
                    )
                }
            else ->
                GrantMigrationDecision(
                    disposition = GrantDisposition.PreserveAndCreateAndroidGrant,
                    preserveSource = true,
                    notes = listOf("unknown_provider"),
                )
        }
    }
}

sealed class UsageDecision {
    data object Allow : UsageDecision()

    data class Pause(
        val reason: String,
    ) : UsageDecision()
}

data class OpenAIUsageLedger(
    val monthKey: String,
    val spentUsdCents: Int,
    val lastRunId: String? = null,
    val lastModel: String? = null,
    val lastRequestCount: Int = 0,
    val lastTokens: Int = 0,
) {
    fun authorize(
        estimatedUsdCents: Int,
        ceilingUsdCents: Int,
    ): UsageDecision {
        if (spentUsdCents + estimatedUsdCents > ceilingUsdCents) {
            return UsageDecision.Pause(reason = "monthly_ceiling")
        }
        return UsageDecision.Allow
    }

    fun record(
        runId: String,
        model: String,
        requestCount: Int,
        tokens: Int,
        costUsdCents: Int,
    ): OpenAIUsageLedger =
        copy(
            spentUsdCents = spentUsdCents + costUsdCents,
            lastRunId = runId,
            lastModel = model,
            lastRequestCount = requestCount,
            lastTokens = tokens,
        )

    fun toAuditRecord(): String =
        "month=$monthKey spentUsdCents=$spentUsdCents run=${lastRunId ?: "-"} " +
            "model=${lastModel ?: "-"} requests=$lastRequestCount tokens=$lastTokens"
}

enum class DomainState {
    VERIFIED,
    SELECTED,
    NONE,
}

data class DomainVerificationSnapshot(
    val hostToState: Map<String, DomainState>,
    val isLinkHandlingAllowed: Boolean,
    val soleDefaultCallbackPackage: String?,
)

sealed class DomainGateResult {
    data object Allow : DomainGateResult()

    data class Block(
        val reason: String,
    ) : DomainGateResult()
}

class DomainVerificationGate(
    private val requiredHost: String = "tryoperator.net",
    private val operatorPackage: String = "app.codexlauncher",
) {
    fun evaluate(snapshot: DomainVerificationSnapshot): DomainGateResult {
        if (snapshot.hostToState[requiredHost] != DomainState.VERIFIED) {
            return DomainGateResult.Block(reason = "host_not_verified")
        }
        if (!snapshot.isLinkHandlingAllowed) {
            return DomainGateResult.Block(reason = "link_handling_disabled")
        }
        if (snapshot.soleDefaultCallbackPackage != operatorPackage) {
            return DomainGateResult.Block(reason = "callback_not_sole_operator")
        }
        return DomainGateResult.Allow
    }
}

object AssetLinksDocument {
    fun build(
        packageName: String,
        sha256Fingerprints: List<String>,
    ): String {
        val fps = sha256Fingerprints.joinToString(",") { "\"$it\"" }
        return "[" +
            "{" +
            "\"relation\":[\"delegate_permission/common.handle_all_urls\"]," +
            "\"target\":{" +
            "\"namespace\":\"android_app\"," +
            "\"package_name\":\"$packageName\"," +
            "\"sha256_cert_fingerprints\":[$fps]" +
            "}" +
            "}" +
            "]"
    }
}

data class OAuthCallback(
    val provider: String,
    val code: String,
    val state: String,
) {
    fun toAuditRecord(): String = "provider=$provider state=$state code_present=${code.isNotEmpty()}"
}

object OAuthCallbackParse {
    fun parse(uriString: String): OAuthCallback {
        val uri = java.net.URI(uriString)
        // The exported callback activity is registered only for the https App Link
        // (AndroidManifest: scheme="https" host="tryoperator.net"). Gate the scheme
        // too, so a hostile app cannot reach this parser with http:// or a custom
        // scheme carrying a forged code/state. Defense in depth behind autoVerify.
        require(uri.scheme == "https") { "wrong_scheme" }
        require(uri.host == "tryoperator.net") { "wrong_host" }
        val path = uri.path.orEmpty()
        require(path.startsWith("/oauth/android/")) { "wrong_path" }
        val provider = path.removePrefix("/oauth/android/").trim('/')
        require(provider == "todoist" || provider == "notion") { "unknown_provider" }
        val query = uri.rawQuery.orEmpty()
        val params =
            query.split('&').mapNotNull { part ->
                val idx = part.indexOf('=')
                if (idx <= 0) {
                    null
                } else {
                    java.net.URLDecoder.decode(part.substring(0, idx), "UTF-8") to
                        java.net.URLDecoder.decode(part.substring(idx + 1), "UTF-8")
                }
            }.toMap()
        val code = params["code"].orEmpty()
        val state = params["state"].orEmpty()
        require(code.isNotEmpty()) { "missing_code" }
        require(state.isNotEmpty()) { "missing_state" }
        return OAuthCallback(provider = provider, code = code, state = state)
    }
}


object TodoistPublicClientRegistration {
    fun build(
        redirectUri: String,
        clientName: String,
    ): String =
        "{" +
            "\"client_name\":\"$clientName\"," +
            "\"redirect_uris\":[\"$redirectUri\"]," +
            "\"scope\":\"data:read_write\"," +
            "\"grant_types\":[\"authorization_code\",\"refresh_token\"]," +
            "\"response_types\":[\"code\"]," +
            "\"token_endpoint_auth_method\":\"none\"" +
            "}"

    fun authorizationUrl(
        clientId: String,
        redirectUri: String,
        state: String,
        codeChallenge: String,
    ): String {
        val enc = java.net.URLEncoder.encode(redirectUri, "UTF-8")
        // Docs source: Context7 /websites/developer_todoist_api_v1 (app.todoist.com authorize).
        return "https://app.todoist.com/oauth/authorize?" +
            "client_id=$clientId&response_type=code&scope=data:read_write" +
            "&redirect_uri=$enc&state=$state" +
            "&code_challenge=$codeChallenge&code_challenge_method=S256"
    }

    fun tokenExchangeForm(
        clientId: String,
        code: String,
        redirectUri: String,
        codeVerifier: String,
    ): String {
        val enc = java.net.URLEncoder.encode(redirectUri, "UTF-8")
        return "client_id=$clientId&code=$code&redirect_uri=$enc&code_verifier=$codeVerifier"
    }
}


object PkceS256 {
    fun newVerifier(random: java.security.SecureRandom = java.security.SecureRandom()): String {
        val bytes = ByteArray(32)
        random.nextBytes(bytes)
        return java.util.Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)
    }

    fun challengeS256(verifier: String): String {
        val digest = java.security.MessageDigest.getInstance("SHA-256").digest(verifier.toByteArray(Charsets.US_ASCII))
        return java.util.Base64.getUrlEncoder().withoutPadding().encodeToString(digest)
    }
}


object NotionPublicClientRegistration {
    fun build(
        redirectUri: String,
        clientName: String,
    ): String =
        "{" +
            "\"client_name\":\"$clientName\"," +
            "\"redirect_uris\":[\"$redirectUri\"]," +
            "\"token_endpoint_auth_method\":\"none\"," +
            "\"grant_types\":[\"authorization_code\",\"refresh_token\"]," +
            "\"response_types\":[\"code\"]" +
            "}"

    fun failsClosedUnlessDomainVerified(gate: DomainGateResult): Boolean =
        gate is DomainGateResult.Allow
}


data class MapsBillingSnapshot(
    val project: String,
    val placesCallsUsed: Int,
    val routesCallsUsed: Int,
    val freePlacesRemaining: Int,
    val freeRoutesRemaining: Int,
)

sealed class MapsBatchDecision {
    data object AllowFourCallBatch : MapsBatchDecision()
    data class Pause(val reason: String) : MapsBatchDecision()
}

object MapsZeroChargeGate {
    fun authorize(snapshot: MapsBillingSnapshot, plannedPlaces: Int = 1, plannedRoutes: Int = 1, plannedRejectProbes: Int = 2): MapsBatchDecision {
        val total = plannedPlaces + plannedRoutes + plannedRejectProbes
        if (total != 4) return MapsBatchDecision.Pause("batch_must_be_four")
        if (snapshot.project != "Operator") return MapsBatchDecision.Pause("wrong_project")
        if (snapshot.freePlacesRemaining < plannedPlaces) return MapsBatchDecision.Pause("places_allowance")
        if (snapshot.freeRoutesRemaining < plannedRoutes) return MapsBatchDecision.Pause("routes_allowance")
        return MapsBatchDecision.AllowFourCallBatch
    }
}

enum class RuntimeFailureKind {
    Starting,
    Healthy,
    Degraded,
    SignedOut,
    ServiceOffline,
    NetworkOffline,
    ForceStopped,
    DeliveryUnknown,
}

object RuntimeFailureCopy {
    fun message(kind: RuntimeFailureKind): String =
        when (kind) {
            RuntimeFailureKind.Starting -> "Codex services are starting."
            RuntimeFailureKind.Healthy -> "Codex services are ready."
            RuntimeFailureKind.Degraded -> "Codex services are degraded. Retry after recovery finishes."
            RuntimeFailureKind.SignedOut -> "A service signed out. Open Codex settings to reconnect."
            RuntimeFailureKind.ServiceOffline -> "A local service is offline. Codex will retry automatically."
            RuntimeFailureKind.NetworkOffline -> "No network connection. Retry when connectivity returns."
            RuntimeFailureKind.ForceStopped -> "Android has force-stopped Codex services. Open Termux once to restore."
            RuntimeFailureKind.DeliveryUnknown -> "Delivery is unknown. Codex will not resend automatically."
        }
}


data class AuthCallbackRoute(
    val provider: String,
    val pathPrefix: String,
    val method: String,
)

object AuthCallbackRoutes {
    val table: List<AuthCallbackRoute> =
        listOf(
            AuthCallbackRoute("todoist", "/oauth/android/todoist", "app_link"),
            AuthCallbackRoute("notion", "/oauth/android/notion", "app_link"),
            AuthCallbackRoute("google", "play_services_authorization_client", "authorization_client"),
            AuthCallbackRoute("microsoft", "msauth", "msal"),
            AuthCallbackRoute("slack", "custom_scheme_pkce", "custom_scheme"),
            AuthCallbackRoute("spotify", "custom_scheme_pkce", "custom_scheme"),
            AuthCallbackRoute("openai", "keystore_only", "broker_direct"),
            AuthCallbackRoute("beeper", "127.0.0.1:23373", "local_oauth"),
        )

    fun requireKnown(provider: String): AuthCallbackRoute =
        table.first { it.provider == provider }
}


data class SlackAuthTestResult(
    val ok: Boolean,
    val user: String,
    val team: String,
    val appId: String,
    val tokenType: String,
)

object SlackSourceHealth {
    fun auditWithoutRefresh(result: SlackAuthTestResult): String {
        require(!result.ok || result.user.isNotBlank()) { "missing_user" }
        return "provider=slack pass=${result.ok} user=${result.user} team=${result.team} appId=${result.appId} tokenType=${result.tokenType}"
    }
}

sealed class OutlookAcquireDecision {
    data object Silent : OutlookAcquireDecision()
    data object InteractiveConsent : OutlookAcquireDecision()
}

object OutlookAcquireGate {
    fun decide(silentAccountFound: Boolean, silentSucceeded: Boolean): OutlookAcquireDecision {
        if (silentAccountFound && silentSucceeded) return OutlookAcquireDecision.Silent
        return OutlookAcquireDecision.InteractiveConsent
    }
}


object InScopeAppPackages {
    val required: Map<String, String> =
        mapOf(
            "instagram" to "com.instagram.android",
            "discord" to "com.discord",
            "google_messages" to "com.google.android.apps.messaging",
            "slack" to "com.Slack",
            "outlook" to "com.microsoft.office.outlook",
            "calendar" to "com.google.android.calendar",
            "drive" to "com.google.android.apps.docs",
            "spotify" to "com.spotify.music",
            "notion" to "notion.id",
            "todoist" to "com.todoist",
            "maps" to "com.google.android.apps.maps",
            "youtube" to "com.google.android.youtube",
        )

    fun missing(installed: Set<String>): List<String> =
        required.filterValues { it !in installed }.keys.sorted()
}


object OpenAIAccountLock {
    const val APPROVED_ORG = "org-oC0Cx9jwKVEEvRlRlqdQzTwE"
    const val APPROVED_MONTHLY_CEILING_USD_CENTS = 50_000

    fun refuseSwitch(orgId: String): Boolean = orgId != APPROVED_ORG
}
