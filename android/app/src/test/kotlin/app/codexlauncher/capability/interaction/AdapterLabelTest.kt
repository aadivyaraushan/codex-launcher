package app.codexlauncher.capability.interaction

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The confirm sheet is the whole consent mechanism for an app action, and it
 * currently prints a programmer's identifier at the user.
 *
 * `CapabilitySheet.kt:40` renders `preview.adapterId.replaceFirstChar(uppercase)`,
 * which is only ever right by accident. On the sheet somebody actually taps to
 * approve, today that reads "Notification_reply · send", "Maps_saved_places ·
 * open", "Gcalendar · create", "Msteams · send". Capitalising the first letter
 * of a snake_case id does not make it a name.
 *
 * Inputs: an adapter id as the Mac sends it. Output: the words to show a
 * person. Algorithm: look the id up in a written-down list of names; if it is
 * not there, tidy it — separators become spaces, each word gets a capital —
 * so an adapter added on the Mac before this list is updated degrades to
 * something plain rather than something wrong.
 *
 * ## Why the list lives on the phone
 *
 * A display name is not on the wire. Putting it there means a new field, a
 * schema change and both machines, to carry a string that never affects a
 * decision. The cost of keeping it here is drift — a new adapter falls to the
 * tidied fallback until somebody adds it. That failure is mild and visible,
 * which is the right way round.
 *
 * ## The reply adapter is the interesting one
 *
 * There is no good app name for `notification_reply`, and inventing one would
 * be a lie: the Mac never knows which app the reply lands in — the phone picks
 * that after this sheet is confirmed (`ReplyAdapter.pick`). So the label says
 * where the work happens rather than naming an app it cannot name. The
 * headline above it already carries the person ("Reply to Maya") and the box
 * below carries the exact text.
 */
class AdapterLabelTest {

    @Test
    fun `an id nobody wrote a name for is tidied rather than shown raw`() {
        assertEquals("Apple Notes", adapterLabel("apple-notes"))
    }

    @Test
    fun `underscores separate words too, not just hyphens`() {
        assertEquals("Saved Places", adapterLabel("saved_places"))
    }

    @Test
    fun `every word gets a capital, not only the first`() {
        assertEquals("One Two Three", adapterLabel("one_two-three"))
    }

    @Test
    fun `a plain one-word id is simply capitalised`() {
        assertEquals("Slack", adapterLabel("slack"))
    }

    /**
     * The four ids whose tidied form is still wrong. These are the reason a
     * written-down list exists at all: no amount of splitting and capitalising
     * turns "gcalendar" into the name of the product.
     */
    @Test
    fun `abbreviated ids are given their real product names`() {
        assertEquals("Google Calendar", adapterLabel("gcalendar"))
        assertEquals("Google Drive", adapterLabel("gdrive"))
        assertEquals("Microsoft Teams", adapterLabel("msteams"))
        assertEquals("Apple Reminders", adapterLabel("apple-reminders"))
    }

    @Test
    fun `maps and its saved-places sibling read as two different things`() {
        assertEquals("Google Maps", adapterLabel("maps"))
        assertEquals("Saved places in Google Maps", adapterLabel("maps_saved_places"))
    }

    /**
     * The headline says who. This line says where. It must not name an app,
     * because at this moment no machine knows which app it will be.
     */
    @Test
    fun `the reply adapter says where the work happens instead of naming an app it cannot name`() {
        assertEquals("This phone", adapterLabel("notification_reply"))
    }

    /**
     * A blank id is a bug upstream. The sheet must still render, and must not
     * print a stray separator dot with nothing before it — the caller can tell
     * there is nothing to show.
     */
    @Test
    fun `a missing id produces nothing rather than a stray label`() {
        assertEquals("", adapterLabel(""))
        assertEquals("", adapterLabel("   "))
    }

    @Test
    fun `an id arriving with odd spacing or case still finds its name`() {
        assertEquals("Google Calendar", adapterLabel("  GCalendar  "))
    }

    /**
     * Separators that end up next to each other, or on the ends, must not
     * produce empty words and double spaces.
     */
    @Test
    fun `runs of separators do not become empty words`() {
        assertEquals("Odd Id", adapterLabel("_odd__id-"))
    }
}
