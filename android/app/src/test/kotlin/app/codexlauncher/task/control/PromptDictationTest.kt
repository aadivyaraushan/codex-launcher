package app.codexlauncher.task.control

import org.junit.Assert.assertEquals
import org.junit.Test

class PromptDictationTest {
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
