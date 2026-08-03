package app.codexlauncher.capability.reply

/**
 * The store that closes RT-4: a place to keep a live reply action for exactly
 * as long as its notification is on screen, and no longer.
 *
 * A reply action carries a PendingIntent, which is permission to act as the
 * app that sent it. Holding on to one after its notification is gone means
 * keeping a capability the user can no longer see and never agreed to leave
 * lying around, and using it later would reach a conversation that has moved
 * on. So forgetting an entry the moment its notification withdraws matters
 * more than remembering one in the first place, and there is a hard cap so a
 * busy phone posting notifications nonstop cannot grow this without limit.
 *
 * It is deliberately generic and knows nothing about Android. The real
 * reply action type only exists on a device, and this class needs to run in
 * a plain unit test on a laptop, so it just holds whatever the caller gives
 * it under whatever key the caller chooses.
 *
 * The listener callbacks that fill this and the UI thread reading from it
 * can both run at the same time, so every operation takes a lock.
 */
class LiveReplyActions<T>(private val limit: Int) {

    private val lock = Any()

    // Insertion order doubles as "how recently remembered" here: a
    // re-remember removes the old entry first so the put always lands at
    // the newest end, and the oldest entry to evict is always the first one
    // in the map.
    private val entries = LinkedHashMap<String, T>()

    val size: Int
        get() = synchronized(lock) { entries.size }

    fun remember(conversationKey: String, action: T) {
        synchronized(lock) {
            entries.remove(conversationKey)
            entries[conversationKey] = action

            if (entries.size > limit) {
                val oldestKey = entries.keys.iterator().next()
                entries.remove(oldestKey)
            }
        }
    }

    fun find(conversationKey: String): T? = synchronized(lock) { entries[conversationKey] }

    /**
     * A copy of every entry remembered right now, keyed by conversation, for
     * a caller that needs to scan all of them rather than look one up.
     * Handing back the live map instead would let a caller iterate it while
     * the listener thread mutates it underneath — this takes the copy under
     * the same lock everything else here uses, so it is always a consistent
     * snapshot of one moment.
     */
    fun snapshot(): Map<String, T> = synchronized(lock) { LinkedHashMap(entries) }

    fun forget(conversationKey: String) {
        synchronized(lock) { entries.remove(conversationKey) }
    }

    fun clear() {
        synchronized(lock) { entries.clear() }
    }
}
