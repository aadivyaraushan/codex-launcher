package app.codexlauncher.appearance.theme

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.math.max
import kotlin.math.min
import kotlin.math.pow

class QuietInstrumentTokensTest {
    @Test
    fun palettesMatchTheApprovedDesignContract() {
        assertEquals(0xFF111210, QuietInstrumentTokens.deepCharcoal.background)
        assertEquals(0xFF1A1B18, QuietInstrumentTokens.deepCharcoal.raisedSurface)
        assertEquals(0xFFF0EEE8, QuietInstrumentTokens.deepCharcoal.primaryText)
        assertEquals(0xFF969990, QuietInstrumentTokens.deepCharcoal.mutedText)
        assertEquals(0xFFF06B3F, QuietInstrumentTokens.deepCharcoal.signal)

        assertEquals(0xFFF4F2ED, QuietInstrumentTokens.warmPaper.background)
        assertEquals(0xFFEBE9E2, QuietInstrumentTokens.warmPaper.raisedSurface)
        assertEquals(0xFF191A18, QuietInstrumentTokens.warmPaper.primaryText)
        assertEquals(0xFF656861, QuietInstrumentTokens.warmPaper.mutedText)
        assertEquals(0xFFE86136, QuietInstrumentTokens.warmPaper.signal)
    }

    @Test
    fun normalTextColorsMeetAccessibleContrastAgainstTheirBackgrounds() {
        for (palette in listOf(QuietInstrumentTokens.deepCharcoal, QuietInstrumentTokens.warmPaper)) {
            assertTrue(contrast(palette.primaryText, palette.background) >= 4.5)
            assertTrue(contrast(palette.mutedText, palette.background) >= 4.5)
        }
    }

    @Test
    fun selectedControlTextMeetsAccessibleContrastAgainstTheSignalColor() {
        for (palette in listOf(QuietInstrumentTokens.deepCharcoal, QuietInstrumentTokens.warmPaper)) {
            assertTrue(contrast(palette.onSignal, palette.signal) >= 4.5)
        }
    }

    @Test
    fun semanticColorsRemainWordsNotAThirdAppearanceMode() {
        assertEquals("Working", QuietInstrumentTokens.workingLabel)
        assertEquals("Needs your answer", QuietInstrumentTokens.waitingLabel)
        assertEquals("Replied", QuietInstrumentTokens.repliedLabel)
        assertEquals("Failed", QuietInstrumentTokens.failedLabel)
        assertEquals(listOf(AppearanceMode.FOLLOW_SYSTEM, AppearanceMode.LIGHT, AppearanceMode.DARK), AppearanceMode.entries)
    }

    @Test
    fun sizingKeepsControlsReadableAndTouchable() {
        assertEquals(listOf(4, 8, 12, 16, 24, 32), QuietInstrumentTokens.spacingDp)
        assertTrue(QuietInstrumentTokens.minimumTouchTargetDp >= 44)
        assertTrue(QuietInstrumentTokens.securityActionHeightDp >= 48)
        assertTrue(QuietInstrumentTokens.bodyTextSp >= 13)
        assertTrue(QuietInstrumentTokens.taskTitleSp >= 15)
    }

    private fun contrast(left: Long, right: Long): Double {
        val lighter = max(luminance(left), luminance(right))
        val darker = min(luminance(left), luminance(right))
        return (lighter + 0.05) / (darker + 0.05)
    }

    private fun luminance(color: Long): Double {
        fun channel(shift: Int): Double {
            val encoded = ((color shr shift) and 0xFF).toDouble() / 255.0
            return if (encoded <= 0.04045) encoded / 12.92 else ((encoded + 0.055) / 1.055).pow(2.4)
        }
        return 0.2126 * channel(16) + 0.7152 * channel(8) + 0.0722 * channel(0)
    }
}
