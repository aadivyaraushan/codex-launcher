package app.codexlauncher.storage.wipe

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * A wipe clears ten stores and leaves the reply stop list alone. That is the
 * behaviour today, and this file pins it so it cannot change by accident.
 *
 * This is a record of what the code does, not an argument that it is right.
 * Nothing anywhere records a decision about it — no comment in
 * [LocalStateWiper], no test, no note in either plan. The stop list is simply
 * not in [WipeStep.deletions], the way a store nobody thought about is not in
 * it.
 *
 * **What is at stake is not only whether stops survive.** The stop list is
 * persisted by `storage/reply/stops/ReplyStopStore.kt`, which writes, for each
 * stopped conversation, an app package name and **a person's name, in plain
 * text**, as JSON in a preferences file called `reply_stops`. So a wipe today
 * leaves real people's names on the device. That sits badly beside a rule this
 * codebase wrote down for itself elsewhere — `LiveReplyBoxesTest`'s header says
 * a conversation title is *who*, not *what*, and so "is kept in memory only …
 * it never reaches `NotificationSighting`, the on-disk ledger, or a log line."
 *
 * **The two answers look opposed and are not.** Keeping the stops means keeping
 * the names only if the name has to be stored as a name. It does not: the guard
 * never displays it. The one and only place `ThreadKey.person` is read anywhere
 * in `app/src/main` is the line in `ReplyStopStore` that writes it out; the
 * guard matches on whole-key equality. So a one-way hash of package-plus-person
 * would keep every stop working across a restart and put no names on disk.
 * That is a change to an on-disk format, with a migration question attached —
 * dropping old entries would quietly resume conversations the user had stopped,
 * which is the dangerous direction — so it is not being made here.
 *
 * Whoever settles this rewrites this file in the same change. Full write-up in
 * `saved-results/the-wipe-leaves-names-behind.md`.
 */
class WipeDoesNotClearTheStopListTest {

    @Test
    fun theWipeClearsTenStoresAndTheStopListIsNotOneOfThem() {
        assertEquals(
            listOf(
                WipeStep.PROJECT_SELECTION,
                WipeStep.ACTION_RECORDS,
                WipeStep.LAST_CONNECTION,
                WipeStep.RESUME_CURSOR,
                WipeStep.DRAFT_CIPHERTEXT,
                WipeStep.DRAFT_KEY,
                WipeStep.DEVICE_IDENTITY,
                WipeStep.PAIRING_KEY,
                WipeStep.PAIRING_RECORD,
                WipeStep.CAPABILITY_UNRESOLVED_CHECK,
            ),
            WipeStep.deletions,
        )
    }

    @Test
    fun noWipeStepNamesTheReplyStopList() {
        // If a step for it is ever added, this test is the one that should be
        // rewritten first — deliberately, with the migration question above
        // answered rather than discovered later.
        val named = WipeStep.entries.filter {
            val n = it.name.lowercase()
            n.contains("stop") || n.contains("reply")
        }

        assertTrue(
            "a wipe step now names the reply stop list: $named. " +
                "Answer the migration question in this file's header before " +
                "deleting anything: dropping stored stops resumes conversations " +
                "the user had stopped, which is the dangerous direction.",
            named.isEmpty(),
        )
    }

    @Test
    fun everyStepThatDeletesSomethingIsBracketedByTheMarkers() {
        // The markers are the wipe's own record that it started and finished.
        // They are not deletions and must never drift into that list, or a
        // half-finished wipe would look complete.
        assertFalse(WipeStep.deletions.contains(WipeStep.MARKER_BEGIN))
        assertFalse(WipeStep.deletions.contains(WipeStep.MARKER_FINISH))
        assertFalse(WipeStep.deletions.contains(WipeStep.MARKER_READ))
        assertEquals(WipeStep.entries.size - 3, WipeStep.deletions.size)
    }
}
