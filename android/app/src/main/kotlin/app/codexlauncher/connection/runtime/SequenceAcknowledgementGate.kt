package app.codexlauncher.connection.runtime

import java.util.TreeMap

internal data class AcknowledgementRequest(
    val throughSequence: Long,
    val actionIds: Set<String>,
)

internal class SequenceAcknowledgementGate {
    private val blockedActions = TreeMap<Long, String>()
    private val readyActions = TreeMap<Long, String>()
    private var requestedThrough = 0L
    private var sentThrough = 0L

    @Synchronized
    fun block(actionId: String, sequence: Long) {
        require(actionId.isNotBlank() && sequence > sentThrough)
        val previous = blockedActions.putIfAbsent(sequence, actionId)
        require(previous == null || previous == actionId)
        requestedThrough = maxOf(requestedThrough, sequence)
    }

    @Synchronized
    fun request(throughSequence: Long): AcknowledgementRequest? {
        if (throughSequence < 1) return null
        requestedThrough = maxOf(requestedThrough, throughSequence)
        return nextRequest()
    }

    @Synchronized
    fun release(actionId: String, sequence: Long): AcknowledgementRequest? {
        if (blockedActions[sequence] != actionId) return null
        blockedActions.remove(sequence)
        readyActions[sequence] = actionId
        return nextRequest()
    }

    @Synchronized
    fun markSent(throughSequence: Long): Set<String> {
        require(throughSequence > sentThrough && throughSequence <= requestedThrough)
        sentThrough = throughSequence
        val acknowledged =
            readyActions.headMap(throughSequence, true)
                .values
                .toSet()
        readyActions.headMap(throughSequence, true).clear()
        return acknowledged
    }

    @Synchronized
    fun reset() {
        blockedActions.clear()
        readyActions.clear()
        requestedThrough = 0
        sentThrough = 0
    }

    private fun nextRequest(): AcknowledgementRequest? {
        val firstBlocked = blockedActions.firstKeyOrNull()
        val safeThrough =
            if (firstBlocked == null) {
                requestedThrough
            } else {
                minOf(requestedThrough, firstBlocked - 1)
            }
        if (safeThrough <= sentThrough) return null
        return AcknowledgementRequest(
            throughSequence = safeThrough,
            actionIds = readyActions.headMap(safeThrough, true).values.toSet(),
        )
    }

    private fun <K, V> TreeMap<K, V>.firstKeyOrNull(): K? = if (isEmpty()) null else firstKey()
}
