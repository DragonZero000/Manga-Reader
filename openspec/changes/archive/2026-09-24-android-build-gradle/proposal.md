## Why

Android-пакет сейчас собирает `fyne package`: он сам компилирует Go, подставляет свою `GoNativeActivity` и собирает APK, но не позволяет добавить в APK ни свой Java/Kotlin-код, ни сторонние библиотеки. Встроенный браузер на Android (изменение `android-browser`) требует GeckoView — AAR с нативными библиотеками и собственные экраны на Kotlin. Поэтому сборку нужно перевести на Gradle, не меняя поведения приложения.

## What Changes

- Android-пакет собирается Gradle-проектом в папке `android/`: Go-часть компилируется в `libmangareader.so` (`go build -buildmode=c-shared`, NDK clang), `GoNativeActivity` и остальные Java-классы Fyne берутся из модуля Fyne той версии, что в `go.mod`, manifest — свой.
- `make build-android` по-прежнему выдаёт `dist/mangareader.apk`; Kotlin и Gradle скачиваются Gradle Wrapper автоматически, JDK берётся из Android Studio (JBR).
- **BREAKING (платформа):** минимальная версия — **Android 11** (API 30); архитектура — только **arm64-v8a**. Список архитектур задаётся в одном месте; добавить `armeabi-v7a` или `x86_64` — одна строка.
- **BREAKING (установка):** APK подписывается ключом проекта (для отладочных сборок — отладочным ключом Android SDK). Версию, собранную `fyne package`, один раз нужно удалить перед установкой новой; папку библиотеки придётся выбрать заново (файлы манги не затрагиваются).
- Поведение приложения не меняется: запуск, выбор папки (SAF), чтение, клавиатура в поиске, кнопка «Назад», иконка, уведомления.
- Сборка для Windows не меняется.

## Non-goals

- Встроенный браузер на Android (изменение `android-browser`).
- Публикация в Google Play (AAB, ключ публикации).
- Архитектуры кроме arm64.

## Capabilities

### New Capabilities
- `android-build`: сборка Android-пакета через Gradle — состав APK, минимальная версия, архитектуры, подпись, неизменность поведения.

### Modified Capabilities
<!-- нет: требования к поведению приложения не меняются -->

## Impact

- Новое: `android/` (Gradle Wrapper, `settings.gradle.kts`, `app/build.gradle.kts`, `AndroidManifest.xml`, ресурсы иконки), `cmd/mangareader/metadata_android.go` (метаданные приложения, которые раньше подставлял `fyne package`).
- `Makefile`: `build-android` → сборка `.so` + `gradlew assembleDebug`/`assembleRelease`; поиск NDK и JBR.
- `README.md`: требования (NDK 27, SDK Platform 35/36, Command-line Tools), команды, переустановка.
- Нужные инструменты: Android SDK Platform API 35/36, NDK 27 (LTS), Command-line Tools; JDK 17+ (JBR из Android Studio).
