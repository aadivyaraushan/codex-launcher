package app.codexlauncher.storage.drafts

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import app.codexlauncher.storage.secrets.PAIRING_KEY_ALIAS
import app.codexlauncher.task.composer.DraftComposerViewModel
import java.io.File
import java.io.RandomAccessFile
import java.nio.ByteBuffer
import java.security.KeyStore
import java.time.Duration
import java.time.Instant
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.yield
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class EncryptedDraftStoreTest {
    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val draftFile = File(context.noBackupFilesDir, "test-drafts/unfinished.bin")
    private val keyStore = DraftKeyStore()
    private val now = Instant.parse("2026-07-13T18:00:00Z")

    @Before
    fun clearBeforeTest() {
        draftFile.parentFile?.deleteRecursively()
        keyStore.delete()
    }

    @After
    fun clearAfterTest() {
        draftFile.parentFile?.deleteRecursively()
        keyStore.delete()
    }

    @Test
    fun savesOnlyCiphertextAndRestoresAfterStoreRecreation() = runBlocking {
        val privateDraft = "Fix the private checkout flow"
        val first = store()
        assertTrue(first.save(privateDraft, now))

        val raw = draftFile.readBytes()
        assertFalse(raw.toString(Charsets.UTF_8).contains(privateDraft))
        assertEquals(DraftReadState.Available(privateDraft, now), store().load(now.plusSeconds(1)))

        val androidKeyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        val secretKey = androidKeyStore.getKey(DRAFT_KEY_ALIAS, null)
        assertNull("Draft AES key material must not be exportable", secretKey.encoded)
        assertTrue(androidKeyStore.containsAlias(DRAFT_KEY_ALIAS))
        assertFalse("Drafts must never reuse the pairing key", DRAFT_KEY_ALIAS == PAIRING_KEY_ALIAS)
    }

    @Test
    fun composerEditsRoundTripThroughRealAndroidEncryption() = runBlocking {
        val firstStore = store()
        val firstComposer =
            DraftComposerViewModel(
                loadDraft = firstStore::load,
                saveDraft = firstStore::save,
                storageDispatcher = Dispatchers.Unconfined,
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        firstComposer.load()
        firstComposer.update("unfinished encrypted composer text")
        yield()

        val restoredStore = store()
        val restoredComposer =
            DraftComposerViewModel(
                loadDraft = restoredStore::load,
                saveDraft = restoredStore::save,
                storageDispatcher = Dispatchers.Unconfined,
                workScope = CoroutineScope(Dispatchers.Unconfined),
            )
        restoredComposer.load()

        assertEquals("unfinished encrypted composer text", restoredComposer.state.value.text)
        assertFalse(draftFile.readText().contains("unfinished encrypted composer text"))
    }

    @Test
    fun emptyDraftDeletesCiphertextAndExpiredDraftIsEvicted() = runBlocking {
        val store = store(maxAge = Duration.ofHours(2))
        assertTrue(store.save("unfinished", now))
        assertTrue(store.save("   ", now.plusSeconds(1)))
        assertEquals(DraftReadState.Empty, store.load(now.plusSeconds(2)))
        assertFalse(draftFile.exists())

        assertTrue(store.save("expired", now))
        assertEquals(DraftReadState.Empty, store.load(now.plus(Duration.ofHours(3))))
        assertFalse(draftFile.exists())
    }

    @Test
    fun oversizedCorruptAndFutureRecordsFailClosedWithoutReturningContent() = runBlocking {
        val store = store()
        assertFalse(store.save("x".repeat(MAX_DRAFT_BYTES + 1), now))
        assertEquals(DraftReadState.Empty, store.load(now))

        draftFile.parentFile?.mkdirs()
        draftFile.writeBytes("private corrupt plaintext".encodeToByteArray())
        assertEquals(DraftReadState.Unavailable(DraftReadFailure.INVALID_DATA), store.load(now))

        assertTrue(store.save("future", now.plusSeconds(60)))
        assertEquals(DraftReadState.Unavailable(DraftReadFailure.INVALID_DATA), store.load(now))
    }

    @Test
    fun failedAtomicReplacementKeepsTheLastCommittedDraft() = runBlocking {
        val stable = store()
        assertTrue(stable.save("first committed draft", now))

        val failing =
            EncryptedDraftStore(
                file = draftFile,
                keys = keyStore,
                now = { now.plusSeconds(1) },
                maxAge = Duration.ofDays(7),
                commit = { _, _ -> throw IllegalStateException("simulated disk failure") },
            )
        assertFalse(failing.save("replacement that must not appear", now.plusSeconds(1)))
        assertEquals(DraftReadState.Available("first committed draft", now), stable.load(now.plusSeconds(2)))
    }

    @Test
    fun tamperedExpiredTimestampIsRejectedWithoutDeletingAuthenticatedCiphertext() = runBlocking {
        val store = store(maxAge = Duration.ofHours(2))
        assertTrue(store.save("must survive unauthenticated expiry", now))
        val tampered = draftFile.readBytes()
        ByteBuffer.wrap(tampered).putLong(5, now.minus(Duration.ofDays(30)).toEpochMilli())
        draftFile.writeBytes(tampered)

        assertEquals(DraftReadState.Unavailable(DraftReadFailure.INVALID_DATA), store.load(now))
        assertTrue("Unauthenticated metadata must not authorize deletion", draftFile.exists())
    }

    @Test
    fun oversizedStoredRecordIsRejectedBeforeItsBodyIsRead() = runBlocking {
        draftFile.parentFile?.mkdirs()
        RandomAccessFile(draftFile, "rw").use { it.setLength((MAX_DRAFT_BYTES + 2048).toLong()) }
        var bodyReads = 0
        val store =
            EncryptedDraftStore(
                file = draftFile,
                keys = keyStore,
                now = { now },
                maxAge = Duration.ofDays(7),
                read = {
                    bodyReads += 1
                    it.readBytes()
                },
            )

        assertEquals(DraftReadState.Unavailable(DraftReadFailure.INVALID_DATA), store.load(now))
        assertEquals(0, bodyReads)
    }

    @Test
    fun separateStoreInstancesSerializeOperationsForTheSameFile() = runBlocking {
        val firstCommitEntered = CountDownLatch(1)
        val releaseFirstCommit = CountDownLatch(1)
        val activeCommits = AtomicInteger()
        val maximumActiveCommits = AtomicInteger()
        val commit: (File, ByteArray) -> Unit = { target, bytes ->
            val active = activeCommits.incrementAndGet()
            maximumActiveCommits.updateAndGet { current -> maxOf(current, active) }
            if (firstCommitEntered.count == 1L) {
                firstCommitEntered.countDown()
                releaseFirstCommit.await(2, TimeUnit.SECONDS)
            }
            target.parentFile?.mkdirs()
            target.writeBytes(bytes)
            activeCommits.decrementAndGet()
        }
        val first = EncryptedDraftStore(draftFile, keyStore, { now }, Duration.ofDays(7), commit)
        val second = EncryptedDraftStore(draftFile, keyStore, { now }, Duration.ofDays(7), commit)
        val executor = Executors.newFixedThreadPool(2)
        try {
            val firstSave = executor.submit<Boolean> { runBlocking { first.save("first", now) } }
            assertTrue(firstCommitEntered.await(2, TimeUnit.SECONDS))
            val secondSave = executor.submit<Boolean> { runBlocking { second.save("second", now.plusSeconds(1)) } }
            Thread.sleep(100)
            releaseFirstCommit.countDown()
            assertTrue(firstSave.get(2, TimeUnit.SECONDS))
            assertTrue(secondSave.get(2, TimeUnit.SECONDS))
        } finally {
            releaseFirstCommit.countDown()
            executor.shutdownNow()
        }
        assertEquals(1, maximumActiveCommits.get())
        assertEquals(DraftReadState.Available("second", now.plusSeconds(1)), store().load(now.plusSeconds(2)))
    }

    private fun store(maxAge: Duration = Duration.ofDays(7)) =
        EncryptedDraftStore(
            file = draftFile,
            keys = keyStore,
            now = { now },
            maxAge = maxAge,
        )
}
