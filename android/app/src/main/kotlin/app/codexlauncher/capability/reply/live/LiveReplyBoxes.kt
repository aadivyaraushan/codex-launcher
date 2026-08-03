package app.codexlauncher.capability.reply.live

import app.codexlauncher.capability.reply.LiveReplyActions
import app.codexlauncher.capability.reply.ReplyHandle
import app.codexlauncher.capability.reply.request.ReplyHandleSource

/**
 * Turns "the Mac said maya" into the reply box (or boxes) this phone
 * currently has for her.
 *
 * This does not reimplement eviction, the cap, or the locking —
 * [LiveReplyActions] already owns all three and gets them right once. This
 * only adds the one thing that store does not have: a person's name sitting
 * next to the handle, and a way to search by it. See the header comment on
 * `LiveReplyBoxesTest` for why the name is allowed to live here and nowhere
 * else in the probe.
 */
class LiveReplyBoxes(limit: Int) : ReplyHandleSource {

    private val boxes = LiveReplyActions<Pair<String, ReplyHandle>>(limit)

    fun remember(conversationKey: String, person: String, handle: ReplyHandle) {
        boxes.remember(conversationKey, person to handle)
    }

    fun forget(conversationKey: String) {
        boxes.forget(conversationKey)
    }

    fun clear() {
        boxes.clear()
    }

    /**
     * Every box whose person matches [handle] — trimmed, case-insensitive,
     * and whole-word only. "May" must never find "Maya"; a substring match
     * would reply to the wrong person the moment two names happen to
     * overlap. A blank query matches nothing, since a name that never
     * arrived is a bug upstream, not an invitation to guess.
     *
     * Every match comes back, including ones in different conversations —
     * choosing between them is `ReplyAdapter.pick`'s job, not this one's.
     */
    override fun candidatesFor(handle: String): List<ReplyHandle> {
        val query = handle.trim()
        if (query.isEmpty()) return emptyList()
        val queryLower = query.lowercase()

        return boxes.snapshot().values
            .filter { (person, _) -> person.matches(queryLower) }
            .map { (_, replyHandle) -> replyHandle }
    }

    private fun String.matches(queryLower: String): Boolean {
        val personLower = trim().lowercase()
        if (personLower.isEmpty()) return false
        return personLower == queryLower || personLower.split(WHITESPACE).contains(queryLower)
    }

    private companion object {
        val WHITESPACE = Regex("\\s+")
    }
}
