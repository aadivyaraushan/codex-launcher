package app.codexlauncher.capability.reply.guard

import app.codexlauncher.diagnostics.AppLog
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/**
 * Names a conversation the way the rest of the reply path already knows it:
 * the app it lives in, plus the person as the user referred to them when they
 * set the exchange up (a display name, not a phone number or a handle).
 *
 * Two [ThreadKey]s are the same conversation when the package matches and the
 * person's name matches once case and surrounding spaces are ignored — the
 * user will type "maya" or "Maya" or " Maya " depending on mood, and all
 * three have to land on the same guard state.
 *
 * That normalising is the guard's own business and stops here. The [person]
 * this class stores is the exact string the caller handed in, untouched,
 * because that is the string that eventually reaches
 * `ReplyHandleSource.candidatesFor`, which matches against whatever Android
 * actually put in the notification shade. Tidying the name before it gets
 * there is how a reply meant for a real person misses its target.
 */
data class ThreadKey(val packageName: String, val person: String) {
    /**
     * The value the guard actually keys its maps by. Kept internal so nothing
     * outside this file is tempted to compare on it instead of on equality of
     * the whole [ThreadKey] — the normalising is a lookup trick, not a new
     * identity for a conversation.
     */
    internal val matchKey: String get() = packageName + " " + person.trim().lowercase()

    /**
     * The name fit to put on screen. [person] stays exactly as it arrived,
     * because it still has to match whatever Android put in the notification
     * shade — this is a separate reading of it for display, not a rewrite of
     * the stored value. Every run of whitespace collapses to a single space
     * and the ends are trimmed, so a stray newline can't split a button
     * across two lines. If nothing is left, "this conversation" stands in,
     * because a button has to say something.
     */
    val displayPerson: String
        get() = person.replace(Regex("\\s+"), " ").trim().ifEmpty { "this conversation" }
}

/** The three things `check` can decide, in order of how much the caller can do about them. */
enum class ReplyVerdict {
    /** Nothing stands in the way. The caller may go on to attempt the reply. */
    ALLOWED,

    /** The user asked for this conversation to stop. Waiting does nothing — only `resume` does. */
    STOPPED_BY_USER,

    /** This conversation has already used its allowance for the current window. Waiting fixes it. */
    TOO_MANY_IN_A_ROW,
}

/**
 * How many replies a single conversation may send inside a sliding window of
 * time, before the guard starts refusing on its own.
 *
 * A reply always answers a notification the person on the other end already
 * sent, and the user reads and confirms every reply before it goes out, so
 * this number is not a rule about how much somebody is allowed to say — it is
 * a backstop against a bug. If Operator answers the same conversation more
 * than a handful of times within a few minutes without the user starting a
 * fresh exchange in between, that is a loop repeating itself, not a
 * conversation happening quickly.
 *
 * [maxInWindow] and [windowMillis] are a product decision the owner has not
 * made yet — three replies in ten minutes is a placeholder that is obviously
 * not wrong, not a considered answer. They are named and declared in this one
 * place, with the reasoning next to them, so that changing them later is a
 * single edit here rather than a hunt through the reply path for a bare
 * number.
 */
data class ReplyCap(val maxInWindow: Int, val windowMillis: Long) {
    companion object {
        val shipped = ReplyCap(maxInWindow = 3, windowMillis = 10 * 60 * 1000L)
    }
}

/**
 * Remembers the two things about the person on the other end of a reply that
 * the rest of the reply path has no memory for: whether the user told it to
 * stop replying to them, and how many replies have already gone to them
 * recently.
 *
 * [check] decides a verdict without changing anything — a screen that shows
 * whether a reply would currently go through must be able to ask as often as
 * it likes without itself spending the allowance it is reporting on. Only
 * [recordSent] spends it, and it is the caller's job to call that once a
 * reply has actually gone out, not before.
 *
 * A stop always wins over the cap. Both are refusals, but they call for
 * different things from the person seeing them: waiting ten minutes clears a
 * cap and does nothing at all for a stop, so a stopped conversation reports
 * itself as stopped even if it also happens to be over its cap.
 *
 * The stop list and the send history are held in memory only. Restarting the
 * app forgets both. That is a real limit, not an oversight: the cap exists to
 * catch a loop firing within seconds of itself, not a slow drip spread across
 * days, so losing that count on restart costs nothing the cap was built to
 * catch. A stop is a longer-lived promise to the user, which is why it is
 * meant to be written to storage that survives a restart by whoever wires
 * this guard up — this class only holds it in memory for the lifetime it is
 * given.
 *
 * [lastReplied] is how a person actually reaches [stop]: it names the one
 * conversation a reply most recently went out to, so a screen can offer to
 * stop it. It only ever holds a conversation a reply **actually went out**
 * to — [recordSent] is the only thing that sets it, and that is called after
 * Android has accepted the text, never before. Offering to stop a
 * conversation Operator never spoke in would tell the user something untrue
 * about what their phone just did. Merely checking whether a reply would go
 * through ([check]) never touches it, so a screen may ask as often as it
 * likes without conjuring an offer out of nothing.
 */
