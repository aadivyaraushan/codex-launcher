package app.codexlauncher.task.dictation

import org.junit.Assert.assertEquals
import org.junit.Test

class LiveDictationDraftTest {
    @Test
    fun `live partials replace the prior partial instead of duplicating it`() {
        val draft = LiveDictationDraft("Typed first")

        assertEquals("Typed first spoken", draft.update("spoken"))
        assertEquals("Typed first spoken words", draft.update("spoken words"))
    }

    @Test
    fun `blank starting text becomes the live transcript`() {
        val draft = LiveDictationDraft("  ")

        assertEquals("hello there", draft.update("  hello there  "))
    }

    @Test
    fun `blank partial never erases the starting draft`() {
        val draft = LiveDictationDraft("Keep this")

        assertEquals("Keep this", draft.update("   "))
    }

    @Test
    fun `starting indentation and trailing whitespace are preserved`() {
        val draft = LiveDictationDraft("  - item\n")

        assertEquals("  - item\nspoken words", draft.update("spoken words"))
    }
}
