## Why

Сейчас приложение можно получить только сборкой на машине автора: нужны Go, gcc, Android Studio и NDK. Чтобы любой мог скачать и установить готовый релиз, нужны автоматические сборки по тегу. Кроме того, APK из разных сборок должны устанавливаться поверх друг друга, а это возможно, только если все релизы подписаны одним постоянным ключом. Проверок на каждый push тоже нет, так что поломку сборки замечают поздно.

## What Changes

- **CI** (`.github/workflows/ci.yml`): на каждый push и pull request — `go vet` и `go test` на Windows, плюс проверочная сборка APK без подписи.
- **Релизы** (`.github/workflows/release.yml`): push тега `vX.Y.Z` запускает сборку. Сначала проверяется, что тег совпадает с `Version` в `FyneApp.toml`, затем проходят тесты, собираются пакеты для Windows и Android, и создаётся **черновик** GitHub Release: автор проверяет его и публикует вручную.
- **Файлы релиза**: `MangaReader-X.Y.Z-windows-x64.zip` (портативная папка с Firefox ESR), `MangaReader-X.Y.Z-android-arm64.apk`, `SHA256SUMS.txt`.
- **Постоянный ключ подписи APK**: release-APK подписывается ключом из GitHub Secrets. Если ключа нет, релиз не собирается: неподписанный или временно подписанный APK никогда не публикуется. Автор создаёт ключ один раз и хранит его резервную копию вне репозитория.
- **`LICENSE` и `THIRD_PARTY_NOTICES.md` внутри пакетов**: в zip для Windows — рядом с `mangareader.exe`, в APK — в `assets/`.
- **`tools/android-build`**: новый флаг `-require-signing` (при release-сборке без ключа сборка завершается ошибкой, а не выдаёт неподписанный APK).
- README: где скачать релиз, как установить APK, как собрать release-APK со своим ключом.

**Платформы:** Windows и Android. Linux-сборка добавится в тот же workflow в изменении `linux-support`.

## Non-goals

- Linux (`linux-support`).
- Двуязычная документация и CONTRIBUTING (`docs`); здесь README правится минимально.
- Google Play, F-Droid, winget, автообновление внутри приложения.
- Подпись Windows-exe сертификатом (Authenticode).
- Автоматическая генерация release notes из коммитов: GitHub предлагает черновик заметок, автор правит его перед публикацией.

## Capabilities

### New Capabilities
- `release-pipeline`: CI на push и pull request; сборка релиза по тегу, проверка версии, состав и имена файлов релиза, контрольные суммы, черновик релиза, лицензии внутри пакетов.

### Modified Capabilities
- `android-build`: требование «Подпись и обновление поверх установленной версии» — release-APK подписывается постоянным ключом проекта; при `-require-signing` без ключа сборка завершается ошибкой; APK содержит лицензии в `assets/`.

## Impact

- Новое: `.github/workflows/ci.yml`, `.github/workflows/release.yml`.
- Изменено:
  - `tools/android-build` (флаг `-require-signing`, копирование лицензий в `assets/`);
  - `Makefile` (`package-windows` кладёт лицензии в папку дистрибутива);
  - `.gitignore` (сгенерированная `android/app/src/main/assets/`);
  - `README.md`.
- Настройки GitHub (делает автор): секреты `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`.
- **Установка:** APK, собранные локально отладочным ключом, и релизные APK подписаны разными ключами. Поверх друг друга они не ставятся: перейти с локальной сборки на релиз можно один раз, удалив старую версию.
