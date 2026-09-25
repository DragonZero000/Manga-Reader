plugins {
    id("com.android.application")
}

android {
    namespace = "io.github.mangareader.spike.browser"
    compileSdk {
        version = release(37) { minorApiLevel = 1 }
    }

    defaultConfig {
        applicationId = "io.github.mangareader.spike.browser"
        minSdk = 30
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"
        ndk { abiFilters += "arm64-v8a" }
    }
    packaging {
        jniLibs {
            keepDebugSymbols += "**/libmangareader.so"
            // сжимать .so в APK (распаковываются при установке): libxul ~150 МБ
            useLegacyPackaging = true
        }
    }
}

dependencies {
    implementation("org.mozilla.geckoview:geckoview:156.0.20260921121718")
}
