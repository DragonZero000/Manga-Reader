## 1. Лицензии в пакетах

- [x] 1.1 `Makefile`, `package-windows`: копировать `LICENSE` и `THIRD_PARTY_NOTICES.md` в `dist/MangaReader/` (варианты для `cmd.exe` и sh, по образцу `COPY_EXE`)
- [x] 1.2 `tools/android-build`: копировать `LICENSE` и `THIRD_PARTY_NOTICES.md` в `android/app/src/main/assets/` перед запуском Gradle; добавить `/android/app/src/main/assets/` в `.gitignore`
- [x] 1.3 Проверить: после `make package-windows` оба файла лежат в `dist/MangaReader/`; после `make build-android` в APK есть `assets/LICENSE` и `assets/THIRD_PARTY_NOTICES.md`; `git status` чист

## 2. Обязательная подпись

- [x] 2.1 `tools/android-build`: флаг `-require-signing`. При `-release` и пустом `MANGAREADER_KEYSTORE` — ошибка до копирования файлов и запуска Gradle, в тексте названы нужные переменные; `-require-signing` без `-release` — ошибка «флаг имеет смысл только с -release»
- [x] 2.2 Вынести проверку в функцию с тестом (`release`/`require`/`keystore` → ошибка или нет)

## 3. CI

- [x] 3.1 `.github/workflows/ci.yml` по D7: триггеры `push`, `pull_request`; `permissions: contents: read`; job `test` (windows-latest, `setup-go` с `go-version-file: go.mod`, `go vet ./...`, `go test ./...`); job `android` (ubuntu-latest, `setup-go`, `setup-java` 17, `gradle/actions/setup-gradle`, `go run ./tools/android-build`)
- [x] 3.2 `.github/dependabot.yml`: экосистема `github-actions`, раз в месяц

## 4. Релиз

- [x] 4.1 `.github/workflows/release.yml`: триггер `push: tags: ['v*.*.*']`, `permissions: contents: read`, `concurrency` по `github.ref`
- [x] 4.2 Job `version`: проверка формата тега `^v[0-9]+\.[0-9]+\.[0-9]+$`; сравнение `go run ./tools/version` с тегом без `v`, при несовпадении — ошибка с обоими значениями; `outputs.version`
- [x] 4.3 Job `test` (windows-latest, `needs: version`): `go vet`, `go test`
- [x] 4.4 Job `windows` (windows-latest, `needs: test`): проверка `gcc --version` (при отсутствии — `choco install mingw`), `choco install make`, кэш `browser/.cache` по `hashFiles('tools/fetch-firefox/main.go')`, `make package-windows`, переименование в `MangaReader-X.Y.Z-windows-x64.zip`, `upload-artifact`
- [x] 4.5 Job `android` (ubuntu-latest, `needs: test`): проверка четырёх секретов с названием отсутствующего; декодирование `ANDROID_KEYSTORE_BASE64` в `$RUNNER_TEMP/release.jks`; `go run ./tools/android-build -release -require-signing -o dist/MangaReader-X.Y.Z-android-arm64.apk` с `MANGAREADER_KEYSTORE`, `MANGAREADER_KEYSTORE_PASSWORD`, `MANGAREADER_KEY_ALIAS`, `MANGAREADER_KEY_PASSWORD`; `upload-artifact`
- [x] 4.6 Там же — проверка отпечатка по D6: `apksigner verify --print-certs` (из последних build-tools), сравнение SHA-256 с `vars.ANDROID_CERT_SHA256` без учёта регистра и двоеточий; если переменной нет — вывести отпечаток и упасть с подсказкой
- [x] 4.7 Job `publish` (ubuntu-latest, `needs: [version, windows, android]`, `permissions: contents: write`): `download-artifact`, `sha256sum` → `SHA256SUMS.txt`, `gh release create "$TAG" --draft --generate-notes --title "MangaReader X.Y.Z"` с тремя файлами

## 5. Документация

- [x] 5.1 `README.md`: раздел «Скачать» (ссылка на Releases, какой файл для какой платформы, проверка `SHA256SUMS.txt`); установка APK (разрешить установку из неизвестных источников; обновление поверх; однократное удаление локальной debug-сборки перед первым релизом)
- [x] 5.2 `README.md`, раздел для разработчиков: как выпустить релиз (версия в `FyneApp.toml` → коммит → тег `vX.Y.Z` → проверить и опубликовать черновик); настройка ключа (команда `keytool`, base64 в PowerShell и sh, `gh secret set` для четырёх секретов, `ANDROID_CERT_SHA256`), предупреждение о резервной копии ключа; флаг `-require-signing`. Перенос в `docs/en` и `docs/ru` — в изменении `docs`

## 6. Проверка

- [x] 6.1 `make test` проходит; `make build-windows`, `make package-windows`, `make build-android` проходят
- [x] 6.2 `go run ./tools/android-build -release -require-signing` без `MANGAREADER_KEYSTORE` завершается ошибкой и не запускает Gradle
- [x] 6.3 Синтаксис workflow проверен (`actionlint`, если доступен, иначе — первый прогон на GitHub)
- [ ] 6.4 После push на GitHub: `ci.yml` зелёный на ветке
- [ ] 6.5 Автор создал ключ и секреты; тег `v0.1.0` → черновик релиза с тремя файлами; `sha256sum -c SHA256SUMS.txt` проходит; zip запускается на Windows с версией `0.1.0`; APK ставится на телефон, а следующий релиз ставится поверх (проверяется при выпуске `0.1.1` или новее)
