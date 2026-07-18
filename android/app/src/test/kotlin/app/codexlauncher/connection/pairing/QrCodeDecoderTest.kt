package app.codexlauncher.connection.pairing

import com.google.zxing.BarcodeFormat
import com.google.zxing.qrcode.QRCodeWriter
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class QrCodeDecoderTest {
    @Test
    fun decodesARealQrLuminanceFrame() {
        val expected = "codex-launcher://pair?host=203.0.113.5"
        val matrix = QRCodeWriter().encode(expected, BarcodeFormat.QR_CODE, 240, 240)
        val luminance =
            ByteArray(matrix.width * matrix.height) { index ->
                val x = index % matrix.width
                val y = index / matrix.width
                if (matrix[x, y]) 0 else 0xff.toByte()
            }

        assertEquals(expected, QrCodeDecoder.decodeLuminance(luminance, matrix.width, matrix.height))
    }

    @Test
    fun rejectsMalformedFrameDimensionsAndNonQrPixels() {
        assertNull(QrCodeDecoder.decodeLuminance(ByteArray(3), width = 2, height = 2))
        assertNull(QrCodeDecoder.decodeLuminance(ByteArray(100) { 0xff.toByte() }, width = 10, height = 10))
    }
}
