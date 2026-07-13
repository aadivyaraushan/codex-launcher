package app.codexlauncher

import android.app.Application
import app.codexlauncher.storage.ownership.LocalStateOwner

class LauncherApplication : Application() {
    val localState: LocalStateOwner by lazy { LocalStateOwner(this) }
}
