package app.codexlauncher.task.attachments

import android.content.ContentResolver
import android.net.Uri
import android.provider.OpenableColumns
import app.codexlauncher.diagnostics.AppLog
import java.io.ByteArrayOutputStream
import java.io.InputStream

data class PickedAttachment(
    val displayName: String,
    val mediaType: String,
    val bytes: ByteArray,
)

class AttachmentDocumentReader(private val contentResolver: ContentResolver) {
    fun read(uri: Uri, maxBytes: Long): PickedAttachment? {
        if (maxBytes <= 0 || maxBytes > Int.MAX_VALUE) return null
        val mediaType = contentResolver.getType(uri)?.trim()?.lowercase() ?: return null
        val displayName = displayName(uri) ?: return null
        val bytes = contentResolver.openInputStream(uri)?.use { readAttachmentBytes(it, maxBytes) } ?: return null
        AppLog.info(
            feature = "attachment-picker",
            message = "system picker content copied into private memory",
            fields = mapOf("media_type" to mediaType, "size_bytes" to bytes.size, "input_shape" to "content_uri"),
        )
        return PickedAttachment(displayName, mediaType, bytes)
    }

    private fun displayName(uri: Uri): String? =
        contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { cursor ->
            if (!cursor.moveToFirst()) return@use null
            cursor.getString(0)?.takeIf { it.isNotBlank() && it.length <= 256 }
        }
}

internal fun readAttachmentBytes(input: InputStream, maxBytes: Long): ByteArray? {
    if (maxBytes <= 0 || maxBytes > Int.MAX_VALUE) return null
    val output = ByteArrayOutputStream(minOf(maxBytes.toInt(), 64 * 1024))
    val buffer = ByteArray(16 * 1024)
    var total = 0L
    while (true) {
        val read = input.read(buffer)
        if (read < 0) break
        total += read
        if (total > maxBytes) return null
        output.write(buffer, 0, read)
    }
    return output.toByteArray()
}
