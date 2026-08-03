package app.codexlauncher.capability.notifications

import android.app.Notification
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import app.codexlauncher.capability.reply.AndroidReplyDispatch
import app.codexlauncher.capability.reply.LiveReplyActions
import app.codexlauncher.capability.reply.ReplyHandle
import app.codexlauncher.capability.reply.access.DeviceNotificationAccess
import app.codexlauncher.capability.reply.live.LiveReplyBoxes
import app.codexlauncher.diagnostics.AppLog
import java.io.File

/**
 * Phase 0 probe. Answers one question per app and then gets deleted: when a
 * messaging app posts a notification, does it attach a reply box we can fire
 * without opening the app?
 *
 * A reply box is a [Notification.Action] carrying a RemoteInput that accepts
 * free text — the same thing a smartwatch uses to answer a message. If no
 * watched app attaches one, phone mode cannot send, and the plan falls back to
 * drafting the reply and opening the app with it loaded.
 *
 * The rules live in [ReplyCapability] and [ProbeLedger], which are plain data
 * and have unit tests. This class only translates Android's notification into
 * a [NotificationSighting] and writes down what the ledger concluded.
 *
 * Message content is never read into any field here. Only its shape, because
 * the question is about capability and these are real conversations.
 */
class NotificationProbeService : NotificationListenerService() {

    private companion object {
        const val FEATURE = "phase0-probe"
        const val LEDGER_FILE = "notification-reply-probe.txt"

        // Only notifications from the handful of watched messaging apps ever
        // reach this far, and only the ones that actually carry a reply box
        // get retained at all — so even a phone that is busy all day only
        // ever needs a few live conversations remembered at once. 20 leaves
        // headroom for that without letting a misbehaving source grow this
        // store without bound, since each entry held here is a live
        // PendingIntent, i.e. real permission to act.
        const val LIVE_REPLY_ACTION_CAP = 20
    }

    private val ledger = ProbeLedger()

    /**
     * The live counterpart to the ledger: not "this app can reply" but "here
     * is the actual Android action to fire if we do". Exposed as a plain
     * property because that is all the rest of the app needs — the store
     * itself already keeps the notification's key as the only way in.
     */
    val liveReplyActions = LiveReplyActions<Notification.Action>(LIVE_REPLY_ACTION_CAP)

    /**
     * The live counterpart to [liveReplyActions] that also remembers *who* a
     * conversation is with. The Mac only ever names a person, never Android's
     * per-notification key, so something on the phone has to bridge that gap
     * — and the probe's own types are deliberately built to hold no names at
     * all. The name lives only here, only for as long as the notification is
     * on screen, and never reaches a sighting, the on-disk ledger, or a log
     * line; see the class doc on [LiveReplyBoxes] for why.
     */
    val liveReplyBoxes = LiveReplyBoxes(LIVE_REPLY_ACTION_CAP)

    // The dispatch this service installed into DeviceNotificationAccess, kept
    // so teardown can release the exact instance it registered rather than
    // whatever happens to be installed at the time — that identity check is
    // what keeps a stale teardown from unregistering a newer connection.
    private var installedDispatch: AndroidReplyDispatch? = null

    override fun onListenerConnected() {
        AppLog.info(
            FEATURE,
            "listener connected",
            mapOf("watching" to WatchList.PACKAGES.values.toSortedSet().joinToString(",")),
        )

        val dispatch = AndroidReplyDispatch(this, liveReplyActions)
        installedDispatch = dispatch
        DeviceNotificationAccess.install(dispatch, liveReplyBoxes)
        AppLog.info(FEATURE, "reply dispatch installed")

        // Re-inspect whatever is already in the shade, so iterating on the probe
        // doesn't require a fresh message to be sent each time.
        val active = runCatching { activeNotifications?.toList().orEmpty() }
            .onFailure { AppLog.error(FEATURE, "could not read active notifications", it) }
            .getOrDefault(emptyList())

        AppLog.info(FEATURE, "sweeping active notifications", mapOf("total" to active.size))
        active.forEach { sbn ->
            runCatching { inspect(sbn) }
                .onFailure { AppLog.error(FEATURE, "sweep inspection failed", it) }
        }
        writeLedger()
    }

