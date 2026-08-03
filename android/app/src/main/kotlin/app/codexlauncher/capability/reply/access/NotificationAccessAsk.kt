package app.codexlauncher.capability.reply.access

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/**
 * Which of the two situations a blocked reply is actually in, worked out from
 * two readings taken at the moment the reply was attempted: whether Android
 * lists Operator as an enabled notification listener, and whether that
 * listener's service is connected right this instant.
 *
 * Both readings point at the same holder in `DeviceNotificationAccess`, but
 * they answer different questions. The OS-level grant is a setting a person
 * flips once and Android remembers forever. The live connection is Android's
 * own bookkeeping about whether the listener service happens to be running
 * right now, and it goes away for a few seconds around an app upgrade, a
 * force-stop, or low memory — none of which have anything to do with the
 * permission. Collapsing the two into one blocked/not-blocked flag would send
 * somebody to a settings screen that already says On.
 */
enum class ReplyBlock {
    /** Nothing is wrong. The listener is connected and a reply can be attempted. */
    NONE,

    /** Android has never been told Operator may read and reply to notifications. This is worth asking about: a settings screen has a fix. */
    ACCESS_NEVER_GRANTED,

    /** The permission is on, but the listener service is not connected at this exact moment. This is a passing gap, not a missing permission, so nothing is worth asking. */
    LISTENER_NOT_CONNECTED,

    ;

    /** Only a missing permission is worth interrupting somebody about — the other blocked case fixes itself. */
    val worthAsking: Boolean get() = this == ACCESS_NEVER_GRANTED

    companion object {
        /**
         * Android will not report a package as connected unless that package is
         * also enabled, so [grantedAtOsLevel] false with [listenerConnected] true
         * is a combination that should never arrive in practice. If it somehow
         * does, this treats it as a missing permission anyway: that is the
         * reading which offers a fix, where the alternative reading would tell
         * somebody to wait for a connection that is not coming.
         */
        fun of(grantedAtOsLevel: Boolean, listenerConnected: Boolean): ReplyBlock = when {
            !grantedAtOsLevel -> ACCESS_NEVER_GRANTED
            !listenerConnected -> LISTENER_NOT_CONNECTED
            else -> NONE
        }
    }
}

/**
 * Holds the one blocked reply the screen should currently be talking about,
 * if any.
 *
 * A reply that is blocked for a reason worth asking about calls [record] to
 * raise it, and the screen shows an ask for as long as that value is
 * [ReplyBlock.worthAsking]. [dismiss] only closes that one dialog — it does
 * not remember the refusal, because the permission can be granted or lost at
 * any point outside the app's control, and the next blocked reply is a fresh
 * moment where asking is genuinely the right thing to do again.
 */
class NotificationAccessAsk {
    private val current = MutableStateFlow(ReplyBlock.NONE)

    val state: StateFlow<ReplyBlock> get() = current

    /** Records the outcome of the reading taken for the reply that was just attempted. */
    fun record(block: ReplyBlock) {
        current.value = block
    }

    /** Closes whatever ask is currently showing, without remembering that it happened. */
    fun dismiss() {
        current.value = ReplyBlock.NONE
    }
}
