package app.codexlauncher.capability.interaction

import app.codexlauncher.connection.protocol.ProtocolCodec
import app.codexlauncher.connection.session.ActionSendResult
import app.codexlauncher.storage.capability.unresolved.UnresolvedCapabilityCheck
import app.codexlauncher.storage.capability.unresolved.UnresolvedCapabilityStore
import java.util.ArrayDeque
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The block that stops a message being sent twice currently dies when Android
 * kills the app.
 *
 * `CapabilityInteraction.pendingCheck` is a plain field on an object the
 * launcher's view model builds in memory (`LauncherSessionViewModel.kt:139`).
 * `CapabilitySessionLostTest` proves it survives everything the app does to
 * itself — reconnecting, clearing, switching destination — and that is the
 * whole point of it, in its own words: *"a real duplicate message to a real
 * person, arriving from a feature built to prevent exactly that."*
 *
 * It does not survive the one thing the app has no say in. Someone says
 * "message Maya", the connection drops mid-send, they get the warning, they
 * switch apps, Android reclaims the memory, they come back, and the phone has
 * forgotten. Nothing refuses the second send. Maya gets it twice.
 *
 * The task-action half of this app already solved the identical problem
 * durably: a fork whose outcome is unknown is still refused after the whole
 * bridge is rebuilt, because the fact lives in `ActionJournal` and not in a
 * field (`TaskActionBridgeTest.unresolvedForkSurvivesBridgeRecreationAndBlocksAnotherFork`).
 * This file asks the capability half for the same guarantee.
 *
 * **How process death is expressed here:** by throwing the object away and
 * building a new one over the same storage. That is exactly what a cold start
 * is — the store outlives the process, the object does not.
 *
 * **The two controls exist because "always refuse" would otherwise pass.** An
 * app that blocks every prompt after every run satisfies each durability test
 * below and is worse than the bug. So a plain success must leave nothing
 * behind, and a phone that has never had an unresolved run must not start life
 * blocked.
 */
class CapabilityUnresolvedDurabilityTest {

    /**
     * A fake of the store, standing in for the file on disk. It keeps its
     * contents across as many [CapabilityInteraction] instances as a test cares
     * to build, which is the only property that matters here.
     */
    private class FakeStore(
        private var saved: String? = null,
        private val readable: Boolean = true,
    ) : UnresolvedCapabilityStore {
        var writes = 0
            private set

        override suspend fun load(): UnresolvedCapabilityCheck =
            when {
                !readable -> UnresolvedCapabilityCheck.Unreadable
                saved == null -> UnresolvedCapabilityCheck.None
                else -> UnresolvedCapabilityCheck.Pending(saved!!)
            }

        override fun remember(check: String?) {
            writes += 1
            saved = check
        }
    }

    /** The first writer: the phone works out for itself that it does not know. */
    @Test
    fun aCheckArmedByALostConnectionSurvivesTheAppBeingKilled() = runBlocking {
        val store = FakeStore()
        drivenToExecuting(store).sessionLost()

        val afterRestart = coldStart(store)

        assertNotNull(
            "the block must come back with the app, not die with the process",
            afterRestart.state.value.unresolvedCheck,
        )
        assertNull("and it must still refuse the next prompt", afterRestart.request("Message Maya"))
    }

    /** The second writer: the computer said outright that it does not know. */
    @Test
    fun aCheckArmedByAnUnknownOutcomeSurvivesTheAppBeingKilled() = runBlocking {
        val store = FakeStore()
        val before = drivenToExecuting(store)
        before.acceptActionResult(unknownResult())
        assertNotNull("precondition: the run must have armed a check", before.state.value.unresolvedCheck)

        val afterRestart = coldStart(store)

        assertNull("both writers must be equally durable", afterRestart.request("Message Maya"))
    }

