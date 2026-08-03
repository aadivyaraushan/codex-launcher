package app.codexlauncher.capability.reply.guard

/**
 * Where the stop list lives between runs. [ReplyGuard] only ever holds the
 * list in memory, so something outside it has to read a saved copy back in
 * on startup and keep it up to date as the user stops and resumes
 * conversations. This is the seam [DurableStops] writes through; the real
 * implementation is a Preferences DataStore, shaped after
 * `storage/projects/ProjectSelectionStore.kt`.
 */
interface StopStore {
    fun read(): List<ThreadKey>

    fun write(keys: List<ThreadKey>)
}

/**
 * What [DurableStops.restore] found when it tried to bring the saved stop
 * list back into the guard.
 */
enum class RestoreOutcome {
    /** The saved list was read without trouble and every entry in it is now in the guard. */
    RESTORED,

    /**
     * The saved list could not be read, so whether anything was saved is
     * genuinely unknown. This is not the same as "there were no stops" —
     * treating a read failure as an empty list would silently let Operator
     * start replying again in a conversation the user meant to keep shut.
     */
    UNKNOWN,
}

/** What [DurableStops.stop] or [DurableStops.resume] found when saving the change. */
enum class SaveOutcome {
    /** The change was written and will still be there after a restart. */
    SAVED,

    /**
     * The write did not succeed, so whether the change is saved is genuinely
     * unknown. The guard still reflects the change for the rest of this run —
     * only the durable copy is in question.
     */
    UNKNOWN,
}
