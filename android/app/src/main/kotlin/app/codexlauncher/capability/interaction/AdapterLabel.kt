package app.codexlauncher.capability.interaction

/**
 * The words to show a person for an adapter id, on the sheet they tap to
 * approve an app action. See `AdapterLabelTest` for why this list is here and
 * not on the wire.
 *
 * Keyed by the id in its tidied-down form — lowercase, trimmed — so an id that
 * arrives with odd spacing or capitals still finds its name.
 */
private val writtenDownNames = mapOf(
    "gcalendar" to "Google Calendar",
    "gdrive" to "Google Drive",
    "msteams" to "Microsoft Teams",
    "maps" to "Google Maps",
    "maps_saved_places" to "Saved places in Google Maps",

    // Not an app name, because there is none to give. The Mac hands a reply to
    // this phone without knowing which app it lands in — the phone chooses that
    // after this sheet is confirmed. Naming a guess here would be the one thing
    // a consent sheet must never do.
    "notification_reply" to "This phone",
)

private val SEPARATORS = Regex("[-_\\s]+")

/**
 * Turns [adapterId] into something worth showing. A blank id gives back a
 * blank string, so the caller can leave the line out rather than print a
 * separator with nothing before it.
 */
fun adapterLabel(adapterId: String): String {
    val id = adapterId.trim().lowercase()
    if (id.isEmpty()) return ""
    writtenDownNames[id]?.let { return it }
    return id.split(SEPARATORS)
        .filter { it.isNotEmpty() }
        .joinToString(" ") { word -> word.replaceFirstChar(Char::uppercase) }
}