    /**
     * Not just *a* warning — the same one. A vaguer message after a restart
     * tells the person less about what to go and look at than they were told a
     * minute earlier, and it is the same fact about the same run.
     */
    @Test
    fun theWarningThatComesBackSaysWhatItSaidBefore() = runBlocking {
        val store = FakeStore()
        val before = drivenToExecuting(store)
        before.sessionLost()
        val original = before.state.value.unresolvedCheck

        val afterRestart = coldStart(store)

        assertEquals(original, afterRestart.state.value.unresolvedCheck)
    }

    /**
     * Nothing lifts the block except someone saying they looked — and that has
     * to be remembered too. If only the arming were durable, tapping "I
     * checked" would be undone by the next cold start and the feature would
     * refuse prompts forever.
     */
    @Test
    fun havingCheckedIsRememberedAcrossARestartToo() = runBlocking {
        val store = FakeStore()
        drivenToExecuting(store).sessionLost()

        val second = coldStart(store)
        second.markChecked()
        val third = coldStart(store)

        assertNull(third.state.value.unresolvedCheck)
        assertEquals("a prompt must go out again", "route-two", third.request("Message Maya"))
    }

    /**
     * First control. An app that refuses everything after every run passes
     * every test above.
     */
    @Test
    fun anOrdinaryResultLeavesNothingBehindToBlockTheNextStart() = runBlocking {
        val store = FakeStore()
        val before = drivenToExecuting(store)
        before.acceptResult(resultFrame())
        assertNull("precondition: a known answer arms nothing", before.state.value.unresolvedCheck)

        val afterRestart = coldStart(store)

        assertNull(afterRestart.state.value.unresolvedCheck)
        assertEquals("route-two", afterRestart.request("Message Maya"))
    }

    /** Second control: a phone that has never had an unresolved run starts free. */
    @Test
    fun aPhoneWithNothingStoredIsNotBlocked() = runBlocking {
        val fresh = coldStart(FakeStore())

        assertNull(fresh.state.value.unresolvedCheck)
        assertEquals("route-two", fresh.request("Message Maya"))
    }

    /**
     * When the storage cannot be read we do not know whether a check is
     * pending. The two ways to be wrong are not equal: refusing costs one tap
     * on "I checked", and not refusing may send a real message to a real person
     * a second time. So it refuses, and it says why.
     *
     * This is the same call the task-action side already made — a fork is
     * blocked when the journal cannot be read (`TaskActionBridge`, `NotSent`).
     */
    @Test
    fun storageWeCannotReadRefusesRatherThanRiskSendingTwice() = runBlocking {
        val unreadable = coldStart(FakeStore(readable = false))

        assertNull("an unreadable store must not read as 'nothing pending'", unreadable.request("Message Maya"))
        val message = unreadable.state.value.message.orEmpty()
        assertTrue("a silent refusal is indistinguishable from a broken app: $message", message.isNotBlank())
    }

    /**
     * Third control, guarding the seam itself. Every other test in this package
     * builds a [CapabilityInteraction] with no store at all, and all of them
     * must keep passing — storage is an addition to this object, not a
     * requirement of it.
     */
    @Test
    fun anInteractionWithNoStoreBehavesExactlyAsBefore() = runBlocking {
        val interaction = interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two")))
        interaction.request("Message Maya")
        interaction.acceptPreview(previewFrame())
        interaction.respond(confirm = true)

        interaction.sessionLost()

        assertNotNull("in-memory blocking is unchanged", interaction.state.value.unresolvedCheck)
        assertNull(interaction.request("Message Maya"))
    }

    /**
     * Writing on every publish would turn a screen update into disk traffic.
     * The fact only changes twice — armed, then cleared — so the store should
     * hear about it twice.
     */
    @Test
    fun theStoreIsToldOnlyWhenTheFactChanges() = runBlocking {
        val store = FakeStore()
        val interaction = drivenToExecuting(store)

        interaction.sessionLost()
        interaction.clear()
        interaction.setDestination(PromptDestination.COMPUTER)
        interaction.markChecked()

        assertEquals("armed once, cleared once", 2, store.writes)
    }

