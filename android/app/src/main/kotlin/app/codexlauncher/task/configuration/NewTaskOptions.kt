package app.codexlauncher.task.configuration

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

data class NewTaskOptions(
    val models: List<TaskModelOption>,
    val permissionModes: List<PermissionModeOption>,
) {
    fun defaultSelection(): NewTaskSelection {
        val model = models.single { it.isDefault }
        val permission = permissionModes.single { it.isDefault }
        return NewTaskSelection(model.id, model.defaultReasoningId, permission.id)
    }

    fun normalize(selection: NewTaskSelection): NewTaskSelection {
        val model = models.firstOrNull { it.id == selection.modelId } ?: models.single { it.isDefault }
        val reasoningId = selection.reasoningId.takeIf { chosen -> model.reasoning.any { it.id == chosen } } ?: model.defaultReasoningId
        val permissionId = selection.permissionModeId.takeIf { chosen -> permissionModes.any { it.id == chosen } }
            ?: permissionModes.single { it.isDefault }.id
        return NewTaskSelection(model.id, reasoningId, permissionId)
    }

    companion object {
        fun fromWelcome(body: JsonObject): NewTaskOptions? =
            body["newTaskOptions"]?.jsonObject?.let { options ->
                NewTaskOptions(
                    models =
                        options.getValue("models").jsonArray.map { raw ->
                            val model = raw.jsonObject
                            TaskModelOption(
                                id = model.string("id"),
                                displayName = model.string("displayName"),
                                isDefault = model.getValue("isDefault").jsonPrimitive.boolean,
                                defaultReasoningId = model.string("defaultReasoningId"),
                                reasoning =
                                    model.getValue("reasoning").jsonArray.map { rawReasoning ->
                                        val reasoning = rawReasoning.jsonObject
                                        ReasoningOption(reasoning.string("id"), reasoning.string("displayName"), reasoning.string("description"))
                                    },
                            )
                        },
                    permissionModes =
                        options.getValue("permissionModes").jsonArray.map { raw ->
                            val mode = raw.jsonObject
                            PermissionModeOption(
                                id = mode.string("id"),
                                displayName = mode.string("displayName"),
                                description = mode.string("description"),
                                isDefault = mode.getValue("isDefault").jsonPrimitive.boolean,
                            )
                        },
                )
            }
    }
}

data class TaskModelOption(
    val id: String,
    val displayName: String,
    val isDefault: Boolean,
    val defaultReasoningId: String,
    val reasoning: List<ReasoningOption>,
)

data class ReasoningOption(val id: String, val displayName: String, val description: String)

data class PermissionModeOption(val id: String, val displayName: String, val description: String, val isDefault: Boolean)

data class NewTaskSelection(val modelId: String, val reasoningId: String, val permissionModeId: String)

private fun JsonObject.string(name: String): String = getValue(name).jsonPrimitive.content
