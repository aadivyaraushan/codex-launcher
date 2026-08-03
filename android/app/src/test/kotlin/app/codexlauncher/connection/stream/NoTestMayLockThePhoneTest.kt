package app.codexlauncher.connection.stream

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * No instrumented test may put the phone's screen to sleep, because on a phone
 * with a real lock nothing can undo it and every test that runs afterwards
 * fails.
 *
 * This is not a style rule. It was measured on the Pixel 9 on 2026-08-03 and it
 * is the reason the connected suite has never been green:
 *
 *  - `CodexConnectionServiceTest` sent `input keyevent KEYCODE_SLEEP`. The
 *    screen went off, and on a secured phone the keyguard comes up with it.
 *  - Its cleanup tried three commands, and the one that mattered does not work:
 *    `KEYCODE_WAKEUP` turns the screen back **on but leaves it locked**, and
 *    `wm dismiss-keyguard` has no effect on a secure keyguard — already measured
 *    in this repo and written into the operator plan's Pixel row.
 *  - So from that moment the keyguard sat on top of every activity, and each
 *    later Compose test failed with "No compose hierarchies found in the app...
 *    (1) the Activity that calls setContent did not launch".
 *
 * The damage is large and varies with test order, which is what made it read as
 * flakiness for so long. Two full runs on an unlocked phone: **29 failures**,
 * then **51 failures**, with a different set of classes each time. The same
 * three classes run on their own passed **28 of 28**.
 *
 * A test that cannot restore what it changed is not isolated, and this one
 * cannot. Screen state is device-wide and outlives the test that changed it.
 *
 * **Doze does not need the screen off.** The supported way to test idle
 * behaviour is to make the device think it is on battery — the reason
 * `force-idle` was timing out here is that the phone is plugged in over USB, and
 * Android will not enter deep idle while charging. `dumpsys battery unplug`
 * fixes that without touching the screen, and `dumpsys battery reset` undoes it.
 *
 * See `saved-results/the-pixel-suite-actually-runs.md`.
 */
class NoTestMayLockThePhoneTest {

    private val instrumentedSources: List<File> =
        File("src/androidTest/kotlin").walkTopDown()
            .filter { it.isFile && it.extension == "kt" }
            .toList()

    private fun offendingLines(vararg needles: String): List<String> =
        instrumentedSources.flatMap { file ->
            file.readLines()
                .withIndex()
                .filter { (_, line) ->
                    !line.trimStart().startsWith("//") &&
                        !line.trimStart().startsWith("*") &&
                        needles.any { line.contains(it) }
                }
                .map { (index, line) -> "${file.path}:${index + 1}: ${line.trim()}" }
        }

    @Test
    fun `the scan can see the instrumented sources at all`() {
        // Guards the guard: a wrong working directory would make the checks
        // below pass by finding nothing at all.
        assertTrue(
            "expected Kotlin sources under src/androidTest/kotlin, found ${instrumentedSources.size}",
            instrumentedSources.size > 10,
        )
        assertTrue(
            "expected CodexConnectionServiceTest.kt among the scanned sources",
            instrumentedSources.any { it.name == "CodexConnectionServiceTest.kt" },
        )
    }

    @Test
    fun `no instrumented test puts the screen to sleep`() {
        val hits = offendingLines("KEYCODE_SLEEP", "input keyevent 223")
        assertEquals(
            "an instrumented test turns the phone's screen off:\n${hits.joinToString("\n")}\n\n" +
                "On a phone with a real lock this raises the keyguard, and nothing available to a " +
                "test can lower it again — KEYCODE_WAKEUP only turns the screen back on, and " +
                "`wm dismiss-keyguard` does nothing to a secure keyguard (measured on this Pixel). " +
                "Every Compose test that runs afterwards then fails with \"No compose hierarchies " +
                "found in the app\", and which ones fail depends on test order, so it reads as " +
                "flakiness rather than as this.\n" +
                "To test doze, do not touch the screen: `dumpsys battery unplug` then " +
                "`cmd deviceidle force-idle`, and undo with `cmd deviceidle unforce` and " +
                "`dumpsys battery reset`. The phone is plugged in over USB, and Android will not " +
                "enter deep idle while charging — which is also why the old test timed out.",
            emptyList<String>(),
            hits,
        )
    }

    @Test
    fun `no instrumented test relies on dismiss-keyguard to clean up`() {
        val hits = offendingLines("dismiss-keyguard")
        assertEquals(
            "an instrumented test calls `wm dismiss-keyguard`:\n${hits.joinToString("\n")}\n\n" +
                "It does nothing on a secure keyguard — measured on this Pixel, and recorded in " +
                "the operator plan's Pixel row. A cleanup step that cannot work is worse than no " +
                "cleanup step, because it makes the test look like it restores what it changed. " +
                "Do not lock the phone in the first place.",
            emptyList<String>(),
            hits,
        )
    }
}
