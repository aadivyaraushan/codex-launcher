package app.codexlauncher.runtime.broker

import android.content.Context

object PendingOAuthStore {
    private const val PREFS = "pending_oauth"

    data class Pending(
        val provider: String,
        val clientId: String,
        val redirectUri: String,
        val codeVerifier: String,
        val state: String,
    )

    fun save(
        context: Context,
        pending: Pending,
    ) {
        context
            .getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            .edit()
            .putString("provider", pending.provider)
            .putString("clientId", pending.clientId)
            .putString("redirectUri", pending.redirectUri)
            .putString("codeVerifier", pending.codeVerifier)
            .putString("state", pending.state)
            .apply()
    }

    fun load(context: Context): Pending? {
        val p = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val provider = p.getString("provider", null) ?: return null
        val clientId = p.getString("clientId", null) ?: return null
        val redirectUri = p.getString("redirectUri", null) ?: return null
        val codeVerifier = p.getString("codeVerifier", null) ?: return null
        val state = p.getString("state", null) ?: return null
        return Pending(provider, clientId, redirectUri, codeVerifier, state)
    }

    fun clear(context: Context) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().clear().apply()
    }
}
