package app.codexlauncher.appearance.theme

import android.app.Activity
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.sp
import androidx.core.view.WindowCompat
import app.codexlauncher.R

val InstrumentSansFontFamily =
    FontFamily(
        Font(R.font.instrument_sans_variable, weight = FontWeight.Normal),
        Font(R.font.instrument_sans_variable, weight = FontWeight.Medium),
        Font(R.font.instrument_sans_variable, weight = FontWeight.SemiBold),
    )

val JetBrainsMonoFontFamily =
    FontFamily(
        Font(R.font.jetbrains_mono_variable, weight = FontWeight.Normal),
        Font(R.font.jetbrains_mono_variable, weight = FontWeight.Medium),
    )

@Composable
fun QuietInstrumentTheme(
    mode: AppearanceMode = AppearanceMode.FOLLOW_SYSTEM,
    content: @Composable () -> Unit,
) {
    val dark =
        when (mode) {
            AppearanceMode.FOLLOW_SYSTEM -> isSystemInDarkTheme()
            AppearanceMode.LIGHT -> false
            AppearanceMode.DARK -> true
        }
    val palette = if (dark) QuietInstrumentTokens.deepCharcoal else QuietInstrumentTokens.warmPaper
    val colors =
        if (dark) {
            darkColorScheme(
                primary = Color(palette.signal),
                onPrimary = Color(palette.onSignal),
                background = Color(palette.background),
                onBackground = Color(palette.primaryText),
                surface = Color(palette.raisedSurface),
                onSurface = Color(palette.primaryText),
                onSurfaceVariant = Color(palette.mutedText),
                outline = Color(palette.hairline),
                // B4-004: Material3's OutlinedButton border and default dividers read
                // outlineVariant, not outline. Left unset it fell back to the stock M3
                // grey; bind it to the same hairline token so every secondary border
                // matches the divider color the design specifies.
                outlineVariant = Color(palette.hairline),
                error = Color(palette.error),
            )
        } else {
            lightColorScheme(
                primary = Color(palette.signal),
                onPrimary = Color(palette.onSignal),
                background = Color(palette.background),
                onBackground = Color(palette.primaryText),
                surface = Color(palette.raisedSurface),
                onSurface = Color(palette.primaryText),
                onSurfaceVariant = Color(palette.mutedText),
                outline = Color(palette.hairline),
                // B4-004: Material3's OutlinedButton border and default dividers read
                // outlineVariant, not outline. Left unset it fell back to the stock M3
                // grey; bind it to the same hairline token so every secondary border
                // matches the divider color the design specifies.
                outlineVariant = Color(palette.hairline),
                error = Color(palette.error),
            )
        }
    val view = LocalView.current
    if (!view.isInEditMode) {
        SideEffect {
            val window = (view.context as Activity).window
            val controller = WindowCompat.getInsetsController(window, view)
            controller.isAppearanceLightStatusBars = !dark
            controller.isAppearanceLightNavigationBars = !dark
        }
    }
    MaterialTheme(
        colorScheme = colors,
        typography = quietTypography,
        content = content,
    )
}

private val quietTypography =
    Typography(
        titleMedium =
            TextStyle(
                fontFamily = InstrumentSansFontFamily,
                fontSize = QuietInstrumentTokens.taskTitleSp.sp,
                fontWeight = FontWeight.Medium,
            ),
        bodyLarge = TextStyle(fontFamily = InstrumentSansFontFamily, fontSize = QuietInstrumentTokens.bodyTextSp.sp),
        bodyMedium = TextStyle(fontFamily = InstrumentSansFontFamily, fontSize = QuietInstrumentTokens.bodyTextSp.sp),
        labelLarge =
            TextStyle(
                fontFamily = InstrumentSansFontFamily,
                fontSize = QuietInstrumentTokens.bodyTextSp.sp,
                fontWeight = FontWeight.Medium,
            ),
        labelSmall =
            TextStyle(
                fontFamily = JetBrainsMonoFontFamily,
                fontSize = 10.sp,
                fontWeight = FontWeight.Medium,
            ),
    )
