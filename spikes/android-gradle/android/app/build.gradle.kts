plugins {
    id("com.android.application")
}

// архитектуры задаются одним свойством: -PmangareaderAbis=arm64-v8a,armeabi-v7a
val abis = (findProperty("mangareaderAbis") as String? ?: "arm64-v8a").split(",").map { it.trim() }

android {
    namespace = "io.github.mangareader.spike.gradle"
    compileSdk = 35

    defaultConfig {
        applicationId = "io.github.mangareader.spike.gradle"
        minSdk = 30
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"
        ndk { abiFilters += abis }
    }
    buildTypes {
        release { isMinifyEnabled = false }
    }
    packaging {
        // .so уже без отладочной информации (go build -ldflags "-s -w")
        jniLibs { keepDebugSymbols += "**/*.so" }
    }
}
