## Context

`fyne package -os android` (fyne.io/tools v1.7.2) делает всё сам:
- компилирует Go в `lib<имя>.so` (`go build -buildmode=c-shared`, `CC` — clang из NDK для каждой архитектуры, `GOARM=7` для arm);
- кладёт в APK готовый `classes.dex` со своими `org.golang.app.GoNativeActivity` и `FyneNotificationReceiver` (исходники — `fyne.io/fyne/v2/internal/driver/mobile/app/*.java`);
- генерирует manifest (`GoNativeActivity` — `NativeActivity` с `meta-data android.app.lib_name`), ресурсы иконки;
- генерирует `fyne_metadata_init.go` (`app.SetMetadata`: ID, имя, версия, сборка, иконка) — от него зависит `a.Metadata()` в `cmd/mangareader/app_android.go`.

Добавить в такой APK свой Java/Kotlin-код или AAR нельзя. Существующий Android-код приложения — JNI из C/Go (`internal/storage/saf_android.c`) через `driver.AndroidContext`; он не меняется.

Установлено у разработчика (проверено): Android Studio с JBR 21, `JAVA_HOME` → JDK 11, NDK 21.4, SDK Platform 31/34, Build-tools 31/37, без Command-line Tools.

Сеть и rate limiting не касаются изменения (Gradle скачивает зависимости только при сборке).

## Goals / Non-Goals

**Goals:** сборка Gradle с тем же поведением приложения; место для своего Kotlin-кода и AAR (для `android-browser`); arm64, Android 11+; простая замена/добавление архитектур; стабильная подпись.

**Non-Goals:** браузер, Google Play/AAB, другие архитектуры сейчас.

## Decisions

### D1. Раскладка

```
android/
├── gradlew, gradlew.bat, gradle/wrapper/…      Gradle Wrapper (в репозитории)
├── settings.gradle.kts
├── gradle.properties                          org.gradle.java.home не задаётся (см. D5)
└── app/
    ├── build.gradle.kts                       applicationId, minSdk 30, abiFilters, подпись
    └── src/main/
        ├── AndroidManifest.xml                свой (копия шаблона Fyne + minSdk)
        ├── res/mipmap-*/ic_launcher.png       из Icon.png (генерируется Makefile)
        ├── java/org/golang/app/*.java         ← копируются из модуля Fyne при сборке (в .gitignore)
        └── jniLibs/arm64-v8a/libmangareader.so ← собирается Makefile (в .gitignore)
```

### D2. Java-классы Fyne — из модуля нужной версии

Makefile находит каталог модуля (`go list -m -f "{{.Dir}}" fyne.io/fyne/v2`) и копирует `internal/driver/mobile/app/GoNativeActivity.java` и `FyneNotificationReceiver.java` в `android/app/src/main/java/org/golang/app/`. Так Java-часть всегда соответствует Go-части Fyne из `go.mod`; при обновлении Fyne ничего переносить вручную не нужно.

*Альтернатива:* закоммитить копии — отклонено: разъедутся с Go-частью при обновлении Fyne.

### D3. Go → `libmangareader.so`

Для каждой архитектуры из `ANDROID_ABIS` (по умолчанию `arm64-v8a`):

```
GOOS=android GOARCH=arm64 CGO_ENABLED=1
CC=<NDK>/toolchains/llvm/prebuilt/windows-x86_64/bin/aarch64-linux-android30-clang(.cmd)
go build -buildmode=c-shared -tags migrated_fynedo -ldflags "-X main.version=… -s -w -extldflags=-Wl,-z,max-page-size=16384"
         -o android/app/src/main/jniLibs/arm64-v8a/libmangareader.so ./cmd/mangareader
```

`max-page-size=16384` — выравнивание сегментов по страницам 16 КБ (Android 15+ устройства с 16-КБ страницами, требование Google Play; S-G1).

Соответствие ABI → `GOARCH`/префикс clang: `arm64-v8a`→`arm64`/`aarch64-linux-android`, `armeabi-v7a`→`arm`+`GOARM=7`/`armv7a-linux-androideabi`, `x86_64`→`amd64`/`x86_64-linux-android`. Имя библиотеки — `mangareader` (как `android.app.lib_name` в manifest).

Метаданные: `cmd/mangareader/metadata_android.go` (`//go:build android`) вызывает `app.SetMetadata` с ID, именем, версией и номером сборки — вместо генерируемого `fyne_metadata_init.go`. Версия передаётся тем же `-X main.version`.

### D4. Gradle-модуль

- `namespace`/`applicationId = "io.github.mangareader.app"`, `minSdk = 30`, `targetSdk = compileSdk = 35`.
- `ndk { abiFilters += <ANDROID_ABIS> }` — одна настройка вместе с Makefile (передаётся свойством `-PmangareaderAbis=arm64-v8a`).
- AGP 9.x поддерживает Kotlin встроенно — отдельный плагин не нужен (понадобится в `android-browser`); кода на Kotlin в этом изменении нет.
- `versionName`/`versionCode` — из свойств `-PversionName`, `-PversionCode` (Makefile берёт из `FyneApp.toml`).
- Подпись: отладочная сборка — стандартный отладочный ключ Android SDK (`%USERPROFILE%\.android\debug.keystore`, создаётся автоматически); release — из переменных `MANGAREADER_KEYSTORE`, `MANGAREADER_KEY_ALIAS`, `MANGAREADER_KEY_PASSWORD`.

