[English](../en/building.md) | **Русский**

# Сборка и релизы

- [Требования](#требования)
- [Команды](#команды)
- [Сборка для Android](#сборка-для-android)
- [Копия Fyne](#копия-fyne)
- [Версия](#версия)
- [Встроенный браузер при разработке](#встроенный-браузер-при-разработке)
- [Замер кадров](#замер-кадров)
- [CI](#ci)
- [Выпуск релиза](#выпуск-релиза)
- [Ключ подписи APK и секреты GitHub](#ключ-подписи-apk-и-секреты-github)

## Требования

- **Go 1.26+**.
- **Windows:** gcc (например, [MinGW-w64](https://www.mingw-w64.org/) или TDM-GCC) — Fyne использует cgo; GNU make (`winget install GnuWin32.Make` или `choco install make`).
- **Android:** [Android Studio](https://developer.android.com/studio) (из неё берётся JDK — JBR), а в Android Studio → Settings → Android SDK:
  - *SDK Tools*: NDK (Side by side) **27** или новее, Android SDK Command-line Tools;
  - *SDK Platforms* ставить вручную не обязательно: недостающие платформы и Build-Tools Gradle скачивает сам.

  Kotlin и Gradle отдельно ставить не нужно — их скачивает Gradle Wrapper. Телефон — Android 11+ (arm64).

Linux и macOS как платформы приложения пока не поддерживаются; собрать APK на Linux можно (так делает CI).

## Команды

Запускаются из корня проекта и работают из PowerShell, cmd и Git Bash:

```sh
make run                    # запуск в режиме разработки (проверки потоков Fyne включены)
make test                   # go vet + go test (включая проверку документации)
make build-windows          # dist/mangareader.exe
make package-windows        # dist/MangaReader/ с браузером и лицензиями и dist/MangaReader.zip
make build-android          # dist/mangareader.apk (debug, arm64)
make build-android-dev      # dist/mangareader-dev.apk (отдельное приложение «MangaReader Dev» с замером кадров)
make build-android-release  # dist/mangareader-release.apk (ключ — см. ниже)
make browser                # портативный Firefox ESR в browser/firefox (для make run, только Windows)
make clean                  # удалить dist/
```

При запуске через `make run` (`go run`) папкой приложения считается рабочая папка: `manga/`, `settings.json`, `library.db` и `browser/` появятся в корне проекта (они в `.gitignore`).

Код собирается только с тегом `sqlite_fts5`: каталог библиотеки (`internal/catalog`) использует SQLite с полнотекстовым поиском FTS5 (`github.com/mattn/go-sqlite3`, cgo). Цели Makefile и CI добавляют тег сами; при ручном запуске — `go test -tags sqlite_fts5 ./...`. Без тега сборка останавливается ошибкой `каталогу_нужен_тег_сборки_sqlite_fts5`. В редакторе с gopls добавьте тег в настройки (`"gopls": {"buildFlags": ["-tags=sqlite_fts5"]}`). Первая сборка на каждой платформе компилирует SQLite из C — 1–2 минуты, дальше берётся кэш Go.

> `make package-windows` не очищает `dist/MangaReader/`. Если вы запускали `mangareader.exe` оттуда, в zip попадут ваши `manga/` и `browser/profile/` (cookies, история). Для раздачи используйте zip из [Releases](https://github.com/DragonZero000/Manga-Reader/releases) или удалите `dist/` перед сборкой (`make clean`).

## Сборка для Android

`make build-android` (он же `go run ./tools/android-build`) собирает Go-часть в `libmangareader.so` компилятором из NDK, берёт Java-классы Fyne из копии Fyne в `third_party/fyne`, кладёт `LICENSE` и `THIRD_PARTY_NOTICES.md` в `assets/` и собирает APK Gradle-проектом из папки `android/`.

- **SDK** — `ANDROID_HOME` (Android Studio задаёт его сама). **NDK** — `ANDROID_NDK_HOME` или самая новая версия ≥ 27 из `%ANDROID_HOME%\ndk`. **JDK** — `JAVA_HOME`, если это JDK 17+, иначе JBR из Android Studio.
- **Архитектуры** — `ANDROID_ABIS` (по умолчанию `arm64-v8a`): `make build-android ANDROID_ABIS=arm64-v8a,armeabi-v7a`. Доступны `arm64-v8a`, `armeabi-v7a`, `x86_64`, `x86`; каждая добавляет к APK ~25 МБ.
- **Подпись.** Debug-сборка подписывается отладочным ключом Android SDK этого компьютера — новые сборки ставятся поверх (`adb install -r dist/mangareader.apk`) без потери настроек. Release-сборка подписывается ключом из переменных `MANGAREADER_KEYSTORE` (путь к `.jks`), `MANGAREADER_KEYSTORE_PASSWORD`, `MANGAREADER_KEY_ALIAS`, `MANGAREADER_KEY_PASSWORD`; без них APK не подписан. Флаг `go run ./tools/android-build -release -require-signing` запрещает сборку без ключа (так собирает CI).
- APK, подписанные разными ключами (ваш debug и релизы проекта), поверх друг друга не ставятся: переход — один раз через удаление приложения. Файлы манги при этом не затрагиваются.
- Библиотека выравнивается по страницам памяти 16 КБ (иначе Android 15+ предупреждает о несовместимости).

## Копия Fyne

Fyne собирается из изменённой копии в [`third_party/fyne`](../../third_party/fyne): в `go.mod` — `replace fyne.io/fyne/v2 => ./third_party/fyne`, а `require` остаётся на версии-основе. Копия меняет темп кадров (см. [Архитектура](architecture.md#темп-кадров)); все правки перечислены в [`PATCHES.md`](../../third_party/fyne/PATCHES.md) и помечены в коде `// mangareader:`. Копия — отдельный модуль, поэтому `go vet ./...` и `go test ./...` корня её не обходят; её собственные тесты: `cd third_party/fyne && go test ./internal/pacing ./internal/animation`. `internal/archtest` проверяет, что `replace` указывает на копию, а версия-основа в `PATCHES.md` совпадает с `require`.

Обновление Fyne — ручной перенос патча по разделу «Перенос на новую версию Fyne» в `PATCHES.md`: скопировать новый модуль из кэша модулей, перенести новые файлы и все правки с пометкой `// mangareader:`, обновить версию там и в `go.mod`, затем `make test`, сборки и замер кадров на телефоне и ПК (`1v` и `miss` не хуже прежних).

## Версия

Версия задаётся **только** в `FyneApp.toml`, поле `Version`, в формате `MAJOR.MINOR.PATCH` (без суффиксов, `MINOR` и `PATCH` ≤ 99). Её берут все сборки; `make VERSION=…` не действует. Номер сборки Android вычисляется: `MAJOR*10000 + MINOR*100 + PATCH` (`0.2.0` → `200`), поэтому новая версия всегда ставится поверх предыдущей. `go run ./tools/version` печатает версию, `-code` — номер сборки.

## Встроенный браузер при разработке

`make browser` один раз скачивает Firefox ESR в `browser\firefox` (версия и SHA-256 закреплены в `tools/fetch-firefox`); без него кнопка **Браузер** показывает «Браузер не найден». Установщик кэшируется в `browser\.cache`. Работает только на Windows.

Базовая сборка — `en-US`; для каждого другого языка приложения `fetch-firefox` добавляет языковой пакет Firefox (`win64/xpi/<язык>.xpi`, проверка по SHA-256) в `browser\firefox\langpacks\`. При запуске браузера пакет языка приложения копируется в профиль. Языковой пакет работает только с той же версией ESR: при обновлении `version` обновите сумму установщика и суммы всех языковых пакетов из `SHA256SUMS` релиза одновременно. Новый язык приложения — строка в списке `langpacks` и `internal/browser/ext/_locales/<язык>/messages.json`.

Интеграционные тесты браузера запускаются, если задать `MANGAREADER_BROWSER_IT` — путь к папке Firefox; без переменной они пропускаются. `TestIntegrationLocale` проверяет язык Firefox и `Accept-Language`; `MANGAREADER_BROWSER_PROFILE` задаёт ему профиль (например, копию существующего).

## Замер кадров

Сборка с тегом `frameprobe` измеряет, насколько ровно кадры попадают на экран. Сама она ничего не запускает: в «Настройках» появляется раздел **Разработка** с кнопками:

- **Запустить тесты** — сценарии автопрокрутки один за другим (около 3 минут, экран не трогать), сводка по каждому;
- **Ручной замер** / **Остановить замер** — запись кадров, пока вы листаете сами, при остановке — сводка `manual`;
- **Поделиться** (Android) — системное меню «Поделиться» с текстом последнего отчёта; на ПК — **Открыть папку** с отчётами.

По окончании тестов или ручного замера отчёт (шапка: версия, платформа, размер и масштаб экрана, `refresh` — частота экрана, `pacing` — режим темпа кадров (`choreographer`, `vsync`, `dwm` или `timer`), `present` — откуда время показа кадров (`egl` — точное, из `EGL_ANDROID_get_frame_timestamps`; `dwm` — vblank DWM после отправки кадра; `swap` — vsync после `eglSwapBuffers`), «Ограничить 60 Гц», плотность сетки, число галерей; затем строки сводок) сохраняется в файл `perf-ГГГГММДД-ЧЧММСС.txt`: на Android — в скрытую папку `.perf` в папке библиотеки (сканер её пропускает), на ПК — в папку `perf` рядом с программой. Каждая сводка пишется и в журнал. В обычных сборках раздела нет.

```sh
make build-android EXTRA_TAGS=frameprobe   # APK с замером
make build-android-dev                     # то же отдельным приложением, см. ниже
adb logcat | grep frameprobe               # frameprobe[grid]: n=2556 p50=6.2ms p95=6.5ms max=12.7ms slow=5 >25ms=0 1v=99.7% miss=0.2% dy=0.066 gc=3.1ms/24 gcmax=1.3ms heap=86MB
make run EXTRA_TAGS=frameprobe             # то же на ПК (в консоль)
```

На ПК тесты можно запустить и без кнопки: `MANGAREADER_PROBE_RUN=1` в окружении сборки с `frameprobe` запускает их через 5 с после показа окна (окно должно оставаться на экране).

Сценарии (метка сводки): `grid` — сетка библиотеки; `search` — результаты поиска `pages:>0`; `details` — страница произведения первой галереи; `settings`; `strip` и `paged` — читалка на галерее с наибольшим числом страниц; `tabs` — переключение вкладок каждые 500 мс; `density-…` — библиотека при каждой плотности сетки (телефон: 2, 3, 4 колонки; ПК: `s`, `m`, `l`). Прокрутка — 1500 логических px/с, 8 с вниз и 8 с вверх (для каждой плотности 4 + 4 с); постраничный режим листается каждые 300 мс. В конце журнала — `frameprobe: scenarios done`. Сценарии прокручивают программно и путь касаний не нагружают; для него — ручной замер (на эмуляторе — со свайпами `adb shell input swipe`). `fling-grid` и `fling-strip` бросают сетку библиотеки и ленту читалки от начала три раза вниз со скоростью 3000 логических px/с, каждый раз дожидаясь остановки (не дольше 4 с), затем один раз вверх; идут на телефоне и на ПК, шаги броска — в `dyf` (норма меньше 0,1, `1v` не меньше 95%). Для пальца `dyt` ручного замера должен быть не больше 0,15.

Пока идёт замер, пробник помечает холст грязным на каждом такте: кадр рисуется на каждом vsync, как при прокрутке, и промежутки между кадрами — темп кадров, а не простой. На Android с точным временем показа (EGL) кадры приходят в замер с задержкой в несколько кадров, по порядку.

Поля сводки: `n` — кадров; `p50`, `p95`, `max` — интервалы между показами кадров на экране; `slow` — интервалов дольше 1,5 × медианы (имеет смысл на экране любой частоты); `>25ms` — дольше 25 мс, для сравнения с прошлыми замерами; `1v` — доля кадров, показанных ровно через один vsync после предыдущего; `miss` — доля пропущенных vsync (промежутки длиннее 8 vsync считаются простоем и в `1v`, `miss` и `dy` не входят); `dy` — неровность прокрутки: насколько скорость за vsync в кадре отличается от средней двух соседних, в долях скорости (смещение берётся в момент рисования кадра; плавный разгон и торможение пальца его не увеличивают; `-` в сценариях без прокрутки); `dyt` и `dyf` — та же неровность только по кадрам с пальцем на экране и только по кадрам броска (инерции прокрутки); `tlat` (Android, если палец был на экране) — на сколько раньше кадра берётся положение пальца: 5 мс, если касания приходят сразу, больше — если система отдаёт их пачкой раз за кадр; `-`, если таких кадров нет; `gc` — сумма пауз сборщика мусора / их число, `gcmax` — наибольшая; `heap` — живая куча в конце сводки.

Как читать: норма — p50, равный периоду экрана (8,3 мс при 120 Гц, 6,25 мс при 160 Гц), `1v` не меньше 95%, `miss` меньше 2% и `dy` меньше 0,1. Низкий `1v` с большим `miss` при ровном p50 — кадры не идут по vsync (смотреть `pacing`). Большой `max` и кадры дольше 25 мс означают, что UI-поток занят (например, подготовкой изображений), и искать надо в коде; один долгий кадр поднимает и `dy`.

`make build-android-dev` собирает APK с замером отдельным приложением: ID `io.github.mangareader.app.dev`, название «MangaReader Dev», версия с суффиксом `-dev` (`tools/android-build -dev`). Оно ставится рядом с обычным, и у него свои настройки, папка библиотеки и данные браузера, так что проверка не затрагивает основное приложение. В остальном оно ведёт себя как обычное; тесты — только по кнопке в разделе «Разработка».

`EXTRA_TAGS` — дополнительные теги сборки через запятую для `run`, `build-windows` и `build-android` (`tools/android-build -tags`).

## CI

- [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) — на каждый push и pull request: `go vet` и `go test` на Windows, сборка debug-APK на Ubuntu. Секреты не нужны, работает и для форков.
- [`.github/workflows/release.yml`](../../.github/workflows/release.yml) — сборка релиза по тегу `vX.Y.Z`.
- [`.github/dependabot.yml`](../../.github/dependabot.yml) — ежемесячные обновления GitHub Actions.

## Выпуск релиза

1. Поднимите `Version` в `FyneApp.toml`, закоммитьте, запушьте.
2. Отправьте тег той же версии: `git tag v0.2.0` и `git push origin v0.2.0`.
3. `release.yml` проверит, что тег совпадает с `FyneApp.toml`, прогонит тесты, соберёт `MangaReader-X.Y.Z-windows-x64.zip` (с Firefox) и подписанный `MangaReader-X.Y.Z-android-arm64.apk`, посчитает `SHA256SUMS.txt` и создаст **черновик** релиза.
4. Проверьте черновик и заметки (GitHub генерирует их из изменений с прошлого тега) и нажмите *Publish release*.

Если сборка упала на проблеме в коде: исправьте, закоммитьте и перенесите тег на новый коммит (`git tag -f vX.Y.Z` и `git push -f origin vX.Y.Z`) — это безопасно, пока релиз не опубликован. Упавшие шаги без изменений в коде (например, после исправления секретов) можно перезапустить кнопкой *Re-run failed jobs*.

## Ключ подписи APK и секреты GitHub

Все релизы подписываются **одним** ключом — только тогда новые версии ставятся поверх установленных. Настраивается один раз. Команды — для PowerShell; выполняйте их в папке **вне репозитория**, например `C:\Users\<вы>\Keys\`.

### 1. Создать ключ

```powershell
keytool -genkeypair -v -keystore mangareader-release.jks -alias mangareader -keyalg RSA -keysize 4096 -validity 10000
```

`keytool` есть в JDK и в Android Studio (`<Android Studio>\jbr\bin\keytool.exe`).

| Вопрос | Ответ |
|---|---|
| `Enter keystore password` | Пароль: **только латиница и цифры, без пробелов**, от 6 символов |
| `Re-enter new password` | Тот же пароль |
| `What is your first and last name?` и остальные | Что угодно. **Эти данные видны всем**, кто скачает APK (`apksigner verify --print-certs`) — не пишите личное, например `CN=MangaReader` |
| `Is CN=... correct?` | `yes` |

Отдельный пароль ключа `keytool` не спросит: в формате PKCS12 (по умолчанию) он совпадает с паролем хранилища.

### 2. Проверить ключ и получить отпечаток

```powershell
keytool -list -v -keystore mangareader-release.jks
```

В выводе нужны две строки:

```text
Alias name: mangareader        ← ANDROID_KEY_ALIAS
SHA256: 3A:7F:0C:…:9E          ← ANDROID_CERT_SHA256
```

Если пишет `password was incorrect` — пароль не тот; проще создать ключ заново, пока нет релизов.

### 3. Что класть в секреты

| Имя | Тип | Значение | Как проверить |
|---|---|---|---|
| `ANDROID_KEYSTORE_BASE64` | Secret | Файл `mangareader-release.jks` в base64, **одной строкой** | ~3–4 тыс. символов, начинается с `MII` |
| `ANDROID_KEYSTORE_PASSWORD` | Secret | Пароль из шага 1 | Ровно те символы, без пробелов по краям |
| `ANDROID_KEY_ALIAS` | Secret | `mangareader` | Строка `Alias name` из шага 2 |
| `ANDROID_KEY_PASSWORD` | Secret | Тот же пароль, что `ANDROID_KEYSTORE_PASSWORD` | Для PKCS12 они совпадают |
| `ANDROID_CERT_SHA256` | **Variable** | Строка `SHA256:` из шага 2 | Двоеточия и регистр не важны |

### 4. Записать секреты

**Через GitHub CLI (рекомендуется)** — значения не копируются вручную, лишние пробелы и переводы строк не попадут. Один раз `gh auth login`, затем:

```powershell
$repo = "DragonZero000/Manga-Reader"
[Convert]::ToBase64String([IO.File]::ReadAllBytes("$PWD\mangareader-release.jks")) | gh secret set ANDROID_KEYSTORE_BASE64 --repo $repo
gh secret set ANDROID_KEYSTORE_PASSWORD --repo $repo   # спросит значение
gh secret set ANDROID_KEY_PASSWORD --repo $repo        # спросит значение
gh secret set ANDROID_KEY_ALIAS --body mangareader --repo $repo
gh variable set ANDROID_CERT_SHA256 --body "3A:7F:0C:…:9E" --repo $repo
```

На Linux и в Git Bash base64: `base64 -w0 mangareader-release.jks | gh secret set ANDROID_KEYSTORE_BASE64 --repo "$repo"`.

**Через сайт:** Settings → Secrets and variables → Actions → вкладка **Secrets** (*New repository secret*) для четырёх секретов и вкладка **Variables** для `ANDROID_CERT_SHA256`. Base64 удобно положить в буфер обмена:

```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("$PWD\mangareader-release.jks")) | Set-Clipboard
```

**Не используйте** `certutil -encode` (добавляет строки-заголовки и портит файл) и онлайн-конвертеры.

Если `ANDROID_CERT_SHA256` не задан, первая сборка релиза остановится и выведет отпечаток в журнал — сохраните его в переменную и перезапустите. Дальше CI не выпустит APK, подписанный другим ключом.

### 5. Проверить

Шаг «Ключ подписи» в журнале сборки выводит `SHA-256 файла ключа`. Сравните с `Get-FileHash mangareader-release.jks` (регистр не важен):

- совпадает, шаг зелёный — всё верно;
- совпадает, но «не удалось открыть ключ» — неверен `ANDROID_KEYSTORE_PASSWORD`;
- не совпадает — испорчен `ANDROID_KEYSTORE_BASE64`, запишите его заново через `gh`.

### 6. Резервная копия — обязательно

Сохраните **файл `mangareader-release.jks` и пароль** в двух местах вне компьютера (менеджер паролей, флешка, облако). Из GitHub Secrets их не прочитать обратно. **Потерянный ключ не восстановить**: новые версии перестанут ставиться поверх у всех пользователей. Файлы `*.jks` и `*.keystore` в `.gitignore`.

### Release-APK локально

Задайте переменные `MANGAREADER_KEYSTORE` (путь к `.jks`), `MANGAREADER_KEYSTORE_PASSWORD`, `MANGAREADER_KEY_ALIAS`, `MANGAREADER_KEY_PASSWORD` и выполните `make build-android-release`.
