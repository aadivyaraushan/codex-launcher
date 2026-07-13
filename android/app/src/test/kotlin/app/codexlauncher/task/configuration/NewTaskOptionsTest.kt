package app.codexlauncher.task.configuration

import app.codexlauncher.connection.protocol.ProtocolCodec
import org.junit.Assert.assertEquals
import org.junit.Test

class NewTaskOptionsTest {
    @Test
    fun `welcome catalog maps opaque ids and host labels`() {
        val welcome = ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"welcome-options","sender":"companion","type":"welcome","body":{"sessionId":"s","capabilities":["new_task_options"],"limits":{"maxJsonBytes":262144,"maxAttachmentBytes":20971520,"maxDeviceUploads":2,"maxGlobalUploads":4,"maxTemporaryBytes":104857600,"uploadExpirySeconds":900},"newTaskOptions":{"models":[{"id":"codex-1","displayName":"Codex 1","isDefault":true,"defaultReasoningId":"medium","reasoning":[{"id":"medium","displayName":"Medium","description":"Balanced."},{"id":"high","displayName":"High","description":"Deeper."}]}],"permissionModes":[{"id":"read-only","displayName":"Read only","description":"No changes.","isDefault":false},{"id":"workspace-write","displayName":"Workspace","description":"Project changes.","isDefault":true}]}}}""",
        )

        val options = NewTaskOptions.fromWelcome(welcome.body)!!

        assertEquals("Codex 1", options.models.single().displayName)
        assertEquals("medium", options.defaultSelection().reasoningId)
        assertEquals("workspace-write", options.defaultSelection().permissionModeId)
    }

    @Test
    fun `changing model resets an unavailable reasoning choice to the host default`() {
        val options =
            NewTaskOptions(
                models =
                    listOf(
                        TaskModelOption("model-a", "Model A", true, "high", listOf(ReasoningOption("high", "High", "Deep."))),
                        TaskModelOption("model-b", "Model B", false, "low", listOf(ReasoningOption("low", "Low", "Fast."))),
                    ),
                permissionModes = listOf(PermissionModeOption("workspace-write", "Workspace", "Project changes.", true)),
            )

        assertEquals(
            NewTaskSelection("model-b", "low", "workspace-write"),
            options.normalize(NewTaskSelection("model-b", "high", "workspace-write")),
        )
    }
}
