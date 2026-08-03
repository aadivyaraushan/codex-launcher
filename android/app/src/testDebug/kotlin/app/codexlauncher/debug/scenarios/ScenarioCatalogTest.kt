package app.codexlauncher.debug.scenarios

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ScenarioCatalogTest {
    @Test
    fun `catalog exposes every documented fixed scenario exactly once`() {
        val expected =
            setOf(
                "catalog",
                "pairing_qr_permission",
                "pairing_manual",
                "pairing_manual_error",
                "pairing_save_retry",
                "home_connecting",
                "home_syncing",
                "home_offline",
                "home_incompatible",
                "home_revoked",
                "home_online",
                "home_choose_project",
                "home_draft_error",
                "home_new_task_review",
                "home_new_task_error",
                "home_attachments",
                "project_choices",
                "project_empty",
                "project_save_retry",
                "task_loading",
                "task_unavailable",
                "task_transcript",
                "task_controls_working",
                "task_controls_idle",
                "task_control_unknown",
                "task_fork_unknown",
                "task_command_detail",
                "task_file_detail",
                "approval_full",
                "approval_redacted",
                "approval_sending",
                "question_choice",
                "question_free_text",
                "question_secret",
                "question_sending",
                "apps",
                "apps_launch_error",
                "appearance",
                "recovery",
                "dialog_attach",
                "dialog_background_warning",
                "dialog_unpair",
                // The four consent surfaces of a reply. Each one shipped with
                // "still unverified — on a real screen" against it in the plan,
                // because none of them was in this catalogue and the connected
                // audit only ever walks this catalogue.
                "reply_access_ask",
                "reply_stop_offer",
                "reply_stopped_list",
                "capability_confirm",
                // Everything the same sheet shows after the user says yes.
                // Two of these five exist to say "we do not know", which is
                // the hardest thing on the screen to get right and the last
                // thing that should go unseen on a real phone.
                "capability_running",
                "capability_result_unknown",
                "capability_failed",
                "capability_question",
                "capability_unresolved_check",
            )

        assertEquals(expected, ScenarioCatalog.all.map { it.wireName }.toSet())
        assertEquals(expected.size, ScenarioCatalog.all.size)
        assertTrue(ScenarioCatalog.all.all { it.expectedText.isNotBlank() })
    }

    @Test
    fun `parser accepts exact names and rejects caller controlled variations`() {
        assertEquals(ScenarioId.HOME_ONLINE, ScenarioCatalog.parse("home_online"))
        listOf(
            null,
            "",
            "HOME_ONLINE",
            " home_online",
            "home_online ",
            "home-online",
            "https://example.test/home_online",
            "{\"scenario\":\"home_online\"}",
            "a".repeat(1_024),
        ).forEach { candidate -> assertNull(candidate, ScenarioCatalog.parse(candidate)) }
    }
}
