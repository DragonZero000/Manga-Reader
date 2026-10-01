[English](README.md) | **Русский**

# MangaReader

[![CI](https://github.com/DragonZero000/Manga-Reader/actions/workflows/ci.yml/badge.svg)](https://github.com/DragonZero000/Manga-Reader/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/DragonZero000/Manga-Reader)](https://github.com/DragonZero000/Manga-Reader/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Офлайн-читалка манги из zip-архивов для **Windows** и **Android** со встроенным браузером на движке Firefox: скачанные в нём архивы сразу попадают в библиотеку. Написана на Go и [Fyne](https://fyne.io).

> Интерфейс на русском и английском: по умолчанию — язык системы (если перевода на него нет — английский), выбор — в **Настройках** → **Язык / Language**.

<p align="center">
  <img src="docs/images/library.png" alt="Библиотека на Windows" height="240">
  <img src="docs/images/reader.png" alt="Читалка на Windows" height="240">
  <img src="docs/images/mobile-library.png" alt="Библиотека на Android" height="240">
</p>

## Возможности

- **Библиотека** — сетка обложек из папки с `.zip`-архивами; новые файлы появляются сами, без перезапуска. Размер карточек (Windows) или число карточек в ряду (Android) — на выбор; кнопка 🎲 открывает случайное произведение.
- **Меню карточки** — **⋮** на карточке, правый клик (ПК) или долгое нажатие (телефон): открыть, читать, открыть в браузере, скопировать название, показать в папке (Windows), сбросить прогресс, удалить — на Windows в корзину, с подтверждением.
- **Метаданные** — названия, теги по типам (автор, группа, персонаж, язык…), дата загрузки и другое из `meta.json` внутри архива; свои теги и скрытие ненужных — без изменения архива.
- **Читалка** — постраничный режим (тап-зоны, свайп, клавиатура, колесо) и лента; масштаб до 500% (Ctrl+колесо на ПК, щипок на Android, двойной тап); переход к странице по номеру; чтение справа налево для манги.
- **Прогресс чтения** — приложение запоминает, где вы остановились: **Продолжить · стр. N** на странице произведения, отметка «дочитано», новые страницы, добавленные в архив, подхватываются.
- **Поиск** — по словам и фразам, тегам (в том числе своим и скрытым), исключению тегов, числу страниц, датам, размеру файла; при вводе или по кнопке; нажатие на тег ищет по нему; 🎲 выбирает случайный результат.
- **Встроенный браузер** — Firefox ESR на Windows, GeckoView на Android, на языке приложения; загрузки сохраняются прямо в библиотеку, приложение запоминает страницу, откуда скачан файл.
- **Ошибки** — всё, что не является архивом с картинками (HTML вместо архива, битый zip, PDF…), собрано на отдельной вкладке с причиной; файлы не удаляются.
- **Портативность на Windows** — всё хранится в одной папке, её можно носить на флешке.

## Скачать

Готовые сборки — на странице [Releases](https://github.com/DragonZero000/Manga-Reader/releases/latest):

| Файл | Для чего |
|---|---|
| `MangaReader-X.Y.Z-windows-x64.zip` | Windows 10/11, 64-бит (~160 МБ, встроенный браузер уже внутри) |
| `MangaReader-X.Y.Z-android-arm64.apk` | Android 11+ (~140 МБ) |
| `SHA256SUMS.txt` | Контрольные суммы: `sha256sum -c SHA256SUMS.txt` (Linux, Git Bash) или `Get-FileHash <файл>` (PowerShell) |

### Установка на Windows

1. Распакуйте zip в любую папку, где есть права на запись (не в `Program Files`), например `D:\MangaReader`.
2. Запустите `MangaReader\mangareader.exe`.
3. Кладите `.zip`-архивы в папку `manga` рядом с приложением или скачивайте их кнопкой **Браузер**.

Для обновления замените `mangareader.exe` и папку `browser\firefox` файлами из нового zip; `manga`, `settings.json`, `library.db`, `user.db` и `browser\profile` сохранятся.

### Установка на Android

1. Скачайте APK на телефон и откройте его; разрешите установку из этого источника, когда Android попросит.
2. При первом запуске выберите папку для библиотеки внутри `Download`, например `Download/manga`.

Новые релизы ставятся **поверх** старых: настройки и выбранная папка сохраняются. Если раньше вы ставили APK, собранный самостоятельно, его подпись другая — удалите его один раз перед установкой релиза (файлы манги не затрагиваются).

## Системные требования

| Платформа | Требования |
|---|---|
| Windows | Windows 10 или 11, x64; ~400 МБ на диске (вместе со встроенным Firefox) |
| Android | Android 11 или новее, arm64; ~300 МБ после установки |
| Linux, macOS | Пока не поддерживаются |

## Документация

- [Руководство пользователя](docs/ru/user-guide.md) — папка библиотеки, формат архива и `meta.json`, меню карточки и удаление, редактирование тегов, читалка и прогресс чтения, синтаксис поиска, браузер, ошибки, настройки.
- [Сборка и релизы](docs/ru/building.md) — требования, команды, Android, выпуск релиза, ключ подписи.
- [Архитектура](docs/ru/architecture.md) — пакеты, платформы, потоки, устройство браузера.

## Для разработчиков

```sh
make run                    # запуск в режиме разработки (нужны Go 1.26+ и gcc)
make test                   # go vet + go test (включая проверку документации)
make build-windows          # dist/mangareader.exe
make package-windows        # dist/MangaReader/ с браузером и лицензиями и dist/MangaReader.zip
make build-android          # dist/mangareader.apk (debug, arm64; нужны Android Studio и NDK 27+)
make build-android-release  # dist/mangareader-release.apk (ключ подписи — см. building.md)
make browser                # портативный Firefox ESR в browser/firefox (для make run, только Windows)
make clean                  # удалить dist/
```

Подробности — в [building.md](docs/ru/building.md). Как предложить изменение — в [CONTRIBUTING.ru.md](CONTRIBUTING.ru.md).

## Лицензия

Код проекта — [MIT](LICENSE). Сторонние компоненты (Firefox ESR, GeckoView, Fyne и др.) распространяются под своими лицензиями — см. [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Firefox — товарный знак Mozilla Foundation; проект не связан с Mozilla.