    override fun onListenerDisconnected() {
        // Losing the listener connection means losing the permission that
        // let us hold these actions in the first place, so every one of them
        // goes at once rather than sitting around stale until reconnect.
        liveReplyActions.clear()
        liveReplyBoxes.clear()
        releaseReplyDispatch()
        AppLog.info(FEATURE, "listener disconnected")
    }

    override fun onDestroy() {
        // Android does not always deliver onListenerDisconnected before
        // tearing the service down, so teardown has to release here too.
        releaseReplyDispatch()
        super.onDestroy()
    }

    private fun releaseReplyDispatch() {
        val dispatch = installedDispatch ?: return
        DeviceNotificationAccess.release(dispatch)
        installedDispatch = null
        AppLog.info(FEATURE, "reply dispatch released")
    }

    override fun onNotificationPosted(sbn: StatusBarNotification?) {
        val posted = sbn ?: return
        runCatching { inspect(posted) }
            .onFailure { AppLog.error(FEATURE, "inspection failed", it) }
    }

    override fun onNotificationRemoved(sbn: StatusBarNotification?) {
        // Whatever action we were holding for this notification is only
        // valid while the notification is still on screen. The moment it is
        // gone, so is our reason to keep the action around.
        val removed = sbn ?: return
        liveReplyActions.forget(removed.key)
        liveReplyBoxes.forget(removed.key)
    }

    private fun inspect(sbn: StatusBarNotification) {
        val sighting = sbn.toSighting()
        val entry = ledger.record(sighting) ?: return
        retainLiveReplyAction(sbn, ReplyCapability.classify(sighting))

        AppLog.info(
            FEATURE,
            when (entry.verdict) {
                ProbeVerdict.CAN_REPLY -> "REPLY BOX FOUND"
                ProbeVerdict.NO_REPLY_BOX -> "NO REPLY BOX"
                ProbeVerdict.NOT_MEASURED -> "IGNORED"
            },
            mapOf(
                "app" to (entry.label ?: "unwatched"),
                "package" to entry.packageName,
                "verdict" to entry.verdict.name,
                "sightings" to entry.sightings,
                "reply_source" to entry.source.name,
                "reply_label" to (entry.replyLabel ?: "none"),
                "remote_input_key" to (entry.remoteInputKey ?: "none"),
                "canned_only_actions" to entry.cannedOnlyActionCount,
                "group_summaries_ignored" to entry.groupSummariesIgnored,
                "non_message_alerts_ignored" to entry.nonMessageAlertsIgnored,
                "is_group_summary" to sighting.isGroupSummary,
                "shade_action_count" to sighting.shadeActions.size,
                "shade_action_labels" to sighting.shadeActions.joinToString("|") { it.label },
                "wear_action_count" to sighting.wearableActions.size,
                "wear_action_labels" to sighting.wearableActions.joinToString("|") { it.label },
                "category" to (sighting.category ?: "none"),
                "template" to (sighting.template ?: "none"),
                "message_count" to sighting.messageCount,
                "body_shape" to if (sighting.bodyPresent) "present,length=${sighting.bodyLength}" else "absent",
            ),
        )

        writeLedger()
    }

    /**
     * Flattens one Android notification into the shape the rules understand.
     * A reply box can live in either place: the main actions array is what the
     * shade draws, and WearableExtender is where a lot of apps put the watch
     * reply action instead, which is the mechanism this phase asks about.
     */
    @Suppress("DEPRECATION") // The typed getParcelableArray needs API 33; minSdk here is 31.
    private fun StatusBarNotification.toSighting(): NotificationSighting {
        val shade = notification.actions?.toList().orEmpty().map { it.toProbeAction() }
        val wear = runCatching { Notification.WearableExtender(notification).actions }
            .getOrDefault(emptyList())
            .map { it.toProbeAction() }
        val body = notification.extras.getCharSequence(Notification.EXTRA_TEXT)

        return NotificationSighting(
            packageName = packageName,
            shadeActions = shade,
            wearableActions = wear,
            isGroupSummary = (notification.flags and Notification.FLAG_GROUP_SUMMARY) != 0,
            category = notification.category,
            template = notification.extras.getString(Notification.EXTRA_TEMPLATE),
            messageCount = notification.extras.getParcelableArray(Notification.EXTRA_MESSAGES)?.size ?: 0,
            bodyLength = body?.length ?: 0,
            bodyPresent = body != null,
        )
    }

