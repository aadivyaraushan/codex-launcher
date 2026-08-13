package app.codexlauncher.runtime.modelauth

import app.codexlauncher.diagnostics.AppLog
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

data class HealthSnapshot(
    val taskCapable: Boolean = false,
    val modelAuth: ModelAuth = ModelAuth.Missing,
) {
    val gateOpen: Boolean
        get() = taskCapable && modelAuth == ModelAuth.OauthReady
}

data class DeviceLoginStart(
    val userCode: String,
    val verificationUrl: String,
)

enum class ModelAuth {
    OauthReady,
    Missing,
    Pending,
    ;

    companion object {
        private val json = Json { ignoreUnknownKeys = true }

        fun fromHealth(value: String?): ModelAuth =
            when (value?.trim()?.lowercase()) {
                "oauth_ready" -> OauthReady
                "pending" -> Pending
                else -> Missing
            }

        fun parseHealth(raw: String): HealthSnapshot {
            val obj = json.parseToJsonElement(raw).jsonObject
            // keyed:true is an API-key signal and must never lift the gate.
            val status = fromHealth(obj.string("modelAuth"))
            val taskCapable = obj["taskCapable"]?.jsonPrimitive?.booleanOrNull == true
            AppLog.info(
                feature = "model-auth",
                message = "parsed runtime health",
                fields =
                    mapOf(
                        "model_auth" to status.name.lowercase(),
                        "task_capable" to taskCapable,
                        "gate_open" to (taskCapable && status == OauthReady),
                    ),
            )
            return HealthSnapshot(taskCapable = taskCapable, modelAuth = status)
        }

        fun parseStart(raw: String): DeviceLoginStart {
            val obj = json.parseToJsonElement(raw).jsonObject
            val userCode = obj.string("userCode")?.trim().orEmpty()
            val verificationUrl = obj.string("verificationUrl")?.trim().orEmpty()
            require(userCode.isNotEmpty()) { "missing_user_code" }
            require(verificationUrl.startsWith("https://auth.openai.com/")) { "invalid_verification_url" }
            AppLog.info(
                feature = "model-auth",
                message = "parsed device-login start",
                fields =
                    mapOf(
                        "user_code_len" to userCode.length,
                        "verification_host" to "auth.openai.com",
                    ),
            )
            return DeviceLoginStart(userCode = userCode, verificationUrl = verificationUrl)
        }

        private fun JsonObject.string(key: String): String? = this[key]?.jsonPrimitive?.contentOrNull
    }
}
