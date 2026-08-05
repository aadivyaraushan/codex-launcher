// Gate: importers=GoogleAuthorizeActivity; callers=unit tests + authorize request;
// API=plan scopes calendar.events + drive.file; schemas=GoogleAuthOutcome;
// user: "Wire Google AuthorizationClient... Calendar/Drive consent"
package app.codexlauncher.runtime.broker.google

object GoogleAuthScopes {
    const val CALENDAR_EVENTS = "https://www.googleapis.com/auth/calendar.events"
    const val DRIVE_FILE = "https://www.googleapis.com/auth/drive.file"

    val operatorCalendarDrive: List<String> = listOf(CALENDAR_EVENTS, DRIVE_FILE)
}

sealed class GoogleAuthOutcome {
    data object NeedsUserConsent : GoogleAuthOutcome()

    data object Granted : GoogleAuthOutcome()

    data class Failed(val reason: String) : GoogleAuthOutcome()

    companion object {
        fun fromAuthorize(
            hasResolution: Boolean,
            accessTokenPresent: Boolean,
        ): GoogleAuthOutcome =
            when {
                hasResolution -> NeedsUserConsent
                accessTokenPresent -> Granted
                else -> Failed("missing_token")
            }
    }
}
