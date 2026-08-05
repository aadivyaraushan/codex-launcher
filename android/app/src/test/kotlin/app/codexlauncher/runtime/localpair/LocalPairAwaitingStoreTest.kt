package app.codexlauncher.runtime.localpair

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class LocalPairAwaitingStoreTest {
    @Before
    fun reset() {
        LocalPairAwaitingStore.waitingForOffer = false
    }

    @Test
    fun beginWaitingEnablesImportLatch() {
        assertFalse(LocalPairAwaitingStore.waitingForOffer)
        LocalPairAwaitingStore.beginWaiting()
        assertTrue(LocalPairAwaitingStore.waitingForOffer)
    }

    @Test
    fun clearWaitingDisablesImportLatch() {
        LocalPairAwaitingStore.beginWaiting()
        LocalPairAwaitingStore.clearWaiting()
        assertFalse(LocalPairAwaitingStore.waitingForOffer)
    }
}
