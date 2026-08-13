package app.codexlauncher.task.control.dictation

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class DeepgramApiKeySourceTest {
    @get:Rule
    val folder = TemporaryFolder()

    @Test
    fun debugPrivateFileWinsOverCompiledKey() {
        val file = folder.newFile("deepgram_api_key")
        file.writeText("  file-key-not-for-prod  \n")
        val source =
            DeepgramApiKeySource(
                debugBuild = true,
                compiledKey = "compiled-key-not-for-prod",
                privateKeyFile = file,
            )

        assertEquals("file-key-not-for-prod", source.read())
    }

    @Test
    fun debugFallsBackToCompiledKeyWhenFileMissing() {
        val source =
            DeepgramApiKeySource(
                debugBuild = true,
                compiledKey = " compiled-key-not-for-prod ",
                privateKeyFile = folder.root.resolve("missing"),
            )

        assertEquals("compiled-key-not-for-prod", source.read())
    }

    @Test
    fun releaseIgnoresPrivateFileAndCompiledKey() {
        val file = folder.newFile("deepgram_api_key")
        file.writeText("file-key-not-for-prod")
        val source =
            DeepgramApiKeySource(
                debugBuild = false,
                compiledKey = "compiled-key-not-for-prod",
                privateKeyFile = file,
            )

        assertNull(source.read())
    }

    @Test
    fun blankSourcesAreAbsent() {
        val file = folder.newFile("deepgram_api_key")
        file.writeText("  \n")
        val source =
            DeepgramApiKeySource(
                debugBuild = true,
                compiledKey = "   ",
                privateKeyFile = file,
            )

        assertNull(source.read())
    }
}
