package com.usbridge.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

private val DarkColorScheme = darkColorScheme(
    primary = BridgePrimary80,
    onPrimary = Color(0xFF35275F),
    primaryContainer = Color(0xFF4D3D75),
    onPrimaryContainer = BridgePrimary90,
    secondary = BridgeSecondary80,
    onSecondary = Color(0xFF332D41),
    secondaryContainer = Color(0xFF4A4458),
    onSecondaryContainer = BridgeSecondary90,
    tertiary = BridgeTertiary80,
    onTertiary = Color(0xFF202F5D),
    tertiaryContainer = Color(0xFF374776),
    onTertiaryContainer = BridgeTertiary90,
    error = BridgeError80,
    onError = Color(0xFF690005),
    errorContainer = Color(0xFF93000A),
    onErrorContainer = BridgeError90,
    background = DarkSurface,
    onBackground = DarkOnSurface,
    surface = DarkSurface,
    onSurface = DarkOnSurface,
    surfaceVariant = DarkSurfaceContainerHighest,
    onSurfaceVariant = DarkOnSurfaceVariant,
    outline = DarkOutline,
    outlineVariant = DarkOutlineVariant,
    surfaceDim = DarkSurface,
    surfaceBright = DarkSurfaceBright,
    surfaceContainerLowest = DarkSurfaceContainerLowest,
    surfaceContainerLow = DarkSurfaceContainerLow,
    surfaceContainer = DarkSurfaceContainer,
    surfaceContainerHigh = DarkSurfaceContainerHigh,
    surfaceContainerHighest = DarkSurfaceContainerHighest,
    inverseSurface = DarkOnSurface,
    inverseOnSurface = DarkSurfaceContainerHigh,
    inversePrimary = BridgePrimary40,
    surfaceTint = BridgePrimary80
)

private val LightColorScheme = lightColorScheme(
    primary = BridgePrimary40,
    onPrimary = BridgePrimary100,
    primaryContainer = BridgePrimary90,
    onPrimaryContainer = Color(0xFF211047),
    secondary = BridgeSecondary40,
    onSecondary = Color.White,
    secondaryContainer = BridgeSecondary90,
    onSecondaryContainer = Color(0xFF1E192B),
    tertiary = BridgeTertiary40,
    onTertiary = Color.White,
    tertiaryContainer = BridgeTertiary90,
    onTertiaryContainer = Color(0xFF091A45),
    error = BridgeError40,
    onError = Color.White,
    errorContainer = BridgeError90,
    onErrorContainer = Color(0xFF410002),
    background = LightSurface,
    onBackground = LightOnSurface,
    surface = LightSurface,
    onSurface = LightOnSurface,
    surfaceVariant = LightSurfaceContainerHighest,
    onSurfaceVariant = LightOnSurfaceVariant,
    outline = LightOutline,
    outlineVariant = LightOutlineVariant,
    surfaceDim = LightSurfaceDim,
    surfaceBright = LightSurface,
    surfaceContainerLowest = LightSurfaceContainerLowest,
    surfaceContainerLow = LightSurfaceContainerLow,
    surfaceContainer = LightSurfaceContainer,
    surfaceContainerHigh = LightSurfaceContainerHigh,
    surfaceContainerHighest = LightSurfaceContainerHighest,
    inverseSurface = Color(0xFF322F35),
    inverseOnSurface = Color(0xFFF5EFF7),
    inversePrimary = BridgePrimary80,
    surfaceTint = BridgePrimary40
)

@Composable
fun USBridgeTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    dynamicColor: Boolean = false,
    content: @Composable () -> Unit
) {
    // A stable product palette is intentional here. Dynamic color can be offered
    // later as an explicit preference without changing status semantics per device.
    val colorScheme = if (darkTheme) DarkColorScheme else LightColorScheme

    MaterialTheme(
        colorScheme = colorScheme,
        typography = Typography,
        shapes = USBridgeShapes,
        content = content
    )
}