### D5. JDK

Makefile выбирает JDK для Gradle: `JAVA_HOME`, если это JDK 17+; иначе JBR Android Studio (`%ProgramFiles%\Android\Android Studio\jbr`); иначе ошибка с подсказкой. `JAVA_HOME` пользователя не меняется глобально.

### D6. Цели Makefile

**Реализовано Go-утилитой `tools/android-build`** (как `tools/fetch-firefox`): поиск SDK/NDK/JDK, копирование Java-классов Fyne и иконки, сборка `.so` по архитектурам, запуск `gradlew` и копирование APK — одинаково из PowerShell, cmd и Git Bash, с unit-тестами. `gradlew.bat` вызывается по абсолютному пути: при `NoDefaultCurrentDirectoryInExePath=1` (так бывает в IDE-окружениях) cmd.exe не запускает программы из текущей папки по имени. Makefile лишь вызывает утилиту. Исходный план:

```
build-android: android-java android-so android-icon
	cd android && gradlew assembleDebug -PmangareaderAbis=… -PversionName=… -PversionCode=…
	copy app/build/outputs/apk/debug/app-debug.apk → dist/mangareader.apk
```
`build-android-release` — то же с `assembleRelease`. NDK: `ANDROID_NDK_HOME`, иначе последняя версия из `%ANDROID_HOME%\ndk` не ниже 27; иначе ошибка «Установите NDK 27: Android Studio → Settings → Android SDK → SDK Tools».

## Risks / Trade-offs

- [`GoNativeActivity.java` зависит от внутренностей Fyne (JNI-сигнатуры в `android.c`)] → копируется из того же модуля, что и Go-код; спайк проверяет выбор папки, клавиатуру, «Назад».
- [Без `fyne_metadata_init.go` что-то в Fyne ждёт метаданные] → свой `SetMetadata`; спайк проверяет `a.Metadata()` и настройки (`Preferences` требует ID).
- [Смена ключа подписи: старую версию нужно удалить] → однократно; в README — «удалите старую версию, выберите папку заново». Файлы манги в `Download/manga` не затрагиваются.
- [Windows-пути и `.cmd`-обёртки clang в NDK] → Makefile использует `clang.cmd` на Windows (как `fyne`).
- [Размер APK] → только arm64 и `-s -w`: ожидается ~35 МБ вместо 124 МБ.

## Migration Plan

1. Удалить установленную версию (подпись отличается).
2. Установить новую, выбрать папку заново.
Откат — сборка прежним `fyne package` (цель сохраняется как `build-android-fyne` до архивации изменения).

## Результаты спайка S-G1 (2026-09-25, `spikes/android-gradle`)

Проверено на Samsung Galaxy S25 Ultra (SM-S938B), Android 16, arm64, по беспроводной отладке.

- **Сборка:** Gradle 9.8.0 (Wrapper, сгенерирован один раз из скачанного дистрибутива, SHA-256 сверен), Android Gradle Plugin **9.4.1** (встроенная поддержка Kotlin — отдельный плагин Kotlin не нужен), JBR 21 из Android Studio. Первая сборка — 5,5 мин (загрузка зависимостей), повторные — десятки секунд. AGP сам доустановил **Build-Tools 36** (лицензия принята автоматически; `sdkmanager --licenses` в новых Command-line Tools больше не нужен).
- **Go → `.so`:** `go build -buildmode=c-shared` с `aarch64-linux-android30-clang.cmd` из NDK 27.3 — 9 с, 23 МБ. APK — 25,7 МБ (было 124 МБ у `fyne package`), `lib/arm64-v8a` только, minSdk 30, targetSdk 35.
- **16 КБ страницы:** Android 16 показал «приложение не поддерживает страницы памяти 16 КБ… `libmangareader.so`: сегмент LOAD не выровнен». Причина — выравнивание `0x1000`; с `-extldflags=-Wl,-z,max-page-size=16384` стало `0x4000`, предупреждение исчезло. Флаг обязателен (D3). Та же проблема есть и в сборке `fyne package` с NDK 21.
- **Java-классы Fyne** из модуля компилируются без изменений (зависят только от `android.R`). `GoNativeActivity` загружает библиотеку через `System.loadLibrary(lib_name)` — Java/Kotlin `native`-методы находят экспортированные `Java_…`-символы Go-библиотеки обычным способом.
- **Работает:** запуск, `app.SetMetadata` вместо `fyne_metadata_init.go` (ID и версия видны), `Preferences` (счётчик запусков переживает `adb install -r`), экранная клавиатура, выбор папки SAF (`Download/manga`, список файлов), «Назад» доходит до Fyne как `Back`.
- **Не связано со сборкой** (одинаково в `fyne package`, проверено сравнительной сборкой того же спайка): при вводе через `adb input keyevent`/`input text` буквы удваиваются (особенность Fyne при аппаратных нажатиях); на Samsung с рабочим профилем перед выбором папки появляется окно «Open File: Личное / Рабочий»; в логе `Fyne error: Failed to load user locales — no current JVM`.
- Длинная надпись без переноса растягивает окно Fyne шире экрана — это свойство вёрстки спайка, не сборки.

## Open Questions

- Нет.
