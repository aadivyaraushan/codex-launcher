package app.codexlauncher.runtime.actionjournal

/**
 * In-memory send journal matching the Beeper send contract. Room encryption
 * wraps this later; domain rules live here.
 */
class ActionJournal {
    enum class State {
        Confirmed,
        Dispatching,
        Submitted,
        DeliveryUnknown,
        Observed,
    }

    class HashMismatch : IllegalStateException("action journal: same id with different payload hash")

    data class Record(
        val actionId: String,
        val payloadHash: String,
        val accountId: String,
        val chatId: String,
        val state: State,
        val pendingMessageId: String = "",
        val shouldResend: Boolean = false,
    )

    private val records = linkedMapOf<String, Record>()

    fun confirm(actionId: String, payloadHash: String, accountId: String, chatId: String): Record {
        val existing = records[actionId]
        if (existing != null) {
            if (existing.payloadHash != payloadHash) throw HashMismatch()
            return existing
        }
        val rec =
            Record(
                actionId = actionId,
                payloadHash = payloadHash,
                accountId = accountId,
                chatId = chatId,
                state = State.Confirmed,
            )
        records[actionId] = rec
        return rec
    }

    fun markDispatching(actionId: String, payloadHash: String): Record {
        val existing = records[actionId] ?: error("action not found")
        if (existing.payloadHash != payloadHash) throw HashMismatch()
        val rec = existing.copy(state = State.Dispatching)
        records[actionId] = rec
        return rec
    }

    fun markSubmitted(actionId: String, payloadHash: String, pendingMessageId: String): Record {
        val existing = records[actionId] ?: error("action not found")
        if (existing.payloadHash != payloadHash) throw HashMismatch()
        val rec =
            existing.copy(
                state = State.Submitted,
                pendingMessageId = pendingMessageId,
                shouldResend = false,
            )
        records[actionId] = rec
        return rec
    }

    fun crashRecover(actionId: String): Record {
        val existing = records[actionId] ?: error("action not found")
        val rec =
            when (existing.state) {
                State.Dispatching ->
                    if (existing.pendingMessageId.isEmpty()) {
                        existing.copy(state = State.DeliveryUnknown, shouldResend = false)
                    } else {
                        existing.copy(shouldResend = false)
                    }
                State.Submitted -> existing.copy(shouldResend = false)
                else -> existing
            }
        records[actionId] = rec
        return rec
    }
}
