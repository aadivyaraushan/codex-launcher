package app.codexlauncher.task.attachments

import java.io.ByteArrayInputStream
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertNull
import org.junit.Test

class AttachmentDocumentReaderTest {
    @Test
    fun `limited read returns an exact private copy within the advertised limit`() {
        val value = "private notes".encodeToByteArray()

        assertArrayEquals(value, readAttachmentBytes(ByteArrayInputStream(value), value.size.toLong()))
    }

    @Test
    fun `limited read rejects content that exceeds the advertised limit`() {
        assertNull(readAttachmentBytes(ByteArrayInputStream(ByteArray(9)), 8))
    }
}
