// Gate: importers=PromptDictationEngine; callers=unit tests + tap-to-talk stop path;
// API=POST /v1/listen with Authorization Token and audio/mp4 body; schemas=
// results.channels[0].alternatives[0].transcript; user: "Replace Operator Google
// RecognizerIntent mic with Deepgram transcription"
package app.codexlauncher.task.control.dictation

import app.codexlauncher.diagnostics.AppLog
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.IOException
import java.util.concurrent.TimeUnit

sealed interface DeepgramListenOutcome {
    data class Transcript(val text: String) : DeepgramListenOutcome

    data object Empty : DeepgramListenOutcome

    data object NetworkError : DeepgramListenOutcome

    data object Rejected : DeepgramListenOutcome
}

class DeepgramListenClient(
    private val baseUrl: String = DEFAULT_BASE_URL,
    private val http: OkHttpClient =
        OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(30, TimeUnit.SECONDS)
            .build(),
) {
    private val json = Json { ignoreUnknownKeys = true }

    fun transcribe(
        audio: ByteArray,
        apiKey: String,
        contentType: String = "audio/mp4",
    ): DeepgramListenOutcome {
        AppLog.info(
            feature = "dictation",
            message = "deepgram listen requested",
            fields = mapOf(
                "input_shape" to "audio_bytes=${audio.size}",
                "key_present" to apiKey.isNotBlank().toString(),
            ),
        )
        val url =
            baseUrl.toHttpUrl().newBuilder()
                .addPathSegments("v1/listen")
                .addQueryParameter("model", "nova-3")
                .addQueryParameter("smart_format", "true")
                .build()
        val request =
            Request.Builder()
                .url(url)
                .post(audio.toRequestBody(contentType.toMediaType()))
                .header("Authorization", "Token $apiKey")
                .header("Content-Type", contentType)
                .build()
        return try {
            http.newCall(request).execute().use { response ->
                val raw = response.body?.string().orEmpty()
                AppLog.info(
                    feature = "dictation",
                    message = "deepgram listen response",
                    fields = mapOf(
                        "status" to response.code.toString(),
                        "body_len" to raw.length.toString(),
                    ),
                )
                when {
                    response.code == 401 || response.code == 403 -> DeepgramListenOutcome.Rejected
                    !response.isSuccessful -> DeepgramListenOutcome.NetworkError
                    else -> parseTranscript(raw)
                }
            }
        } catch (error: IOException) {
            AppLog.error(
                feature = "dictation",
                message = "deepgram listen network failed",
                error = error,
                fields = mapOf("decision" to "failed_network"),
            )
            DeepgramListenOutcome.NetworkError
        }
    }

    private fun parseTranscript(raw: String): DeepgramListenOutcome {
        val transcript =
            runCatching {
                json.parseToJsonElement(raw)
                    .jsonObject["results"]
                    ?.jsonObject
                    ?.get("channels")
                    ?.jsonArray
                    ?.firstOrNull()
                    ?.jsonObject
                    ?.get("alternatives")
                    ?.jsonArray
                    ?.firstOrNull()
                    ?.jsonObject
                    ?.get("transcript")
                    ?.jsonPrimitive
                    ?.contentOrNull
                    ?.trim()
                    .orEmpty()
            }.getOrDefault("")
        return if (transcript.isEmpty()) DeepgramListenOutcome.Empty else DeepgramListenOutcome.Transcript(transcript)
    }

    companion object {
        const val DEFAULT_BASE_URL = "https://api.deepgram.com"
    }
}
