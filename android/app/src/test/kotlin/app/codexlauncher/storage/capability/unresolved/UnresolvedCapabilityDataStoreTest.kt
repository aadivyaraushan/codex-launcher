package app.codexlauncher.storage.capability.unresolved

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.stringPreferencesKey
import java.io.IOException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Test

class UnresolvedCapabilityDataStoreTest {
    @Test
    fun saveThenLoadReturnsTheSameMessage() = runBlocking {
        val dataStore = FakeUnresolvedDataStore()
        val store = UnresolvedCapabilityDataStore(dataStore, NoOpUnresolvedCapabilityReporter, scope = CoroutineScope(Dispatchers.Unconfined))

        store.remember("Message Maya — outcome unknown. Check the computer before sending another.")

        assertEquals(
            UnresolvedCapabilityCheck.Pending("Message Maya — outcome unknown. Check the computer before sending another."),
            store.load(),
        )
    }

    @Test
    fun savingNullThenLoadReturnsNone() = runBlocking {
        val dataStore = FakeUnresolvedDataStore()
        val store = UnresolvedCapabilityDataStore(dataStore, NoOpUnresolvedCapabilityReporter, scope = CoroutineScope(Dispatchers.Unconfined))

        store.remember("armed")
        store.remember(null)

        assertEquals(UnresolvedCapabilityCheck.None, store.load())
    }

    @Test
    fun aFreshStoreReturnsNone() = runBlocking {
        val store = UnresolvedCapabilityDataStore(FakeUnresolvedDataStore(), NoOpUnresolvedCapabilityReporter, scope = CoroutineScope(Dispatchers.Unconfined))

        assertEquals(UnresolvedCapabilityCheck.None, store.load())
    }

    @Test
    fun anIoExceptionOnReadReturnsUnreadable() = runBlocking {
        val dataStore = FakeUnresolvedDataStore(readFailure = IOException("unavailable"))
        val store = UnresolvedCapabilityDataStore(dataStore, NoOpUnresolvedCapabilityReporter, scope = CoroutineScope(Dispatchers.Unconfined))

        assertEquals(UnresolvedCapabilityCheck.Unreadable, store.load())
    }
}

private object NoOpUnresolvedCapabilityReporter : UnresolvedCapabilityReporter {
    override fun readFailed(error: IOException) = Unit
    override fun writeCompleted(present: Boolean) = Unit
    override fun writeFailed(error: IOException) = Unit
}

private class FakeUnresolvedDataStore(
    initial: Preferences = emptyPreferences(),
    private val readFailure: Throwable? = null,
) : DataStore<Preferences> {
    private val state = MutableStateFlow(initial)
    override val data: Flow<Preferences> = if (readFailure == null) state else flow { throw readFailure }
    override suspend fun updateData(transform: suspend (Preferences) -> Preferences): Preferences =
        transform(state.value).also { state.value = it }
}
