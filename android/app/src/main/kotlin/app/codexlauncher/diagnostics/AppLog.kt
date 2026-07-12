package app.codexlauncher.diagnostics

import android.util.Log

object AppLog {
    enum class Level { DEBUG, INFO, WARN, ERROR }

    private const val MAX_VALUE_LENGTH = 256
    private val sensitiveKeyParts = setOf(
        "attachment",
        "authorization",
        "command",
        "credential",
        "file",
        "path",
        "prompt",
        "secret",
        "token",
    )

    fun format(
        feature: String,
        level: Level,
        message: String,
        fields: Map<String, Any?> = emptyMap(),
    ): String {
        val prefix = "[${singleLine(feature)}] level=${level.name} message=${singleLine(message)}"
        if (fields.isEmpty()) return prefix

        val context = fields.toSortedMap().entries.joinToString(" ") { (key, value) ->
            "${singleLine(key)}=${safeValue(key, value)}"
        }
        return "$prefix $context"
    }

    fun info(feature: String, message: String, fields: Map<String, Any?> = emptyMap()) {
        Log.i(tag(feature), format(feature, Level.INFO, message, fields))
    }

    fun error(
        feature: String,
        message: String,
        error: Throwable,
        fields: Map<String, Any?> = emptyMap(),
    ) {
        val errorFields = fields + safeErrorFields(error)
        Log.e(tag(feature), format(feature, Level.ERROR, message, errorFields))
    }

    internal fun safeErrorFields(error: Throwable): Map<String, String> {
        val errorMessage = error.message
        return mapOf(
            "error_type" to error::class.java.simpleName,
            "error_message_shape" to if (errorMessage == null) {
                "absent"
            } else {
                "present,length=${errorMessage.length}"
            },
        )
    }

    private fun safeValue(key: String, value: Any?): String {
        val normalizedKey = key.lowercase()
        if (sensitiveKeyParts.any(normalizedKey::contains)) return "[REDACTED]"

        val normalizedValue = singleLine(value?.toString() ?: "null")
        return if (normalizedValue.length > MAX_VALUE_LENGTH) {
            "[TRUNCATED length=${normalizedValue.length}]"
        } else {
            normalizedValue
        }
    }

    private fun singleLine(value: String): String = value.replace(Regex("[\\r\\n\\t]+"), " ").trim()

    private fun tag(feature: String): String = "CodexLauncher/${singleLine(feature).take(20)}"
}
