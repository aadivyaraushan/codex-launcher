package app.codexlauncher.capability.reply.guard

import app.codexlauncher.diagnostics.AppLog

/**
 * Makes a stop survive the process dying, and makes it possible to undo one.
 *
 * Holds both the [guard] that actually decides whether a reply goes through
 * and the [store] that remembers what the user asked for across a restart.
 * See the file header on `DurableStopsTest` for why persistence lives here
 * rather than inside [ReplyGuard] itself.
 */
class DurableStops(private val guard: ReplyGuard, private val store: StopStore) {

    /**
     * Brings back whatever was saved last time the app ran. Only ever adds
     * stops to the guard — it is safe to call again (Android can bring the
     * app back to the front and trigger this a second time) without undoing
     * a stop made since the last call.
     */
    fun restore(): RestoreOutcome =
        try {
            val saved = store.read()
            saved.forEach(guard::stop)
            RestoreOutcome.RESTORED
        } catch (error: Exception) {
            AppLog.error(
                feature = "reply-stops",
                message = "stop list restore failed",
                error = error,
                fields = mapOf("decision" to "starting_with_no_restored_stops"),
            )
            RestoreOutcome.UNKNOWN
        }

    /**
     * Stops [key] in the guard first, so it holds for the rest of this run no
     * matter what happens next, then writes the guard's full stop list down.
     */
    fun stop(key: ThreadKey): SaveOutcome {
        guard.stop(key)
        return persist(op = "stop", key = key)
    }

    /** Resumes [key] in the guard first, then writes the change down. */
    fun resume(key: ThreadKey): SaveOutcome {
        guard.resume(key)
        return persist(op = "resume", key = key)
    }

    /**
     * What a screen should list as currently stopped. Reads the guard, not
     * the store, because the guard is what actually refuses replies.
     */
    fun stopped(): Set<ThreadKey> = guard.stopped()

    private fun persist(op: String, key: ThreadKey): SaveOutcome {
        val current = guard.stopped()
        return try {
            store.write(current.toList())
            SaveOutcome.SAVED
        } catch (error: Exception) {
            // Package and a count only -- never the person's name -- matching
            // ReplyGuard.logRefusal's own rule for what may reach a log line.
            AppLog.error(
                feature = "reply-stops",
                message = "stop list write failed",
                error = error,
                fields = mapOf(
                    "package" to key.packageName,
                    "operation" to op,
                    "stopped_count" to current.size,
                    "decision" to "kept_in_memory_only",
                ),
            )
            SaveOutcome.UNKNOWN
        }
    }
}
