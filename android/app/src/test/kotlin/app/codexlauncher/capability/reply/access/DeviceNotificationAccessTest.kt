package app.codexlauncher.capability.reply.access

import app.codexlauncher.capability.reply.DeliveryResult
import app.codexlauncher.capability.reply.ReplyDispatch
import app.codexlauncher.capability.reply.request.ReplyHandleSource
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * The whole RT-4 reply stack is written and tested and nothing in the running
 * app can reach it, because Android constructs a NotificationListenerService
 * itself. There is no constructor to inject into and no useful binder, so the
 * ordinary answer is one process-wide holder the service fills in when it
 * connects and empties when it goes away.
 *
 * Two rules earn this file. The first is that empty means *there is no reply
 * box right now* — never *nobody has checked yet*. A holder that keeps a
 * service Android has already torn down hands out permission slips for
 * notifications that no longer exist, and every reply fired through them would
 * be reported as sent.
 *
 * The second is subtler and is the reason for [releasingAStaleDispatchLeavesTheLiveOneAlone].
 * Android is free to build the replacement service before it destroys the old
 * one. If the old instance's teardown clears the holder unconditionally, it
 * wipes the *new* instance's registration, and from then on every reply on a
 * perfectly healthy phone answers "that notification is gone" forever, with
 * nothing in the logs to say why.
 */
class DeviceNotificationAccessTest {

    /**
     * A dispatch and its box search are installed and released together, so
     * every test here has to supply both. These tests are about which dispatch
     * is currently installed, so the search itself can be empty — but it
     * cannot be absent, because a live dispatch with no way to find a
     * conversation would answer "that notification is gone" to every reply on
     * a perfectly healthy phone.
     */
    private val noBoxes = ReplyHandleSource { emptyList() }

    /** Stands in for AndroidReplyDispatch. Records nothing; identity is the point. */
    private class FakeDispatch(val name: String) : ReplyDispatch {
        override fun deliver(conversationKey: String, remoteInputKey: String, text: String) =
            DeliveryResult.HANDED_TO_THE_APP
    }

    /**
     * The holder is process-wide by design, so it survives between tests the
     * way it survives between screens. Emptying it first is what keeps each
     * test honest about what it installed.
     */
    @Before
    fun clearHolder() {
        DeviceNotificationAccess.current()?.let { DeviceNotificationAccess.release(it) }
    }

    @Test
    fun withNoListenerConnectedThereIsNoReplyBox() {
        assertNull(DeviceNotificationAccess.current())
    }

    @Test
    fun aConnectedListenerCanBeReachedByTheRestOfTheApp() {
        val dispatch = FakeDispatch("connected")
        DeviceNotificationAccess.install(dispatch, noBoxes)

        assertSame(dispatch, DeviceNotificationAccess.current())
    }

    /**
     * Losing the listener connection loses the permission that made those
     * reply boxes fireable. The holder has to go empty at the same moment, not
     * keep handing out a dispatch that will only ever fail.
     */
    @Test
    fun aDisconnectedListenerLeavesNothingBehind() {
        val dispatch = FakeDispatch("connected")
        DeviceNotificationAccess.install(dispatch, noBoxes)
        DeviceNotificationAccess.release(dispatch)

        assertNull(DeviceNotificationAccess.current())
    }

    @Test
    fun releasingWhenNothingWasEverInstalledIsFine() {
        DeviceNotificationAccess.release(FakeDispatch("never-installed"))

        assertNull(DeviceNotificationAccess.current())
    }

    /**
     * A reconnect installs a fresh dispatch over the old one. The old one is
     * built on a listener connection that no longer exists, so it must not be
     * what a caller gets.
     */
    @Test
    fun reconnectingReplacesTheOlderDispatch() {
        val first = FakeDispatch("first")
        val second = FakeDispatch("second")
        DeviceNotificationAccess.install(first, noBoxes)
        DeviceNotificationAccess.install(second, noBoxes)

        assertSame(second, DeviceNotificationAccess.current())
    }

    /**
     * The one that keeps a healthy phone working. Android builds the new
     * service, the new service installs itself, and only then does the old
     * service's teardown run. Its release names a dispatch that is no longer
     * the current one, and must be ignored.
     */
    @Test
    fun releasingAStaleDispatchLeavesTheLiveOneAlone() {
        val stale = FakeDispatch("stale")
        val live = FakeDispatch("live")
        DeviceNotificationAccess.install(stale, noBoxes)
        DeviceNotificationAccess.install(live, noBoxes)

        DeviceNotificationAccess.release(stale)

        assertSame(
            "an outgoing service's teardown must not unregister its replacement",
            live,
            DeviceNotificationAccess.current(),
        )
    }

    /**
     * The service installs on Android's own thread; the socket that carries a
     * reply request reads from another. Neither side may see a half-written
     * holder, and a reader must only ever get a dispatch that was genuinely
     * installed at some point — never a torn or invented one.
     */
    @Test
    fun readingWhileTheListenerConnectsIsSafe() {
        val installed = List(8) { FakeDispatch("dispatch-$it") }
        val known = installed.toSet()
        val start = CountDownLatch(1)
        val seen = java.util.Collections.synchronizedList(mutableListOf<ReplyDispatch>())

        val writer = Thread {
            start.await()
            installed.forEach { DeviceNotificationAccess.install(it, noBoxes) }
        }
        val readers = List(4) {
            Thread {
                start.await()
                repeat(200) { DeviceNotificationAccess.current()?.let(seen::add) }
            }
        }

        (readers + writer).forEach(Thread::start)
        start.countDown()
        (readers + writer).forEach { it.join(TimeUnit.SECONDS.toMillis(5)) }

        assertTrue("every value a reader saw must be one that was installed", seen.all(known::contains))
        assertEquals(installed.last(), DeviceNotificationAccess.current())
    }
}
