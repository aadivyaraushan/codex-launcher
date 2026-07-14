package app.codexlauncher.launcher.home

import android.content.Context
import android.text.format.DateUtils

internal fun lastConnectedLabel(context: Context, epochMillis: Long): String =
    "Last connected " +
        DateUtils.formatDateTime(
            context,
            epochMillis,
            DateUtils.FORMAT_SHOW_DATE or DateUtils.FORMAT_SHOW_TIME or DateUtils.FORMAT_ABBREV_ALL,
        )
