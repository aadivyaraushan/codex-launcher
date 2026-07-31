package app.codexlauncher.capability.outcome

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The three ceilings, shown to a person.
 *
 * Operator acts inside other people's apps, so a request can stop in three
 * places. The whole point of this file is the middle one: work that is done
 * but has NOT happened yet, sitting one tap short of the irreversible step.
 * A user who reads that as "sent" has been lied to by the interface.
 */
class CapabilityOutcomeTest {

    // ---- the marks are the ones DESIGN.md fixed, and no others ------------

    @Test
    fun everyMarkCarriesItsWords() {
        // DESIGN.md: "Both carry their words." A shape alone is not a state.
        for (mark in StateMark.entries) {
            assertTrue("$mark has no label", mark.label.isNotBlank())
        }
    }

    @Test
    fun oneTapLeftAndHandedOffAreImpossibleToConfuse() {
        // These are the two a user is most likely to mix up, and the cost of
        // mixing them up is believing a message was sent when it was not.
        // DESIGN.md: one is filled and warm, the other outlined and muted.
        val tap = StateMark.ONE_TAP_LEFT
        val handed = StateMark.HANDED_OFF

        assertEquals(MarkFill.SOLID, tap.fill)
        assertEquals(MarkTone.WARNING, tap.tone)
        assertEquals(MarkShape.HALF_CIRCLE, tap.shape)

        assertEquals(MarkFill.OUTLINED, handed.fill)
        assertEquals(MarkTone.MUTED, handed.tone)
        assertEquals(MarkShape.CIRCLE_WITH_EXIT_ARROW, handed.shape)

        assertTrue("they differ only in one way, which is not enough", tap.fill != handed.fill)
        assertTrue(tap.tone != handed.tone)
        assertTrue(tap.shape != handed.shape)
        assertTrue(tap.label != handed.label)
    }

    @Test
    fun theMarkNamesMatchTheDesignDocumentExactly() {
        assertEquals("One tap left", StateMark.ONE_TAP_LEFT.label)
        assertEquals("Handed off", StateMark.HANDED_OFF.label)
        assertEquals("Unverified", StateMark.UNVERIFIED.label)
    }

    @Test
    fun noMarkSharesAShapeAndFillAndToneWithAnother() {
        // Two marks that render identically are one mark with two names.
        val seen = mutableSetOf<Triple<MarkShape, MarkFill, MarkTone>>()
        for (mark in StateMark.entries) {
            val look = Triple(mark.shape, mark.fill, mark.tone)
            assertTrue("$mark looks the same as an earlier mark", seen.add(look))
        }
    }

    // ---- completes: the only state allowed to claim it happened -----------

    @Test
    fun aFinishedCompletesRunSaysItHappened() {
        val out = CapabilityOutcome.of(
            ceiling = Ceiling.COMPLETES,
            done = true,
            detail = "Replied to Maya: on my way",
            app = "WhatsApp",
        )
        assertEquals(StateMark.REPLIED, out.mark)
        assertTrue(out.claimsSuccess)
        assertFalse(out.claimsFailure)
        assertNull("nothing was handed off", out.handedOffToApp)
        assertNull("nothing is left to confirm", out.confirmControl)
    }

    // ---- one tap left: done, but it has NOT happened ----------------------

    @Test
    fun aOneTapOutcomeNeverClaimsTheThingHappened() {
        val out = CapabilityOutcome.of(
            ceiling = Ceiling.ONE_TAP,
            done = true,
            detail = "Reply to Maya: on my way",
            app = "WhatsApp",
        )
        assertEquals(StateMark.ONE_TAP_LEFT, out.mark)
        assertFalse("the send has not happened yet", out.claimsSuccess)
        assertFalse(out.claimsFailure)
    }

    @Test
    fun aOneTapOutcomeAlwaysCarriesThePreviewAndTheControlThatFinishesIt() {
        // DESIGN.md: "Always accompanied by the preview of what will happen and
        // the control that finishes it." A held action with no visible way to
        // finish it is a task that silently never completes.
        val out = CapabilityOutcome.of(
            ceiling = Ceiling.ONE_TAP,
            done = true,
            detail = "Reply to Maya: on my way",
            app = "WhatsApp",
        )
        assertNotNull("no control to finish it", out.confirmControl)
        assertTrue(out.confirmControl!!.isNotBlank())
        assertTrue("the preview does not say what will happen", out.detail.contains("on my way"))
    }

    @Test
    fun aOneTapOutcomeWithNothingToShowIsRejectedRatherThanShownBlank() {
        // Asking someone to confirm an action we cannot describe is asking them
        // to sign a blank page.
        try {
            CapabilityOutcome.of(ceiling = Ceiling.ONE_TAP, done = true, detail = "  ", app = "WhatsApp")
            throw AssertionError("a blank preview was accepted")
        } catch (expected: IllegalArgumentException) {
            assertTrue(expected.message!!.isNotBlank())
        }
    }

    // ---- handed off: we stopped being able to see ------------------------

    @Test
    fun aHandOffClaimsNeitherSuccessNorFailure() {
        // DESIGN.md: "Never claim success after this mark, and never claim
        // failure — say what was handed over and to which app."
        val out = CapabilityOutcome.of(
            ceiling = Ceiling.HANDS_OFF,
            done = true,
            detail = "Opened Uber with the trip loaded",
            app = "Uber",
        )
        assertEquals(StateMark.HANDED_OFF, out.mark)
        assertFalse(out.claimsSuccess)
        assertFalse(out.claimsFailure)
    }

