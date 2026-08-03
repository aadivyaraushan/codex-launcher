package app.codexlauncher.capability.notifications

/**
 * Phase 0's one question, as data rather than as an Android class.
 *
 * When a messaging app posts a notification, does it attach a box we can type
 * into and send from — without opening the app? Everything Android-specific is
 * flattened into [NotificationSighting] at the service boundary so the rules
 * below can be tested on a laptop in milliseconds.
 *
 * Message text never enters any type in this file. There is no field able to
 * hold it.
 */

/** One typed field on a notification action. Android calls this a RemoteInput. */
data class ProbeRemoteInput(
    val resultKey: String,
    val allowFreeFormInput: Boolean,
    val choiceCount: Int,
)

/** One button on a notification. */
data class ProbeAction(
    val label: String,
    val remoteInputs: List<ProbeRemoteInput>,
)

/** One notification, reduced to the parts that decide the question. */
data class NotificationSighting(
    val packageName: String,
    val shadeActions: List<ProbeAction>,
    val wearableActions: List<ProbeAction>,
    val isGroupSummary: Boolean,
    val category: String?,
    val template: String?,
    val messageCount: Int,
    val bodyLength: Int,
    val bodyPresent: Boolean,
)

/** Where a reply box was found, since the two places behave differently. */
enum class ReplySource { SHADE, WEARABLE_EXTENDER, NONE }

/** What one sighting proves. */
data class ReplyFinding(
    val packageName: String,
    val canReply: Boolean,
    val source: ReplySource,
    val replyLabel: String?,
    val remoteInputKey: String?,
    /**
     * Where the chosen action sits in the list it came from — the shade list
     * when [source] is SHADE, the watch-extender list when it is
     * WEARABLE_EXTENDER. Null whenever there is no usable reply box, since an
     * index of 0 would otherwise look like "the first action" instead of
     * "nothing to point at". The listener needs this to reach back into the
     * live Android action later; see the note on [ReplyCapability.classify].
     */
    val replyActionIndex: Int?,
    val cannedOnlyActionCount: Int,
    val shadeActionCount: Int,
    val wearableActionCount: Int,
    val isGroupSummary: Boolean,
    val category: String?,
    val template: String?,
    val messageCount: Int,
    val bodyPresent: Boolean,
    val bodyLength: Int,
)

object ReplyCapability {

    /**
     * A reply box is an action carrying a typed field that accepts our own
     * words. An action offering only canned choices — the three phrases the
     * phone suggests — is deliberately a no: Operator has to send what the
     * user meant, not pick from a menu.
     *
     * The shade wins over the watch extender when both have one, because the
     * shade action is the one Android itself draws and is the more stable of
     * the two.
     *
     * The index carried on the result is not decoration: firing a reply means
     * holding the real Android action, and only the notification listener can
     * see those — it gets a list and has to pick the same one out of it that
     * this function already picked. Re-deriving the rule in the listener would
     * be a second copy that can drift silently, so classify names the position
     * instead and the listener just counts to it.
     */
    fun classify(sighting: NotificationSighting): ReplyFinding {
        val shadeReply = sighting.shadeActions.findUsableReply()
        val wearReply = sighting.wearableActions.findUsableReply()

        val chosen = shadeReply ?: wearReply
        val source = when {
            shadeReply != null -> ReplySource.SHADE
            wearReply != null -> ReplySource.WEARABLE_EXTENDER
            else -> ReplySource.NONE
        }

        val cannedOnly = (sighting.shadeActions + sighting.wearableActions)
            .count { action -> action.remoteInputs.isNotEmpty() && action.usableReply() == null }

        return ReplyFinding(
            packageName = sighting.packageName,
            canReply = chosen != null,
            source = source,
            replyLabel = chosen?.label,
            remoteInputKey = chosen?.input?.resultKey,
            replyActionIndex = chosen?.index,
            cannedOnlyActionCount = cannedOnly,
            shadeActionCount = sighting.shadeActions.size,
            wearableActionCount = sighting.wearableActions.size,
            isGroupSummary = sighting.isGroupSummary,
            category = sighting.category,
            template = sighting.template,
            messageCount = sighting.messageCount,
            bodyPresent = sighting.bodyPresent,
            bodyLength = sighting.bodyLength,
        )
    }

    private fun ProbeAction.usableReply(): Pair<String, ProbeRemoteInput>? {
        val input = remoteInputs.firstOrNull { it.allowFreeFormInput } ?: return null
        return label to input
    }

    /** The usable reply box in a list, plus where it sits in that same list. */
    private data class ChosenReply(val index: Int, val label: String, val input: ProbeRemoteInput)

    private fun List<ProbeAction>.findUsableReply(): ChosenReply? {
        forEachIndexed { index, action ->
            val reply = action.usableReply() ?: return@forEachIndexed
            return ChosenReply(index, reply.first, reply.second)
        }
        return null
    }
}

/** The five apps Wave 0 must answer for, plus the aliases each ships under. */
object WatchList {
    val PACKAGES: Map<String, String> = mapOf(
        "com.google.android.apps.messaging" to "messages",
        "com.whatsapp" to "whatsapp",
        "com.whatsapp.w4b" to "whatsapp",
        "com.instagram.android" to "instagram",
        "com.facebook.orca" to "messenger",
        "com.facebook.mlite" to "messenger",
        "org.thoughtcrime.securesms" to "signal",
    )

