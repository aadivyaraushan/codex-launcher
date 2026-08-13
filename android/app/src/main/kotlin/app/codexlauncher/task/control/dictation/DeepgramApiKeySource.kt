// Gate: importers=PromptDictationEngine; callers=debug Pixel installs + unit tests;
// API=read() never returns a key on release builds; debug reads app-private file
// then BuildConfig; user: "API key must NOT be committed to git"
package app.codexlauncher.task.control.dictation

import android.content.Context
import app.codexlauncher.BuildConfig
import app.codexlauncher.diagnostics.AppLog
import java.io.File

class DeepgramApiKeySource(
    private val debugBuild: Boolean,
    private val compiledKey: String,
    private val privateKeyFile: File?,
) {
    fun read(): String? {
        if (!debugBuild) {
            AppLog.info(
                feature = "dictation",
                message = "deepgram key lookup skipped",
                fields = mapOf("decision" to "release_build", "key_present" to "false"),
            )
            return null
        }
        val fromFile = privateKeyFile?.takeIf { it.isFile }?.readText()?.trim().orEmpty()
        if (fromFile.isNotEmpty()) {
            AppLog.info(
                feature = "dictation",
                message = "deepgram key loaded",
                fields = mapOf("source" to "private_file", "key_present" to "true"),
            )
            return fromFile
        }
        val compiled = compiledKey.trim()
        if (compiled.isNotEmpty()) {
            AppLog.info(
                feature = "dictation",
                message = "deepgram key loaded",
                fields = mapOf("source" to "compiled", "key_present" to "true"),
            )
            return compiled
        }
        AppLog.info(
            feature = "dictation",
            message = "deepgram key missing",
            fields = mapOf("source" to "absent", "key_present" to "false"),
        )
        return null
    }

    companion object {
        const val PRIVATE_FILE_NAME = "deepgram_api_key"

        fun android(context: Context): DeepgramApiKeySource =
            DeepgramApiKeySource(
                debugBuild = BuildConfig.DEBUG,
                compiledKey = BuildConfig.DEEPGRAM_API_KEY,
                privateKeyFile = context.applicationContext.filesDir.resolve(PRIVATE_FILE_NAME),
            )
    }
}
