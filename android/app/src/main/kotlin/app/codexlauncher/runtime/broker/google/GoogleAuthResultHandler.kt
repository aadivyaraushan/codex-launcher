// Gate: importers=GoogleAuthorizeActivity; callers=unit tests + activity result path;
// API=decide ParseIntent vs MarkDenied; recreate must not re-authorize while awaiting;
// schemas=Decision; user: "I think the Google OAuth path is buggy in some way. Please fix."
package app.codexlauncher.runtime.broker.google

object GoogleAuthResultHandler {
    enum class NextStep {
        ParseIntent,
        MarkDenied,
    }

    data class Decision(
        val next: NextStep,
        val terminalStatus: String? = null,
        val detail: String? = null,
        val retryAuthorize: Boolean = false,
    )

    /**
     * Google's Identity sample always calls getAuthorizationResultFromIntent on
     * the activity result. It does not gate on RESULT_OK. Play Services has been
     * observed returning RESULT_CANCELED (0) with a usable result Intent after
     * account/consent UI — rejecting that path marks Operator denied while Google
     * already regranted scopes.
     */
    fun onActivityResult(
        resultCode: Int,
        intentDataPresent: Boolean,
    ): Decision {
        if (intentDataPresent) {
            return Decision(next = NextStep.ParseIntent)
        }
        if (resultCode == android.app.Activity.RESULT_OK) {
            return Decision(next = NextStep.ParseIntent)
        }
        return Decision(
            next = NextStep.MarkDenied,
            terminalStatus = "denied",
            detail = "user_cancelled_or_failed",
        )
    }

    fun afterParsedResult(
        accessTokenPresent: Boolean,
        authorizedScopeCount: Int,
    ): Decision {
        if (accessTokenPresent) {
            return Decision(
                next = NextStep.ParseIntent,
                terminalStatus = "granted",
                detail = "play_services_cached",
                retryAuthorize = false,
            )
        }
        if (authorizedScopeCount > 0) {
            return Decision(
                next = NextStep.ParseIntent,
                terminalStatus = "awaiting_token",
                detail = "retry_authorize_for_token",
                retryAuthorize = true,
            )
        }
        return Decision(
            next = NextStep.MarkDenied,
            terminalStatus = "error",
            detail = "missing_token",
            retryAuthorize = false,
        )
    }

    /**
     * Recreating GoogleAuthorizeActivity while the PendingIntent UI is up must
     * not call authorize() again — that resets prefs to pending and can launch a
     * second chooser that races the first result.
     */
    fun shouldStartAuthorizeOnCreate(
        isRecreate: Boolean,
        awaitingResolution: Boolean,
    ): Boolean {
        if (awaitingResolution) return false
        if (isRecreate) return false
        return true
    }
}
