plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

val appVersionName = providers.gradleProperty("usbridgeVersionName").orElse("0.3.0")
val appVersionCode = providers.gradleProperty("usbridgeVersionCode").orElse("4")
val releaseKeystorePath = providers.environmentVariable("USBRIDGE_ANDROID_KEYSTORE").orNull
val releaseStorePassword = providers.environmentVariable("USBRIDGE_ANDROID_STORE_PASSWORD").orNull
val releaseKeyAlias = providers.environmentVariable("USBRIDGE_ANDROID_KEY_ALIAS").orNull
val releaseKeyPassword = providers.environmentVariable("USBRIDGE_ANDROID_KEY_PASSWORD").orNull
val releaseSigningValues = listOf(
    releaseKeystorePath,
    releaseStorePassword,
    releaseKeyAlias,
    releaseKeyPassword
)
val releaseSigningConfigured = releaseSigningValues.all { !it.isNullOrBlank() }

if (releaseSigningValues.any { !it.isNullOrBlank() } && !releaseSigningConfigured) {
    throw GradleException(
        "Android release signing is only partially configured. Set all four " +
            "USBRIDGE_ANDROID_* signing environment variables."
    )
}
if (releaseSigningConfigured && !file(releaseKeystorePath!!).isFile) {
    throw GradleException("Android release keystore does not exist: $releaseKeystorePath")
}

android {
    namespace = "com.usbridge"
    compileSdk {
        version = release(36) {
            minorApiLevel = 1
        }
    }

    defaultConfig {
        applicationId = "com.usbridge"
        minSdk = 30
        targetSdk = 36
        versionCode = appVersionCode.get().toInt()
        versionName = appVersionName.get()

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    signingConfigs {
        if (releaseSigningConfigured) {
            create("release") {
                storeFile = file(releaseKeystorePath!!)
                storePassword = releaseStorePassword
                keyAlias = releaseKeyAlias
                keyPassword = releaseKeyPassword
                storeType = providers.environmentVariable("USBRIDGE_ANDROID_STORE_TYPE")
                    .orElse("JKS")
                    .get()
            }
        }
    }

    buildTypes {
        release {
            if (releaseSigningConfigured) {
                signingConfig = signingConfigs.getByName("release")
            }
            optimization {
                enable = true
            }
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }
    buildFeatures {
        aidl = true
        buildConfig = true
        compose = true
    }
}

dependencies {
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.compose.material.icons.extended)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.libsu.core)
    implementation(libs.libsu.service)
    testImplementation(libs.junit)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(libs.androidx.junit)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
    debugImplementation(libs.androidx.compose.ui.tooling)
}