    fun label(packageName: String): String? = PACKAGES[packageName]
}

/** What we are willing to say about one app. */
enum class ProbeVerdict {
    /** Seen, and a usable reply box was attached at least once. */
    CAN_REPLY,

    /** Seen at least once as a real message, never with a usable reply box. */
    NO_REPLY_BOX,

    /** Never seen. Not an answer, and must never be rendered as one. */
    NOT_MEASURED,
}

/** What the probe knows about one app, accumulated across sightings. */
data class LedgerEntry(
    val packageName: String,
    val label: String?,
    val verdict: ProbeVerdict,
    val sightings: Int,
    val groupSummariesIgnored: Int,
    val nonMessageAlertsIgnored: Int,
    val source: ReplySource,
    val replyLabel: String?,
    val remoteInputKey: String?,
    val cannedOnlyActionCount: Int,
    val lastTemplate: String?,
    val lastCategory: String?,
)

/**
 * Accumulates findings per app.
 *
 * The rule that matters: **a yes is never taken back.** Bundled group
 * summaries ("3 new messages") carry no reply box even for apps that plainly
 * have one, and a later plain notification can arrive with fewer actions than
 * an earlier one. Any of those overwriting a yes would produce a confident
 * wrong answer, which is the single failure this whole probe exists to avoid.
 */
class ProbeLedger {

    private val entries = linkedMapOf<String, LedgerEntry>()

    /**
     * Three tells, any one of which means a real conversation arrived:
     * Android's messaging layout, the message category, or a populated list of
     * messages. WhatsApp's bundled summary on the Pixel used the inbox layout
     * with the message category, so requiring the layout alone would discard
     * real sightings.
     */
    private fun looksLikeMessage(sighting: NotificationSighting): Boolean =
        sighting.category == "msg" ||
            sighting.template?.endsWith("MessagingStyle") == true ||
            sighting.messageCount > 0

    /** True if this notification is worth measuring at all. */
    private fun isMeasurable(sighting: NotificationSighting): Boolean =
        WatchList.label(sighting.packageName) != null || looksLikeMessage(sighting)

    fun record(sighting: NotificationSighting): LedgerEntry? {
        if (!isMeasurable(sighting)) return null

        val finding = ReplyCapability.classify(sighting)
        val previous = entries[sighting.packageName]

        // A group summary can add to the count but can never decide the verdict.
        val summaryIgnored = sighting.isGroupSummary && !finding.canReply

        // Neither can a follow, a like or a story. Instagram posts those from
        // the same app as DMs, and none of them say anything about whether a DM
        // carries a reply box. A yes still counts wherever it turns up — only a
        // no has to be earned by a real message.
        val nonMessageIgnored = !summaryIgnored && !looksLikeMessage(sighting) && !finding.canReply

        val verdict = when {
            previous?.verdict == ProbeVerdict.CAN_REPLY -> ProbeVerdict.CAN_REPLY
            finding.canReply -> ProbeVerdict.CAN_REPLY
            summaryIgnored || nonMessageIgnored -> previous?.verdict ?: ProbeVerdict.NOT_MEASURED
            else -> ProbeVerdict.NO_REPLY_BOX
        }

        val keepReplyDetail = verdict == ProbeVerdict.CAN_REPLY && !finding.canReply

        val updated = LedgerEntry(
            packageName = sighting.packageName,
            label = WatchList.label(sighting.packageName),
            verdict = verdict,
            sightings = (previous?.sightings ?: 0) + 1,
            groupSummariesIgnored = (previous?.groupSummariesIgnored ?: 0) + if (summaryIgnored) 1 else 0,
            nonMessageAlertsIgnored = (previous?.nonMessageAlertsIgnored ?: 0) + if (nonMessageIgnored) 1 else 0,
            source = if (keepReplyDetail) previous?.source ?: finding.source else finding.source,
            replyLabel = if (keepReplyDetail) previous?.replyLabel else finding.replyLabel,
            remoteInputKey = if (keepReplyDetail) previous?.remoteInputKey else finding.remoteInputKey,
            cannedOnlyActionCount = maxOf(previous?.cannedOnlyActionCount ?: 0, finding.cannedOnlyActionCount),
            lastTemplate = finding.template,
            lastCategory = finding.category,
        )

        entries[sighting.packageName] = updated
        return updated
    }

    fun entry(packageName: String): LedgerEntry? = entries[packageName]

    fun verdict(packageName: String): ProbeVerdict =
        entries[packageName]?.verdict ?: ProbeVerdict.NOT_MEASURED

    /**
     * Every watched app, seen or not, plus anything else that turned up.
     * Apps never seen are reported as [ProbeVerdict.NOT_MEASURED] rather than
     * omitted, because a missing row reads as a no.
     */
    fun report(watched: Collection<String>): List<LedgerEntry> {
        val known = entries.values.toList()
        val unseen = watched
            .filterNot { entries.containsKey(it) }
            .map { pkg ->
                LedgerEntry(
                    packageName = pkg,
                    label = WatchList.label(pkg),
                    verdict = ProbeVerdict.NOT_MEASURED,
                    sightings = 0,
                    groupSummariesIgnored = 0,
                    nonMessageAlertsIgnored = 0,
                    source = ReplySource.NONE,
                    replyLabel = null,
                    remoteInputKey = null,
                    cannedOnlyActionCount = 0,
                    lastTemplate = null,
                    lastCategory = null,
                )
            }
        return known + unseen
    }
}
