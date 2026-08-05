package app.codexlauncher.runtime.localpair

/**
 * Operator-owned latch: local-pair imports are accepted only while the
 * user-opened "Link local runtime" screen is waiting.
 */
object LocalPairAwaitingStore {
    @Volatile
    var waitingForOffer: Boolean = false

    private val seen = linkedSetOf<String>()

    val seenOfferIds: Set<String>
        get() = synchronized(seen) { seen.toSet() }

    fun beginWaiting() {
        waitingForOffer = true
    }

    fun clearWaiting() {
        waitingForOffer = false
    }

    fun markSeen(offerId: String) {
        synchronized(seen) { seen.add(offerId) }
    }
}
