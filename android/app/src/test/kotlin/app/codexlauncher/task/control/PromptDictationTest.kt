package app.codexlauncher.task.control

import org.junit.Assert.assertEquals
import org.junit.Test

class PromptDictationTest {
    @Test
    fun `home recognition appends text and reports success`() {
        assertEquals(
            "Dictation added",
            homeDictationMessage(PromptDictationResult.Recognized("new words"), recognizedApplied = true),
        )
    }

    @Test
    fun `home dictation failures keep the current draft`() {
        assertEquals(
            "Dictation canceled",
            homeDictationMessage(PromptDictationResult.Cancelled, recognizedApplied = false),
        )
        assertEquals(
            "Speech recognition isn’t installed",
            homeDictationMessage(PromptDictationResult.Unavailable, recognizedApplied = false),
        )
        assertEquals(
            "Couldn’t understand speech",
            homeDictationMessage(PromptDictationResult.Failed, recognizedApplied = false),
        )
    }

    @Test
    fun `home recognition cannot claim success when the draft became unavailable`() {
        assertEquals(
            "Dictation wasn’t added because the draft changed",
            homeDictationMessage(PromptDictationResult.Recognized("new words"), recognizedApplied = false),
        )
    }

    @Test
    fun `recognized words append to existing editable text`() {
        assertEquals("Existing words new words", mergePromptDictation("Existing words", "  new words  "))
    }

    @Test
    fun `recognized words become the text when the composer is blank`() {
        assertEquals("new words", mergePromptDictation("  ", " new words "))
    }

    @Test
    fun `blank recognition never changes typed text`() {
        assertEquals("Keep this", mergePromptDictation("Keep this", "   "))
    }

    @Test
    fun `dictation preserves existing indentation and trailing whitespace`() {
        assertEquals("  - item\nspoken words", mergePromptDictation("  - item\n", "spoken words"))
    }
}
