package app.codexlauncher.capability.reply.request

import app.codexlauncher.capability.reply.ReplyHandle

/**
 * Looks up whatever reply boxes this phone currently has for the person the
 * Mac named.
 *
 * The Mac only ever knows a *person* — "maya" — never a conversation.
 * Android's per-notification key is made fresh each time the shade is built
 * and thrown away when it changes; it has never crossed the wire and never
 * will. So turning a person's name into the live notification (or
 * notifications) that belong to them is work the phone has to do on its own,
 * with whatever it can currently see in the shade — and declining when it
 * cannot tell is exactly the point, not a gap to close later.
 */
fun interface ReplyHandleSource {
    fun candidatesFor(handle: String): List<ReplyHandle>
}
