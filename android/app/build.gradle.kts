// Android-пакет MangaReader. Go-часть (libmangareader.so) и Java-классы Fyne
// кладёт сюда Makefile (make build-android) — см. openspec/…/android-build-gradle.
plugins {
    id("com.android.application")
}

fun prop(name: String, default: String) = (findProperty(name) as String?) ?: default

// архитектуры задаются одним свойством (Makefile: ANDROID_ABIS)
val abis = prop("mangareaderAbis", "arm64-v8a").split(",").map { it.trim() }.filter { it.isNotEmpty() }

android {
    namespace = "io.github.mangareader.app"
    // GeckoView 156 требует compileSdk 37.1 (AGP сам установит платформу)
    compileSdk {
        version = release(37) { minorApiLevel = 1 }
    }

    defaultConfig {
        applicationId = "io.github.mangareader.app"
        minSdk = 30
        targetSdk = 35
        // версию и versionCode передаёт tools/android-build из FyneApp.toml;
        // значения по умолчанию — только для запуска Gradle напрямую (Android Studio)
        versionName = prop("versionName", "dev")
        versionCode = prop("versionCode", "1").toInt()
        ndk { abiFilters += abis }
    }

    // release подписывается ключом проекта из переменных окружения;
    // debug — стандартным отладочным ключом Android SDK
    val keystore = System.getenv("MANGAREADER_KEYSTORE")
    signingConfigs {
        if (keystore != null) {
            create("release") {
                storeFile = file(keystore)
                storePassword = System.getenv("MANGAREADER_KEYSTORE_PASSWORD") ?: System.getenv("MANGAREADER_KEY_PASSWORD")
                keyAlias = System.getenv("MANGAREADER_KEY_ALIAS")
                keyPassword = System.getenv("MANGAREADER_KEY_PASSWORD")
            }
        }
    }
    buildTypes {
        release {
            isMinifyEnabled = false
            if (keystore != null) signingConfig = signingConfigs.getByName("release")
        }
    }
    packaging {
        // .so уже без отладочной информации (go build -ldflags "-s -w")
        jniLibs {
            // libmangareader.so уже без отладочной информации (-s -w)
            keepDebugSymbols += "**/libmangareader.so"
            // сжимать .so в APK (при установке распаковываются): libxul ~150 МБ
            useLegacyPackaging = true
        }
    }
}

// встроенный браузер: GeckoView (движок Firefox), release-канал.
// При обновлении версии обновите THIRD_PARTY_NOTICES.md (версия и ссылка на исходники).
val geckoviewVersion = "156.0.20260921121718"

dependencies {
    implementation("org.mozilla.geckoview:geckoview:$geckoviewVersion")
}
