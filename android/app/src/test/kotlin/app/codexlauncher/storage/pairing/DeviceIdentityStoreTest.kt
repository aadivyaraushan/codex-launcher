package app.codexlauncher.storage.pairing

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.preferencesOf
import androidx.datastore.preferences.core.stringPreferencesKey
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class DeviceIdentityStoreTest {
    @Test
    fun createsOneSafeAppPrivateIdentifierAndReusesIt() = runBlocking {
        val dataStore = FakeDeviceIdentityDataStore()
        var generated = 0
        val store =
            DeviceIdentityStore(dataStore, NoOpDeviceIdentityReporter) {
                generated += 1
                "android-generated-$generated"
            }

        val first = store.loadOrCreate()
        val second = store.loadOrCreate()

        assertEquals("android-generated-1", first)
        assertEquals(first, second)
        assertEquals(1, generated)
        assertEquals(first, dataStore.current()[stringPreferencesKey("device_id")])
    }

    @Test
    fun replacesUnsafeStoredValuesAndFailsWhenStorageCannotBeRead() = runBlocking {
        val unsafe = preferencesOf(stringPreferencesKey("device_id") to "/private/device/path")
        val repaired = DeviceIdentityStore(FakeDeviceIdentityDataStore(unsafe), NoOpDeviceIdentityReporter) { "android-repaired" }
        assertEquals("android-repaired", repaired.loadOrCreate())

        val unavailable =
            DeviceIdentityStore(
                FakeDeviceIdentityDataStore(readFailure = IOException("disk unavailable")),
                NoOpDeviceIdentityReporter,
            ) { "android-never-used" }
        val error = assertThrows(DeviceIdentityException::class.java) { runBlocking { unavailable.loadOrCreate() } }
        assertTrue(error.cause is IOException)
    }
}

private object NoOpDeviceIdentityReporter : DeviceIdentityReporter {
    override fun generated() = Unit

    override fun invalidStoredValue() = Unit

    override fun failed(error: IOException) = Unit
}

private class FakeDeviceIdentityDataStore(
    initial: Preferences = emptyPreferences(),
    readFailure: Throwable? = null,
) : DataStore<Preferences> {
    private val state = MutableStateFlow(initial)

    override val data: Flow<Preferences> =
        if (readFailure == null) {
            state
        } else {
            flow { throw readFailure }
        }

    override suspend fun updateData(transform: suspend (Preferences) -> Preferences): Preferences {
        val updated = transform(state.value)
        state.value = updated
        return updated
    }

    fun current(): Preferences = state.value
}
