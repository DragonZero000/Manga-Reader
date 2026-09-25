## 1. Подготовка и спайк

- [x] 1.1 Инструменты: SDK Platform API 35, NDK 27, Command-line Tools (Android Studio → Settings → Android SDK); принять лицензии `sdkmanager --licenses`
- [x] 1.2 Спайк S-G1 (`spikes/android-gradle`): пустое Fyne-приложение (окно, поле ввода, выбор папки) — `.so` через `go build -buildmode=c-shared` + Gradle-проект с Java-классами Fyne; запуск на телефоне arm64 с Android 11+; результаты — в design.md

## 2. Gradle-проект

- [x] 2.1 `android/`: Gradle Wrapper, `settings.gradle.kts`, `app/build.gradle.kts` (applicationId, minSdk 30, compile/targetSdk 35, abiFilters из свойства, Kotlin-плагин, подпись debug/release)
- [x] 2.2 `AndroidManifest.xml` по шаблону Fyne (GoNativeActivity, lib_name `mangareader`, FyneNotificationReceiver, разрешения), иконка из `Icon.png` в `res/mipmap-*`
- [x] 2.3 `.gitignore`: `android/app/src/main/java/org/golang/`, `jniLibs/`, `android/.gradle`, `android/app/build`

## 3. Go-часть

- [x] 3.1 `cmd/mangareader/metadata_android.go`: `app.SetMetadata` (ID, имя, версия, сборка)
- [x] 3.2 Makefile: копирование Java-классов Fyne из модуля (`go list -m`), сборка `.so` для каждой архитектуры из `ANDROID_ABIS` с clang из NDK 27, выбор JDK (JAVA_HOME ≥17 или JBR)
- [x] 3.3 `build-android` / `build-android-release` через `gradlew`, результат в `dist/mangareader.apk`; понятные ошибки при отсутствии NDK/JDK; прежняя сборка — `build-android-fyne` до архивации

## 4. Проверка

- [x] 4.1 APK: только `lib/arm64-v8a`, minSdk 30, applicationId и версия; размер
- [x] 4.2 На телефоне: первый запуск и выбор папки, чтение, поиск с клавиатурой, «Назад», иконка; повторная установка `adb install -r` без потери папки
- [x] 4.3 Проверка добавления архитектуры: сборка с `ANDROID_ABIS="arm64-v8a armeabi-v7a"` даёт обе библиотеки
- [x] 4.4 README: требования, команды, переустановка при переходе, добавление архитектур
