package app.codexlauncher.diagnostics

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AppLogTest {
    @Test
    fun `redacts work content and credentials but keeps diagnostic context`() {
        val output = AppLog.format(
            feature = "connection",
            level = AppLog.Level.INFO,
            message = "request prepared",
            fields = mapOf(
                "task_id" to "task-42",
                "input_shape" to "action(start, attachment_count=1)",
                "branch_reason" to "fresh snapshot required",
                "error_type" to "none",
                "prompt" to "private prompt text",
                "command_body" to "rm -rf private-project",
                "workspace_path" to "/Users/person/private-project",
                "access_token" to "token-value",
                "pairing_secret" to "pairing-value",
            ),
        )

        assertTrue(output.contains("[connection]"))
        assertTrue(output.contains("task_id=task-42"))
        assertTrue(output.contains("input_shape=action(start, attachment_count=1)"))
        assertTrue(output.contains("branch_reason=fresh snapshot required"))
        assertTrue(output.contains("error_type=none"))
        assertTrue(output.contains("prompt=[REDACTED]"))
        assertTrue(output.contains("command_body=[REDACTED]"))
        assertTrue(output.contains("workspace_path=[REDACTED]"))
        assertTrue(output.contains("access_token=[REDACTED]"))
        assertTrue(output.contains("pairing_secret=[REDACTED]"))
        assertFalse(output.contains("private-project"))
        assertFalse(output.contains("token-value"))
        assertFalse(output.contains("pairing-value"))
    }

    @Test
    fun `normalizes line breaks and truncates large values`() {
        val largeValue = "x".repeat(600)
        val output = AppLog.format(
            feature = "task-control\nspoofed",
            level = AppLog.Level.ERROR,
            message = "failed\nto send",
            fields = mapOf("result_shape" to largeValue),
        )

        assertFalse(output.contains("\n"))
        assertTrue(output.contains("[task-control spoofed]"))
        assertTrue(output.contains("message=failed to send"))
        assertTrue(output.contains("result_shape=[TRUNCATED length=600]"))
    }

    @Test
    fun `caught error context describes the exception without logging its message`() {
        val output = AppLog.format(
            feature = "pairing",
            level = AppLog.Level.ERROR,
            message = "pairing failed",
            fields = AppLog.safeErrorFields(
                IllegalStateException("token abc at /Users/person/private-project"),
            ),
        )

        assertTrue(output.contains("error_type=IllegalStateException"))
        assertTrue(output.contains("error_message_shape=present,length=42"))
        assertFalse(output.contains("token abc"))
        assertFalse(output.contains("private-project"))
    }
}