class ReplyGuard(private val now: () -> Long, private val cap: ReplyCap) {
    // A reply can arrive on a network thread at the same moment a screen
    // listing stopped conversations reads on the main thread, so every read
    // and write to the state below goes through this one lock.
    private val lock = Any()

    // Keyed by ThreadKey.matchKey. The value is the ThreadKey exactly as it
    // was passed to stop(), so stopped() can hand it back the way the user
    // would recognise it, even though lookups happen on the normalised key.
    private val stoppedThreads = mutableMapOf<String, ThreadKey>()

    // Keyed by ThreadKey.matchKey. Each list holds the times a reply was sent
    // to that conversation, oldest first. recordSent prunes entries that have
    // aged out of the window so this cannot grow without bound on a phone
    // that stays on for weeks.
    private val sendHistory = mutableMapOf<String, MutableList<Long>>()

    // The conversation most recently handed a reply, exactly as the caller
    // spelled it — never the normalised matchKey — because a screen shows
    // this back to the user and hands it straight to stop().
    private val mutableLastReplied = MutableStateFlow<ThreadKey?>(null)

    val lastReplied: StateFlow<ThreadKey?> get() = mutableLastReplied

    fun check(key: ThreadKey): ReplyVerdict = synchronized(lock) {
        if (stoppedThreads.containsKey(key.matchKey)) {
            logRefusal(ReplyVerdict.STOPPED_BY_USER, key)
            return ReplyVerdict.STOPPED_BY_USER
        }

        val cutoff = now() - cap.windowMillis
        val sentInWindow = sendHistory[key.matchKey]?.count { it > cutoff } ?: 0
        if (sentInWindow >= cap.maxInWindow) {
            logRefusal(ReplyVerdict.TOO_MANY_IN_A_ROW, key)
            return ReplyVerdict.TOO_MANY_IN_A_ROW
        }

        ReplyVerdict.ALLOWED
    }

    fun recordSent(key: ThreadKey) = synchronized(lock) {
        val cutoff = now() - cap.windowMillis
        val history = sendHistory.getOrPut(key.matchKey) { mutableListOf() }
        history.removeAll { it <= cutoff }
        history.add(now())
        mutableLastReplied.value = key
    }

    fun stop(key: ThreadKey) = synchronized(lock) {
        stoppedThreads[key.matchKey] = key
        // Once this conversation has been acted on there is nothing left to
        // offer — leaving the offer up invites a second tap that appears to
        // do nothing. An offer for a different conversation is untouched.
        if (mutableLastReplied.value?.matchKey == key.matchKey) {
            mutableLastReplied.value = null
        }
    }

    fun resume(key: ThreadKey) = synchronized(lock) {
        stoppedThreads.remove(key.matchKey)
    }

    /** Clears whatever offer is currently showing, without stopping anything. */
    fun dismissOffer() = synchronized(lock) {
        mutableLastReplied.value = null
    }

    fun stopped(): Set<ThreadKey> = synchronized(lock) { stoppedThreads.values.toSet() }

    private fun logRefusal(verdict: ReplyVerdict, key: ThreadKey) {
        // The person's name never appears in a log line, only which package
        // it happened in and which of the two refusals it was.
        AppLog.info(
            feature = "reply",
            message = "reply guard refused",
            fields = mapOf("verdict" to verdict.name, "package" to key.packageName),
        )
    }
}
