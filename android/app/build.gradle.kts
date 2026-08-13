import java.util.Properties
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.compose.compiler)
}

val releaseStoreFile = providers.environmentVariable("CODEX_LAUNCHER_STORE_FILE").orNull
val releaseStorePassword = providers.environmentVariable("CODEX_LAUNCHER_STORE_PASSWORD").orNull
val releaseKeyAlias = providers.environmentVariable("CODEX_LAUNCHER_KEY_ALIAS").orNull
val releaseKeyPassword = providers.environmentVariable("CODEX_LAUNCHER_KEY_PASSWORD").orNull
val releaseSigningValues = listOf(releaseStoreFile, releaseStorePassword, releaseKeyAlias, releaseKeyPassword)
val releaseSigningConfigured = releaseSigningValues.all { !it.isNullOrBlank() }
if (releaseSigningValues.any { !it.isNullOrBlank() } && !releaseSigningConfigured) {
    throw GradleException("Android release signing requires all CODEX_LAUNCHER signing variables")
}

fun localProperty(name: String): String {
    val file = rootProject.file("local.properties")
    if (!file.isFile) return ""
    val properties = Properties()
    file.inputStream().use(properties::load)
    return properties.getProperty(name).orEmpty()
}

fun quotedJavaString(value: String): String =
    buildString {
        append('"')
        value.forEach { ch ->
            when (ch) {
                '\\', '"' -> append('\\').append(ch)
                else -> append(ch)
            }
        }
        append('"')
    }

android {
    namespace = "app.codexlauncher"
    compileSdk = 36

    defaultConfig {
        applicationId = "app.codexlauncher"
        minSdk = 31
        targetSdk = 36
        versionCode = 1
        versionName = "0.1.0-alpha.1"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    signingConfigs {
        if (releaseSigningConfigured) {
            create("release") {
                storeFile = file(requireNotNull(releaseStoreFile))
                storePassword = requireNotNull(releaseStorePassword)
                keyAlias = requireNotNull(releaseKeyAlias)
                keyPassword = requireNotNull(releaseKeyPassword)
            }
        }
    }

    buildTypes {
        getByName("debug") {
            // Debug-only: operator puts DEEPGRAM_API_KEY in gitignored android/local.properties.
            // Release APKs always compile an empty field so the key cannot ship.
            buildConfigField("String", "DEEPGRAM_API_KEY", quotedJavaString(localProperty("DEEPGRAM_API_KEY")))
        }
        getByName("release") {
            signingConfig = signingConfigs.findByName("release")
            isMinifyEnabled = false
            buildConfigField("String", "DEEPGRAM_API_KEY", quotedJavaString(""))
        }
    }

    // Dogfood OpenAI key import against the signed release install (same signer as
    // phone APK). Default stays debug for day-to-day unit/instrument work unless
    // CODEX_LAUNCHER_TEST_BUILD_TYPE=release is set.
    testBuildType =
        providers.environmentVariable("CODEX_LAUNCHER_TEST_BUILD_TYPE").orElse("debug").get()

    testOptions {
        unitTests.isReturnDefaultValues = true
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(JvmTarget.JVM_17)
    }
}

dependencies {
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.material3)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.activity.compose)
    implementation(libs.datastore.preferences)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.camera.camera2)
    implementation(libs.camera.lifecycle)
    implementation(libs.camera.view)
    implementation(libs.lifecycle.runtime.compose)
    implementation(libs.lifecycle.viewmodel.ktx)
    implementation(libs.zxing.core)
    implementation(libs.tink.android)
    implementation(libs.play.services.auth)
    implementation(libs.msal)
    debugImplementation(libs.compose.ui.tooling)
    debugImplementation(libs.compose.ui.test.manifest)
    testImplementation(libs.junit)
    testImplementation(libs.mockwebserver)
    testImplementation(libs.okhttp.tls)
    androidTestImplementation(platform(libs.compose.bom))
    androidTestImplementation(libs.compose.ui.test.junit4)
    androidTestImplementation(libs.compose.ui.test.junit4.accessibility)
    androidTestImplementation(libs.android.test.junit)
    androidTestImplementation(libs.android.test.runner)
    androidTestImplementation(libs.espresso.intents)
    androidTestImplementation(libs.mockwebserver)
}

tasks.withType<org.gradle.api.tasks.testing.Test>().configureEach {
    inputs.dir(rootProject.file("../protocol"))
}
