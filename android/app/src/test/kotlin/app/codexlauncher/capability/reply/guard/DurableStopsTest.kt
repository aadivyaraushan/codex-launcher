package app.codexlauncher.capability.reply.guard

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * A stop is a promise, and right now the app forgets it.
 *
 * `ReplyGuard` holds the stop list in memory. Restarting the app — an upgrade,
 * a force-stop, Android reclaiming memory — forgets every stop the user ever
 * set, and Operator quietly starts replying again in a conversation somebody
 * deliberately shut. Nothing tells them. That is fine for the send cap, which
 * exists to catch a loop firing within seconds and has nothing to remember
 * across a restart, and it is plainly wrong for a promise made to a person.
 *
 * There is also no way to undo one. `resume` has no caller: a stop taken from
 * the offer row is, from the user's side, permanent.
 *
 * Inputs: stops and resumes as the user makes them, plus whatever was saved
 * last time the app ran. Output: a guard whose stop list matches what the user
 * asked for, and a saved copy that survives the process dying.
 *
 * ## Why this does not live inside `ReplyGuard`
 *
 * The guard's job is to decide. Where a decision is kept between runs is a
 * question about this app — which storage, which thread, what happens when the
 * disk is unreadable — and none of that belongs in the rule. Keeping it out
 * also means the guard's existing callers do not change, and a rule with many
 * callers is a bad place to be adding required constructor arguments.
 *
 * So this is a thin thing that owns both: it holds the guard and a place to
 * write to, and it is what the screens talk to.
 *
 * ## The port
 *
 * [StopStore] is the seam. The real one is Jetpack Preferences DataStore,
 * shaped after `storage/projects/ProjectSelectionStore.kt`. This file uses a
 * fake, so the rules below stay free of Android and can run on the JVM.
 */
class DurableStopsTest {

    /** A [StopStore] that keeps everything in a list, and can be made to fail. */
    private class FakeStore(
        var saved: MutableList<ThreadKey> = mutableListOf(),
        var readThrows: Boolean = false,
        var writeThrows: Boolean = false,
    ) : StopStore {
        var writes = 0

        override fun read(): List<ThreadKey> {
            if (readThrows) throw IllegalStateException("preferences file is corrupt")
            return saved.toList()
        }

        override fun write(keys: List<ThreadKey>) {
            writes++
            if (writeThrows) throw IllegalStateException("disk is full")
            saved = keys.toMutableList()
        }
    }

    private val maya = ThreadKey(packageName = "com.whatsapp", person = "Maya")
    private val sam = ThreadKey(packageName = "com.instagram.android", person = "Sam")

    private fun guard() = ReplyGuard(now = { 0L }, cap = ReplyCap.shipped)

    @Test
    fun aStopIsWrittenDownWhenItIsMade() {
        val store = FakeStore()
        val stops = DurableStops(guard = guard(), store = store)

        stops.stop(maya)

        assertEquals(listOf(maya), store.saved)
    }

    @Test
    fun aStopMadeThisRunIsObeyedImmediatelyWithoutWaitingForARestart() {
        val guard = guard()
        val stops = DurableStops(guard = guard, store = FakeStore())

        stops.stop(maya)

        assertEquals(ReplyVerdict.STOPPED_BY_USER, guard.check(maya))
    }

    /**
     * The headline. A stop set before the app died is still a stop after it
     * comes back — otherwise the promise lasts only as long as the process.
     */
    @Test
    fun aStopSetBeforeARestartIsStillInForceAfterOne() {
        val store = FakeStore(saved = mutableListOf(maya))
        val guard = guard()

        DurableStops(guard = guard, store = store).restore()

        assertEquals(ReplyVerdict.STOPPED_BY_USER, guard.check(maya))
    }

    @Test
    fun restoringBringsBackEveryStopNotJustTheLast() {
        val store = FakeStore(saved = mutableListOf(maya, sam))
        val guard = guard()

        DurableStops(guard = guard, store = store).restore()

        assertEquals(setOf(maya, sam), guard.stopped())
    }

    @Test
    fun undoingAStopTakesItOutOfStorageToo() {
        val store = FakeStore(saved = mutableListOf(maya, sam))
        val guard = guard()
        val stops = DurableStops(guard = guard, store = store)
        stops.restore()

        stops.resume(maya)

        assertEquals(listOf(sam), store.saved)
        assertEquals(ReplyVerdict.ALLOWED, guard.check(maya))
        assertEquals(ReplyVerdict.STOPPED_BY_USER, guard.check(sam))
    }