    @Test
    fun aHandOffNamesTheAppItHandedTo() {
        val out = CapabilityOutcome.of(
            ceiling = Ceiling.HANDS_OFF,
            done = true,
            detail = "Opened Uber with the trip loaded",
            app = "Uber",
        )
        assertEquals("Uber", out.handedOffToApp)
        assertNull("a hand-off has nothing left for the user to confirm here", out.confirmControl)
    }

    @Test
    fun aHandOffWithNoAppNamedIsRejected() {
        // "Handed off" with no destination tells the user nothing they can act on.
        try {
            CapabilityOutcome.of(ceiling = Ceiling.HANDS_OFF, done = true, detail = "Opened it", app = null)
            throw AssertionError("a hand-off with no named app was accepted")
        } catch (expected: IllegalArgumentException) {
            assertTrue(expected.message!!.isNotBlank())
        }
    }

    // ---- failure outranks the ceiling ------------------------------------

    @Test
    fun aRunThatDidNotFinishIsFailedWhateverItsCeiling() {
        for (ceiling in Ceiling.entries) {
            val out = CapabilityOutcome.of(
                ceiling = ceiling,
                done = false,
                detail = "Notes is not running",
                app = "Notes",
            )
            assertEquals("$ceiling did not report failure", StateMark.FAILED, out.mark)
            assertTrue(out.claimsFailure)
            assertFalse(out.claimsSuccess)
            assertNull(out.handedOffToApp)
        }
    }

    @Test
    fun aFailureAlwaysOffersARecoveryAction() {
        // DESIGN.md: failed is "always paired with `Failed` and a recovery action".
        val out = CapabilityOutcome.of(Ceiling.COMPLETES, done = false, detail = "Notes is not running", app = "Notes")
        assertNotNull(out.recoveryAction)
        assertTrue(out.recoveryAction!!.isNotBlank())
    }

    @Test
    fun onlyAFailureOffersARecoveryAction() {
        val fine = CapabilityOutcome.of(Ceiling.COMPLETES, done = true, detail = "Wrote the note", app = "Notes")
        assertNull(fine.recoveryAction)
    }

    // ---- unverified sits next to the capability, not the task ------------

    @Test
    fun aCapabilityNeverMeasuredReadsAsUnverifiedNotAsAConfidentClaim() {
        val badge = CapabilityBadge.of(declared = Ceiling.COMPLETES, measured = null)
        assertEquals(StateMark.UNVERIFIED, badge.mark)
        assertEquals("Unverified", badge.label)
        assertEquals("the shown ceiling is still the declared one", Ceiling.COMPLETES, badge.shown)
    }

    @Test
    fun aMeasurementBelowTheDeclaredCeilingShowsTheMeasuredOneAndSaysDegraded() {
        val badge = CapabilityBadge.of(declared = Ceiling.COMPLETES, measured = Ceiling.ONE_TAP)
        assertEquals(Ceiling.ONE_TAP, badge.shown)
        assertEquals(StateMark.UNVERIFIED, badge.mark)
        assertEquals("Degraded", badge.label)
    }

    @Test
    fun aMeasurementCanNeverRaiseTheCeilingAboveWhatWasDeclared() {
        // A service that answers better than we promised does not get to promise
        // more on our behalf. Measurement demotes; it never promotes.
        val badge = CapabilityBadge.of(declared = Ceiling.ONE_TAP, measured = Ceiling.COMPLETES)
        assertEquals(Ceiling.ONE_TAP, badge.shown)
    }

    @Test
    fun aMeasurementThatMatchesTheDeclaredCeilingCarriesNoWarningMark() {
        val badge = CapabilityBadge.of(declared = Ceiling.COMPLETES, measured = Ceiling.COMPLETES)
        assertNull("a proven capability is not flagged", badge.mark)
        assertEquals(Ceiling.COMPLETES, badge.shown)
    }

    @Test
    fun theCeilingsRankInTheOrderTheCompanionUses() {
        // completes(3) beats one tap(2) beats hands off(1), same as the Go side,
        // because the two ranks are compared against each other over the wire.
        assertEquals(3, Ceiling.COMPLETES.rank)
        assertEquals(2, Ceiling.ONE_TAP.rank)
        assertEquals(1, Ceiling.HANDS_OFF.rank)
        assertEquals("completes", Ceiling.COMPLETES.wireName)
        assertEquals("one_tap", Ceiling.ONE_TAP.wireName)
        assertEquals("hands_off", Ceiling.HANDS_OFF.wireName)
        for (ceiling in Ceiling.entries) {
            assertEquals(ceiling, Ceiling.fromWire(ceiling.wireName))
        }
    }

    // ---- the words a user actually reads ---------------------------------

    @Test
    fun eachCeilingReadsDifferentlyOnTheTaskRow() {
        val done = CapabilityOutcome.of(Ceiling.COMPLETES, true, "Sent to Maya", "WhatsApp").label
        val tap = CapabilityOutcome.of(Ceiling.ONE_TAP, true, "Reply to Maya: hi", "WhatsApp").label
        val handed = CapabilityOutcome.of(Ceiling.HANDS_OFF, true, "Opened Uber", "Uber").label
        assertEquals(3, setOf(done, tap, handed).size)
        for (label in listOf(done, tap, handed)) {
            assertTrue(label.isNotBlank())
        }
    }
}
