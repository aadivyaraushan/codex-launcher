package app.codexlauncher.task.control

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.speech.RecognizerIntent
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class PromptDictationContractTest {
    private val contract = PromptDictationContract()

    @Test
    fun contractRequestsFreeFormSpeechAndReturnsTheFirstUsefulResult() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val request = contract.createIntent(context, Unit)

        assertEquals(RecognizerIntent.ACTION_RECOGNIZE_SPEECH, request.action)
        assertEquals(
            RecognizerIntent.LANGUAGE_MODEL_FREE_FORM,
            request.getStringExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL),
        )
        assertEquals("Speak your follow-up", request.getStringExtra(RecognizerIntent.EXTRA_PROMPT))

        val response =
            Intent().putStringArrayListExtra(
                RecognizerIntent.EXTRA_RESULTS,
                arrayListOf("  recognized words  ", "second choice"),
            )
        assertEquals(
            PromptDictationResult.Recognized("recognized words"),
            contract.parseResult(Activity.RESULT_OK, response),
        )
    }

    @Test
    fun contractKeepsCancelAndEmptyRecognitionDistinct() {
        assertTrue(contract.parseResult(Activity.RESULT_CANCELED, null) is PromptDictationResult.Cancelled)
        assertTrue(contract.parseResult(Activity.RESULT_OK, Intent()) is PromptDictationResult.Failed)
    }
}
