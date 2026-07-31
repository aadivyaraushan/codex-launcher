package app.codexlauncher.capability.notifications

import android.app.Notification
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
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
    }

    private val ledger = ProbeLedger()

    override fun onListenerConnected() {
        AppLog.info(
            FEATURE,
            "listener connected",
            mapOf("watching" to WatchList.PACKAGES.values.toSortedSet().joinToString(",")),
        )

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
        AppLog.info(FEATURE, "listener disconnected")
    }

    override fun onNotificationPosted(sbn: StatusBarNotification?) {
        val posted = sbn ?: return
        runCatching { inspect(posted) }
            .onFailure { AppLog.error(FEATURE, "inspection failed", it) }
    }

    private fun inspect(sbn: StatusBarNotification) {
        val sighting = sbn.toSighting()
        val entry = ledger.record(sighting) ?: return

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
