package app.codexlauncher.appearance.theme

enum class AppearanceMode {
    FOLLOW_SYSTEM,
    LIGHT,
    DARK,
}

data class QuietPalette(
    val background: Long,
    val raisedSurface: Long,
    val primaryText: Long,
    val mutedText: Long,
    val hairline: Long,
    val signal: Long,
    val onSignal: Long,
    val success: Long,
    val warning: Long,
    val error: Long,
)

object QuietInstrumentTokens {
    val deepCharcoal =
        QuietPalette(
            background = 0xFF111210,
            raisedSurface = 0xFF1A1B18,
            primaryText = 0xFFF0EEE8,
            mutedText = 0xFF969990,
            hairline = 0xFF30312D,
            signal = 0xFFF06B3F,
            onSignal = 0xFF111210,
            success = 0xFF75A987,
            warning = 0xFFD49A3B,
            error = 0xFFE46F61,
        )

    val warmPaper =
        QuietPalette(
            background = 0xFFF4F2ED,
            raisedSurface = 0xFFEBE9E2,
            primaryText = 0xFF191A18,
            mutedText = 0xFF656861,
            hairline = 0xFFD8D7D1,
            signal = 0xFFE86136,
            onSignal = 0xFF191A18,
            success = 0xFF4D8061,
            warning = 0xFFB87816,
            error = 0xFFA33D32,
        )

    const val workingLabel = "Working"
    const val approvalLabel = "Approval needed"
    const val waitingLabel = "Needs your answer"
    const val repliedLabel = "Replied"
    const val failedLabel = "Failed"
    const val interruptedLabel = "Interrupted"
    const val oneTapLeftLabel = "One tap left"
    const val handedOffLabel = "Handed off"
    // The Unverified mark's TASK-row words. DESIGN.md (Core States, line 111;
    // amendment 2026-08-03) gives this one mark different words per surface:
    // beside a capability it reads "Unverified" (the enum's StateMark.label),
    // but on a task row — a lost run in a list you are scrolling past — it must
    // read "Couldn't confirm that happened". This constant is used only for the
    // home task row (HomeUiState.toHomeTask), so it carries the task phrasing;
    // the capability word stays on StateMark.UNVERIFIED.label.
    const val unverifiedLabel = "Couldn't confirm that happened"

    val spacingDp = listOf(4, 8, 12, 16, 24, 32)
    const val minimumTouchTargetDp = 44
    const val securityActionHeightDp = 48
    const val bodyTextSp = 13
    const val taskTitleSp = 15
}