    /**
     * The block cannot depend on someone remembering to call
     * [CapabilityInteraction.restoreUnresolvedCheck] first.
     *
     * The launcher starts that read in the background and does not wait for it
     * (`LauncherSessionViewModel.kt:173`), so there is a window at startup —
     * short, but exactly when a cold start happens — where the object exists,
     * the stored block has not been read yet, and a prompt sails straight
     * through. That is the one thing this whole feature exists to prevent, so
     * asking is what must be safe, not the order of two calls.
     */
    @Test
    fun aPromptIsRefusedEvenWhenNobodyReadTheStoreFirst() = runBlocking {
        val stored = FakeStore(saved = "Maya may already have that message. Check the app.")
        val neverRestored = interaction(ArrayDeque(listOf("route-two", "route-three")), stored)

        assertNull(
            "a prompt must not slip past a stored block just because startup had not finished reading it",
            neverRestored.request("Message Maya"),
        )
    }

    /**
     * The control on that. Making [CapabilityInteraction.request] wait for the
     * store is only correct if the wait always ends — a phone with nothing
     * stored must send, and it must send now, not after some signal that never
     * arrives. The timeout is the point of the test: a block that hangs is a
     * launcher that never answers.
     */
    @Test
    fun aPromptOnAnEmptyStoreStillGoesOutWithoutWaitingForAnything() = runBlocking {
        val empty = interaction(ArrayDeque(listOf("route-two", "route-three")), FakeStore())

        val actionId = withTimeout(2_000) { empty.request("Message Maya") }

        assertEquals("route-two", actionId)
    }

    // --- harness ---

    private fun interaction(
        ids: ArrayDeque<String>,
        store: UnresolvedCapabilityStore? = null,
        sent: MutableList<String>? = null,
    ) = CapabilityInteraction(
        sendAction = { encoded, _ ->
            sent?.add(encoded)
            ActionSendResult.SENT_UNKNOWN
        },
        nextActionId = ids::removeFirst,
        unresolvedStore = store,
    )

    /** A brand-new object over storage that outlived the last one — a cold start. */
    private suspend fun coldStart(store: UnresolvedCapabilityStore): CapabilityInteraction =
        interaction(ArrayDeque(listOf("route-two", "route-three")), store)
            .also { it.restoreUnresolvedCheck() }

    private suspend fun drivenToExecuting(store: UnresolvedCapabilityStore): CapabilityInteraction =
        interaction(ArrayDeque(listOf("route-action", "confirm-action", "route-two")), store).also {
            it.restoreUnresolvedCheck()
            it.request("Message Maya")
            it.acceptPreview(previewFrame())
            it.respond(confirm = true)
            assertEquals(CapabilityPhase.EXECUTING, it.state.value.phase)
        }

    private fun previewFrame(requestId: String = "route-action") =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"preview-1","sender":"companion","type":"capability_preview","body":{"requestId":"$requestId","adapterId":"todoist","verb":"write","headline":"Create a Todoist task","lines":["Buy oat milk","Before tomorrow"],"confirmLabel":"Create task","fingerprint":"${"a".repeat(64)}"}}""",
        )

    private fun resultFrame() =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"result-1","sender":"companion","type":"capability_result","seq":9,"body":{"requestId":"route-action","ceiling":"completes","done":true,"detail":"Created Todoist task","handedOffTo":""}}""",
        )

    private fun unknownResult(actionId: String = "confirm-action") =
        ProtocolCodec.decodeText(
            """{"version":{"major":1,"minor":0},"messageId":"unknown-1","sender":"companion","type":"action_result","seq":11,"body":{"actionId":"$actionId","state":"outcome_unknown","error":{"code":"outcome_unknown","retryable":false}}}""",
        )
}
