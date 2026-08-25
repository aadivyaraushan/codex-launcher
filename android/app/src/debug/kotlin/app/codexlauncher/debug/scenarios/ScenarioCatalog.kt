package app.codexlauncher.debug.scenarios

enum class ScenarioId(
    val wireName: String,
    val expectedText: String,
) {
    CATALOG("catalog", "UI audit scenarios"),
    PAIRING_QR_PERMISSION("pairing_qr_permission", "Camera access is used only to read the pairing code."),
    PAIRING_MANUAL("pairing_manual", "Pair computer"),
    PAIRING_MANUAL_ERROR("pairing_manual_error", "That pairing link isn't valid"),
    PAIRING_SAVE_RETRY("pairing_save_retry", "Retry saving"),
    HOME_CONNECTING("home_connecting", "Connecting"),
    HOME_SYNCING("home_syncing", "Syncing"),
    HOME_OFFLINE("home_offline", "Computer offline"),
    HOME_INCOMPATIBLE("home_incompatible", "Desktop integration needs an update"),
    HOME_REVOKED("home_revoked", "Pairing revoked"),
    HOME_ONLINE("home_online", "Sample task"),
    HOME_CHOOSE_PROJECT("home_choose_project", "Choose project"),
    HOME_DRAFT_ERROR("home_draft_error", "Draft could not be saved"),
    HOME_NEW_TASK_REVIEW("home_new_task_review", "Couldn't confirm that happened"),
    HOME_NEW_TASK_ERROR("home_new_task_error", "The computer could not start this task"),
    HOME_ATTACHMENTS("home_attachments", "sample-notes.txt"),
    PROJECT_CHOICES("project_choices", "Choose where Codex works"),
    PROJECT_EMPTY("project_empty", "No approved projects"),
    PROJECT_SAVE_RETRY("project_save_retry", "Retry saving"),
    TASK_LOADING("task_loading", "Loading task"),
    TASK_UNAVAILABLE("task_unavailable", "Task unavailable"),
    TASK_TRANSCRIPT("task_transcript", "Sample task"),
    TASK_CONTROLS_WORKING("task_controls_working", "Queue follow-up"),
    TASK_CONTROLS_IDLE("task_controls_idle", "Send follow-up"),
    TASK_CONTROL_UNKNOWN("task_control_unknown", "Sample task"),
    TASK_FORK_UNKNOWN("task_fork_unknown", "Couldn't confirm that happened"),
    TASK_COMMAND_DETAIL("task_command_detail", "Command output"),
    TASK_FILE_DETAIL("task_file_detail", "File change"),
    APPROVAL_FULL("approval_full", "Approval needed"),
    APPROVAL_REDACTED("approval_redacted", "Some command details could not be shown safely"),
    APPROVAL_SENDING("approval_sending", "Approval needed"),
    QUESTION_CHOICE("question_choice", "Codex needs your answer"),
    QUESTION_FREE_TEXT("question_free_text", "What should Codex check?"),
    QUESTION_SECRET("question_secret", "Answer on computer"),
    QUESTION_SENDING("question_sending", "Codex needs your answer"),
    APPS("apps", "All apps"),
    APPS_LAUNCH_ERROR("apps_launch_error", "App could not be opened"),
    APPEARANCE("appearance", "Appearance"),
    RECOVERY("recovery", "Finishing private data cleanup"),
    DIALOG_ATTACH("dialog_attach", "Attach"),
    DIALOG_BACKGROUND_WARNING("dialog_background_warning", "Background connection unavailable"),
    DIALOG_UNPAIR("dialog_unpair", "Remove computer"),
    // The three consent surfaces of a reply. Each one shipped unit-tested but
    // never seen on a real screen — the connected audit only walks this
    // catalogue, so nothing outside it has ever been rendered on a phone.
    REPLY_ACCESS_ASK("reply_access_ask", "Let Codex reply for you"),
    REPLY_STOP_OFFER("reply_stop_offer", "Stop replying to Maya"),
    REPLY_STOPPED_LIST("reply_stopped_list", "Turn replies back on"),
}

object ScenarioCatalog {
    val all: List<ScenarioId> = ScenarioId.entries.toList()

    private val byWireName = all.associateBy(ScenarioId::wireName)

    fun parse(candidate: String?): ScenarioId? = candidate?.let(byWireName::get)
}
