package app.codexlauncher.capability.handoff

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Instagram hand-off is a plain "open the app" launch, and the documentation
 * says that is the only thing we can do for free. This test fires the day
 * someone reaches for more.
 *
 * What Meta actually publishes (read from their pages on 2026-08-03, written up
 * in `saved-results/how-precisely-android-can-open-instagram.md`):
 *
 *  - There is **no documented way to open a specific person's DM thread**. The
 *    one link that exists, `https://ig.me/m/<username>`, is published inside the
 *    Instagram Messaging API docs — the product for businesses running a bot —
 *    and needs the account connected to an app and published.
 *  - There is **no documented profile deep link**. `instagram://user?username=`
 *    appears in no Meta documentation. It works today by observation and Meta
 *    owes it nothing.
 *  - Sharing into **Stories** is documented, but Meta's page says: "Beginning in
 *    January 2023, you must provide a Facebook AppID to share content to
 *    Instagram Stories." Without one the user is shown "The app you shared from
 *    doesn't currently support sharing to Stories." So adding that intent
 *    without first registering a Facebook app ships a dead button.
 *  - Sharing into **Feed** is documented as a plain Android share sheet and
 *    needs no App ID — that one is fine to add, and it is deliberately not
 *    caught below.
 *
 * So the two things below are not style rules. Each one, added on its own,
 * produces a user-visible failure that no other test would catch: a deep link
 * that silently degrades to a web page, or a share button that shows an error.
 *
 * If you are adding one of these on purpose, the remedy is in the failure
 * message, not here.
 */
class InstagramHandOffStaysGenericTest {

    private val mainSources: List<File> =
        File("src/main/kotlin").walkTopDown().filter { it.isFile && it.extension == "kt" }.toList()

    private fun linesMentioning(needle: String): List<String> =
        mainSources.flatMap { file ->
            file.readLines()
                .withIndex()
                .filter { (_, line) -> line.contains(needle) && !line.trimStart().startsWith("//") }
                .map { (index, line) -> "${file.path}:${index + 1}: ${line.trim()}" }
        }

    @Test
    fun `the test can see the app sources at all`() {
        // Guards the guard: a wrong working directory would make every check
        // below pass by finding nothing.
        assertTrue(
            "expected to find Kotlin sources under src/main/kotlin, found ${mainSources.size}",
            mainSources.size > 50,
        )
        assertTrue(
            "expected HandOffActions.kt among the scanned sources",
            mainSources.any { it.name == "HandOffActions.kt" },
        )
    }

    @Test
    fun `nothing builds an instagram deep link`() {
        val hits = linesMentioning("instagram://") + linesMentioning("ig.me")
        assertEquals(
            "an Instagram deep link was added:\n${hits.joinToString("\n")}\n\n" +
                "Neither form is a documented promise. `instagram://user?username=` is in no " +
                "Meta documentation at all, and `ig.me/m/<user>` is documented only inside the " +
                "Instagram Messaging API, for business accounts connected to a bot. Both will " +
                "look like they work in testing and then quietly stop, or land somewhere other " +
                "than the thread the user expected.\n" +
                "If you have new evidence that one of them is supported, cite the Meta page in " +
                "saved-results/how-precisely-android-can-open-instagram.md and delete this test.",
            emptyList<String>(),
            hits,
        )
    }

    @Test
    fun `nothing shares to instagram stories without a facebook app id`() {
        val storyIntents = linesMentioning("com.instagram.share.ADD_TO_STORY") +
            linesMentioning("com.instagram.share.ADD_TO_REEL")
        if (storyIntents.isEmpty()) return

        val declaresAppId = mainSources.any { file ->
            file.readLines().any { line ->
                line.contains("com.facebook.app.id") || line.contains("source_application")
            }
        }
        assertTrue(
            "sharing to Instagram Stories or Reels was added:\n${storyIntents.joinToString("\n")}\n\n" +
                "but nothing passes a Facebook App ID. Meta's page says: \"Beginning in January " +
                "2023, you must provide a Facebook AppID to share content to Instagram Stories.\" " +
                "Without it the user gets \"The app you shared from doesn't currently support " +
                "sharing to Stories\" — a button that always fails.\n" +
                "Registering a Facebook app is an owner act, not a coding task. Either get the " +
                "App ID and pass it as the `source_application` extra, or use Feed sharing " +
                "(ACTION_SEND + createChooser), which Meta documents with no App ID at all.",
            declaresAppId,
        )
    }

    @Test
    fun `instagram hand-off is still the generic package launch`() {
        val handOff = mainSources.single { it.name == "HandOffActions.kt" }.readText()
        assertTrue(
            "HandOffActions no longer opens Instagram with getLaunchIntentForPackage. " +
                "That plain launch is exactly what Meta's documentation supports with no " +
                "registration, and the plan's contract is \"open Instagram, you choose the " +
                "thread\". If the launch path changed, re-read " +
                "saved-results/how-precisely-android-can-open-instagram.md first.",
            handOff.contains("getLaunchIntentForPackage"),
        )
        assertTrue(
            "the instagram -> com.instagram.android mapping is gone from HandOffActions",
            handOff.contains("\"instagram\" -> \"com.instagram.android\""),
        )
    }
}