    /**
     * Keeps the real Android action behind whatever reply box [ReplyCapability]
     * just found, so a later reply attempt has something to fire — not just a
     * description of one. The rule for which action counts as the reply box
     * lives entirely in [ReplyCapability.classify]; this only counts to the
     * index it was handed, in the same list ([source] says which) that
     * [toSighting] already read it from. If that list turns out shorter than
     * the index — a notification that changed shape between being classified
     * and being read again here — the honest move is to retain nothing.
     */
    @Suppress("DEPRECATION") // Same minSdk 31 constraint as toSighting().
    private fun retainLiveReplyAction(sbn: StatusBarNotification, finding: ReplyFinding) {
        // Every path that does not end in remembering has to forget first.
        // Messaging apps re-post the same conversation on every new message,
        // and the new post can drop the reply box the old one had — a chat
        // archived, a group left, a notification downgraded to a summary.
        // Returning early there would leave the earlier post's action sitting
        // under this same key, which is a live PendingIntent for a reply box
        // that no longer exists.
        val index = finding.replyActionIndex
        if (index == null) {
            liveReplyActions.forget(sbn.key)
            liveReplyBoxes.forget(sbn.key)
            return
        }
        val actions = when (finding.source) {
            ReplySource.SHADE -> sbn.notification.actions?.toList().orEmpty()
            ReplySource.WEARABLE_EXTENDER -> runCatching { Notification.WearableExtender(sbn.notification).actions }
                .getOrDefault(emptyList())
            ReplySource.NONE -> emptyList()
        }
        val action = actions.getOrNull(index)
        if (action == null) {
            liveReplyActions.forget(sbn.key)
            liveReplyBoxes.forget(sbn.key)
            return
        }
        liveReplyActions.remember(sbn.key, action)

        // The title is who this conversation is with, not what it says — see
        // the privacy note on liveReplyBoxes. Without a title there is no
        // person to route a reply to later, so the honest move is the same
        // as an unusable action: forget, don't remember half a box.
        val person = sbn.notification.extras.getCharSequence(Notification.EXTRA_TITLE)?.toString()
        if (person.isNullOrBlank()) {
            liveReplyBoxes.forget(sbn.key)
            return
        }
        liveReplyBoxes.remember(
            conversationKey = sbn.key,
            person = person,
            handle = ReplyHandle(
                packageName = sbn.packageName,
                appLabel = WatchList.label(sbn.packageName) ?: sbn.packageName,
                verdict = ProbeVerdict.CAN_REPLY,
                remoteInputKey = finding.remoteInputKey,
                source = finding.source,
                notificationLive = true,
                conversationKey = sbn.key,
            ),
        )
    }

    private fun Notification.Action.toProbeAction(): ProbeAction = ProbeAction(
        label = title?.toString() ?: "?",
        remoteInputs = remoteInputs?.toList().orEmpty().map { input ->
            ProbeRemoteInput(
                resultKey = input.resultKey ?: "?",
                allowFreeFormInput = input.allowFreeFormInput,
                choiceCount = input.choices?.size ?: 0,
            )
        },
    )

    /**
     * The probe's whole output, on disk, so the answer survives logcat being
     * cleared and can be pulled off the phone in one command. Overwritten on
     * every sighting; it is a current verdict, not a history.
     */
    private fun writeLedger() {
        runCatching {
            val rows = ledger.report(WatchList.PACKAGES.keys)
                .sortedWith(compareBy({ it.label ?: "zz-unwatched" }, { it.packageName }))
                .joinToString("\n") { entry ->
                    listOf(
                        "app=${entry.label ?: "unwatched"}",
                        "package=${entry.packageName}",
                        "verdict=${entry.verdict.name}",
                        "sightings=${entry.sightings}",
                        "source=${entry.source.name}",
                        "reply_label=${entry.replyLabel ?: "none"}",
                        "remote_input_key=${entry.remoteInputKey ?: "none"}",
                        "canned_only=${entry.cannedOnlyActionCount}",
                        "summaries_ignored=${entry.groupSummariesIgnored}",
                        "non_message_alerts_ignored=${entry.nonMessageAlertsIgnored}",
                        "template=${entry.lastTemplate ?: "none"}",
                    ).joinToString(" ")
                }
            File(filesDir, LEDGER_FILE).writeText(rows + "\n")
        }.onFailure { AppLog.error(FEATURE, "could not write probe ledger", it) }
    }
}
