package app.codexlauncher.capability.reply.guard

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * "Stop" has to work, and nothing may fire into the same conversation over
 * and over.
 *
 * These are rules four and two from the plan's list about the person on the
 * other end of the reply, and they are the two the code does not have. Every
 * other rule there is a design constraint that is already satisfied by the
 * shape of the system — Operator never signs a message as a person, it only
 * ever replies into a thread somebody else started, the user reads the words
 * first. These two need a thing that remembers.
 *
 * Inputs: a conversation, named by the app it lives in and the person as the
 * user referred to them; and the history of what this guard has already let
 * through.
 *
 * Output: one of three verdicts, decided before any Android call is made.
 *
 *	the user said stop for this person   ->  STOPPED_BY_USER
 *	too many already, too recently       ->  TOO_MANY_IN_A_ROW
 *	otherwise                            ->  ALLOWED
 *
 * Both refusals become `refused` on the wire — the word that means we know
 * for certain nothing was sent, which is exactly true here. The reason stays
 * on the phone, like the notification-access reason does, because the wire's
 * four words are fixed and a fifth one is dropped by the Mac's decoder.
 *
 * The cap is not enforced across a restart on purpose, and that is a real
 * limit worth stating rather than hiding: a guard that forgets is still worth
 * having, because the case it exists for is a loop firing within seconds, not
 * a slow drip across days. The stop list is the half that must survive, and
 * it is kept separately for that reason.
 */
class ReplyGuardTest {

    private var clock = 0L
    private val cap = ReplyCap(maxInWindow = 3, windowMillis = 10 * 60 * 1000L)
    private val guard = ReplyGuard(now = { clock }, cap = cap)

    private val maya = ThreadKey(packageName = "com.whatsapp", person = "Maya")

    @Test
    fun aFirstReplyToSomebodyIsAllowed() {
        assertEquals(ReplyVerdict.ALLOWED, guard.check(maya))
    }

    @Test
    fun repliesUpToTheCapAreAllowedAndTheOneAfterIsNot() {
        repeat(cap.maxInWindow) {
            assertEquals(ReplyVerdict.ALLOWED, guard.check(maya))
            guard.recordSent(maya)
        }

        assertEquals(ReplyVerdict.TOO_MANY_IN_A_ROW, guard.check(maya))
    }

    /**
     * The cap counts a window, not a lifetime. Somebody who exchanged three
     * messages this morning is not banned from replying this afternoon.
     */
    @Test
    fun theCapLetsGoOnceTheWindowHasPassed() {
        repeat(cap.maxInWindow) { guard.recordSent(maya) }
        assertEquals(ReplyVerdict.TOO_MANY_IN_A_ROW, guard.check(maya))

        clock += cap.windowMillis + 1

        assertEquals(ReplyVerdict.ALLOWED, guard.check(maya))
    }

    /**
     * The cap is per conversation. A busy morning with one person must not
     * silently stop Operator answering somebody else.
     */
    @Test
    fun oneBusyConversationDoesNotBlockAnother() {
        repeat(cap.maxInWindow) { guard.recordSent(maya) }
        assertEquals(ReplyVerdict.TOO_MANY_IN_A_ROW, guard.check(maya))

        val sam = ThreadKey(packageName = "com.whatsapp", person = "Sam")
        assertEquals(ReplyVerdict.ALLOWED, guard.check(sam))
    }

    /**
     * Same person, different app, is a different conversation. Answering
     * Maya on WhatsApp says nothing about whether Operator should also be
     * answering her on Instagram.
     */
    @Test
    fun theSamePersonInTwoAppsIsTwoConversations() {
        repeat(cap.maxInWindow) { guard.recordSent(maya) }

        val mayaElsewhere = ThreadKey(packageName = "com.instagram.android", person = "Maya")
        assertEquals(ReplyVerdict.ALLOWED, guard.check(mayaElsewhere))
    }

    @Test
    fun stopMeansStopRegardlessOfHowQuietTheConversationHasBeen() {
        guard.stop(maya)

        assertEquals(ReplyVerdict.STOPPED_BY_USER, guard.check(maya))
    }

    /**
     * A stop outranks a cap. Both refuse, but the person needs to be told the
     * right one: waiting ten minutes fixes a cap and does nothing at all for
     * a stop, and telling somebody to wait when the answer is "you turned
     * this off" sends them back to watch a clock for no reason.
     */
    @Test
    fun aStoppedConversationSaysStoppedNotTooMany() {
        repeat(cap.maxInWindow) { guard.recordSent(maya) }
        guard.stop(maya)

        assertEquals(ReplyVerdict.STOPPED_BY_USER, guard.check(maya))
    }

    @Test
    fun aStopCanBeLiftedAgain() {
        guard.stop(maya)
        guard.resume(maya)

        assertEquals(ReplyVerdict.ALLOWED, guard.check(maya))
    }

    @Test
    fun stoppedConversationsCanBeListedSoTheUserCanSeeAndUndoThem() {
        val sam = ThreadKey(packageName = "com.instagram.android", person = "Sam")
        guard.stop(maya)
        guard.stop(sam)
        guard.resume(sam)

        assertEquals(setOf(maya), guard.stopped())
    }

    /**
     * The user says "stop replying to maya" in whatever case they feel like,
     * and it has to catch the conversation they set up as "Maya". So the key
     * compares case-insensitively and ignores surrounding spaces.
     *
     * This normalising is the guard's own business and stops here. The name
     * that goes on to the phone's notification matcher is still the one the
     * user typed, untouched — `ReplyHandleSource.candidatesFor` matches
     * against what Android actually put in the shade, and tidying the name
     * before it gets there is how a reply to a real person misses.
     */
    @Test
    fun aStopSetOnOneSpellingOfANameCatchesTheOthers() {
        guard.stop(ThreadKey(packageName = "com.whatsapp", person = "  MAYA "))

        assertEquals(ReplyVerdict.STOPPED_BY_USER, guard.check(maya))
    }

    @Test
    fun aStopDoesNotLeakAcrossPeopleWhoseNamesMerelyStartTheSame() {
        guard.stop(maya)

        val mayaK = ThreadKey(packageName = "com.whatsapp", person = "Maya K")
        assertEquals(ReplyVerdict.ALLOWED, guard.check(mayaK))
    }

    /**
     * Checking is not sending. A verdict has to be free of side effects, or a
     * screen that shows whether a reply would go through would itself spend
     * the allowance it is reporting on.
     */
    @Test
    fun askingWhetherAReplyIsAllowedDoesNotUseUpTheAllowance() {
        repeat(20) { assertEquals(ReplyVerdict.ALLOWED, guard.check(maya)) }
    }

    /**
     * The cap's numbers are a product decision, not a fact about the code, so
     * they are declared in one named place with the reasoning next to them
     * rather than written as bare numbers at the point of use. This test only
     * holds that the shipped default exists and is sane; the owner sets the
     * values.
     */
    @Test
    fun theShippedCapIsDeclaredInOnePlaceAndIsNotZero() {
        assertTrue(ReplyCap.shipped.maxInWindow > 0)
        assertTrue(ReplyCap.shipped.windowMillis > 0)
    }
}