    /**
     * What a screen listing stopped conversations reads. It has to come from
     * the guard rather than the store, because the guard is what actually
     * refuses replies — showing the user a list that storage agrees with but
     * the running app does not would be showing them a fiction.
     */
    @Test
    fun theListOfferedToAScreenIsTheOneTheGuardIsActuallyEnforcing() {
        val guard = guard()
        val stops = DurableStops(guard = guard, store = FakeStore())
        stops.stop(maya)
        stops.stop(sam)

        assertEquals(setOf(maya, sam), stops.stopped())

        stops.resume(sam)

        assertEquals(setOf(maya), stops.stopped())
    }

    /**
     * Stopping the same conversation twice is something a user can do by
     * tapping quickly. It must not write a duplicate that then needs two
     * resumes to clear.
     */
    @Test
    fun stoppingTheSameConversationTwiceLeavesOneEntry() {
        val store = FakeStore()
        val stops = DurableStops(guard = guard(), store = store)

        stops.stop(maya)
        stops.stop(ThreadKey(packageName = "com.whatsapp", person = "  maya  "))

        assertEquals(1, store.saved.size)
    }

    /**
     * Resuming something that was never stopped is a no-op, not an error. A
     * screen may be showing a list that a moment ago was accurate.
     */
    @Test
    fun undoingAStopThatWasNeverSetChangesNothing() {
        val store = FakeStore(saved = mutableListOf(maya))
        val stops = DurableStops(guard = guard(), store = store)
        stops.restore()

        stops.resume(sam)

        assertEquals(listOf(maya), store.saved)
    }

    /**
     * If the saved list cannot be read, the app has to start rather than
     * crash — but it must not pretend the list was empty and go on replying.
     * Restoring reports that it could not, and this codebase already has a
     * word for that shape of answer rather than a boolean: not knowing is a
     * distinct outcome from knowing there is nothing.
     */
    @Test
    fun anUnreadableStopListIsReportedRatherThanTreatedAsNoStops() {
        val stops = DurableStops(guard = guard(), store = FakeStore(readThrows = true))

        assertEquals(RestoreOutcome.UNKNOWN, stops.restore())
    }

    @Test
    fun aStopListThatReadsCleanlyReportsThatItDid() {
        val stops = DurableStops(guard = guard(), store = FakeStore(saved = mutableListOf(maya)))

        assertEquals(RestoreOutcome.RESTORED, stops.restore())
    }

    /**
     * A write that fails must not leave the running app disagreeing with what
     * the user just asked for. The stop still holds for this run — losing it
     * on the next restart is bad, silently not stopping at all right now is
     * worse.
     */
    @Test
    fun aStopStillHoldsForThisRunEvenIfItCouldNotBeWrittenDown() {
        val guard = guard()
        val stops = DurableStops(guard = guard, store = FakeStore(writeThrows = true))

        stops.stop(maya)

        assertEquals(ReplyVerdict.STOPPED_BY_USER, guard.check(maya))
    }

    @Test
    fun aStopThatCouldNotBeWrittenDownSaysSo() {
        val stops = DurableStops(guard = guard(), store = FakeStore(writeThrows = true))

        assertEquals(SaveOutcome.UNKNOWN, stops.stop(maya))
    }

    @Test
    fun aStopThatWasWrittenDownSaysSo() {
        val stops = DurableStops(guard = guard(), store = FakeStore())

        assertEquals(SaveOutcome.SAVED, stops.stop(maya))
    }

    /**
     * Restoring twice — which happens if the app is brought back to the front
     * after Android trimmed it — must not undo a stop the user made in
     * between. Restore adds what was saved; it never clears what is there.
     */
    @Test
    fun restoringAgainDoesNotUndoAStopMadeSinceTheLastRestore() {
        val store = FakeStore(saved = mutableListOf(maya))
        val guard = guard()
        val stops = DurableStops(guard = guard, store = store)
        stops.restore()
        stops.stop(sam)

        stops.restore()

        assertTrue(guard.stopped().containsAll(setOf(maya, sam)))
    }
}
