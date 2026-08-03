package app.codexlauncher.capability.reply.guard

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test

/**
 * The name goes on screen exactly as the Mac spelled it, and that is one job
 * too many for one string.
 *
 * [ThreadKey] is deliberate about keeping [ThreadKey.person] untouched: it is
 * the string that eventually reaches `ReplyHandleSource.candidatesFor`, which
 * matches it against whatever Android actually put in the notification shade,
 * and tidying it on the way there is how a reply meant for a real person
 * misses its target. That reasoning is correct and this test does not disturb
 * it.
 *
 * What the class never gave anyone is a version of the name fit to *show*.
 * Three places print the raw string — the offer banner and its button
 * (LauncherDialogs.kt:118 and :123) and the stopped-conversations list in
 * settings (AppearanceScreen.kt:144) — so whatever spacing arrived on the wire
 * shows through. Trailing spaces merely look sloppy. A line break is worse:
 * inside `TextButton { Text("Stop replying to $name") }` it splits the button
 * across two lines and the row's layout goes with it.
 *
 * So: one new read-only accessor for display, and the stored spelling stays
 * exactly as it was. The two must not be confused, which is why the last two
 * tests here exist at all.
 *
 * On the empty case, one decision made rather than left dangling: a name that
 * is nothing but whitespace falls back to "this conversation", because
 * "Stop replying to ." is not a sentence and a button has to say something.
 * The wording is mine, not a considered product call — it is easy to change
 * and nothing else depends on it.
 */
class ThreadKeyDisplayNameTest {

    @Test
    fun surroundingSpacesDoNotReachTheScreen() {
        assertEquals("Maya", ThreadKey("com.whatsapp", "  Maya  ").displayPerson)
    }

    @Test
    fun anOrdinaryNameIsLeftCompletelyAlone() {
        // The common case by far. A fix that "tidies" normal names is a bug.
        assertEquals("Maya Rivera", ThreadKey("com.whatsapp", "Maya Rivera").displayPerson)
    }

    @Test
    fun aLineBreakCannotSplitTheButtonItSitsIn() {
        // The one that is a layout bug rather than a cosmetic one.
        assertEquals("Maya Rivera", ThreadKey("com.whatsapp", "Maya\nRivera").displayPerson)
    }

    @Test
    fun aRunOfSpacesInTheMiddleCollapsesToOne() {
        assertEquals("Maya Rivera", ThreadKey("com.whatsapp", "Maya \t  Rivera").displayPerson)
    }

    @Test
    fun aNameThatIsNothingButSpaceStillReadsAsSomething() {
        assertEquals("this conversation", ThreadKey("com.whatsapp", "   ").displayPerson)
        assertEquals("this conversation", ThreadKey("com.whatsapp", "").displayPerson)
    }

    @Test
    fun theStoredSpellingSurvivesUntouched() {
        // The whole point of the class. Whatever display does, the string that
        // goes on to match the notification shade is byte-for-byte what arrived.
        val key = ThreadKey("com.whatsapp", "  Maya\nRivera  ")

        assertEquals("  Maya\nRivera  ", key.person)
        assertNotEquals(
            "display must be a separate reading of the name, not a rewrite of it",
            key.person,
            key.displayPerson,
        )
    }

    @Test
    fun twoSpellingsOfTheSameNameAreStillOneConversation() {
        // Regression guard: the guard's own matching already ignores case and
        // surrounding space, and adding a display accessor must not touch it.
        val typedOneWay = ThreadKey("com.whatsapp", " Maya ")
        val typedAnother = ThreadKey("com.whatsapp", "maya")

        assertEquals(typedOneWay.matchKey, typedAnother.matchKey)
    }
}
