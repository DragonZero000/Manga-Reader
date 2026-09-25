# MangaReader — сборка для Windows и Android.
# Запускать из корня проекта: make build-windows / make build-android

APP_NAME := mangareader
MAIN     := ./cmd/mangareader
DIST     := dist
# Версия — только в FyneApp.toml. «=» — вычисляется лишь в целях сборки
# (make run и make test утилиту не запускают); override запрещает make VERSION=…
override VERSION = $(shell go run ./tools/version)
LDFLAGS  = -X main.version=$(VERSION)
# Код мигрирован на fyne.Do: в релизных сборках отключаем проверки потоков.
# В `make run` FyneApp.toml читается из корня и проверки остаются включены.
TAGS     := migrated_fynedo

# NDK: если ANDROID_NDK_HOME не задан, берём последнюю версию из $(ANDROID_HOME)/ndk.
ANDROID_NDK_HOME ?= $(lastword $(sort $(wildcard $(subst \,/,$(ANDROID_HOME))/ndk/*)))
export ANDROID_NDK_HOME

# Команды оболочки: на Windows всегда cmd.exe (одинаково из PowerShell, cmd и Git Bash).
ifeq ($(OS),Windows_NT)
SHELL      := cmd.exe
.SHELLFLAGS := /c
MKDIR_DIST := if not exist $(DIST) mkdir $(DIST)
RM_DIST    := if exist $(DIST) rmdir /S /Q $(DIST)
COPY_EXE   := copy /Y $(DIST)\$(APP_NAME).exe $(DIST)\MangaReader\ >nul
COPY_LIC   := copy /Y LICENSE $(DIST)\MangaReader\ >nul && copy /Y THIRD_PARTY_NOTICES.md $(DIST)\MangaReader\ >nul
RM_ZIP     := if exist $(DIST)\MangaReader.zip del $(DIST)\MangaReader.zip
# tar из Windows (bsdtar) умеет zip; tar из Git (GNU) — нет
ZIP_DIST   := "%SystemRoot%\System32\tar.exe" -a -cf $(DIST)\MangaReader.zip --options zip:compression=deflate -C $(DIST) MangaReader
else
MKDIR_DIST := mkdir -p $(DIST)
RM_DIST    := rm -rf $(DIST)
COPY_EXE   := cp $(DIST)/$(APP_NAME).exe $(DIST)/MangaReader/
COPY_LIC   := cp LICENSE THIRD_PARTY_NOTICES.md $(DIST)/MangaReader/
RM_ZIP     := rm -f $(DIST)/MangaReader.zip
ZIP_DIST   := cd $(DIST) && zip -qr MangaReader.zip MangaReader
endif

.PHONY: run test build-windows build-android build-android-release browser package-windows clean

run:
	go run $(MAIN)

test:
	go vet ./...
	go test ./...

build-windows:
	$(if $(VERSION),,$(error не удалось прочитать версию из FyneApp.toml — см. сообщение выше))
	$(MKDIR_DIST)
	go build -tags $(TAGS) -ldflags "-H windowsgui $(LDFLAGS)" -o $(DIST)/$(APP_NAME).exe $(MAIN)

# Android-пакет через Gradle (android/): Android Studio, SDK Platform 35,
# NDK 27+. Архитектуры — ANDROID_ABIS (через запятую), версия — FyneApp.toml.
ANDROID_ABIS ?= arm64-v8a

build-android:
	go run ./tools/android-build -abis "$(ANDROID_ABIS)"

# release-APK: подпись ключом из MANGAREADER_KEYSTORE, MANGAREADER_KEY_ALIAS,
# MANGAREADER_KEY_PASSWORD (без них — неподписанный).
build-android-release:
	go run ./tools/android-build -abis "$(ANDROID_ABIS)" -release -o $(DIST)/mangareader-release.apk

# Портативный Firefox ESR для встроенного браузера (только Windows):
# browser/firefox — для make run; версия и SHA-256 закреплены в tools/fetch-firefox.
browser:
	go run ./tools/fetch-firefox -dst browser/firefox -cache browser/.cache

# Папка дистрибутива Windows с браузером и лицензиями и её zip: dist/MangaReader(.zip).
package-windows: build-windows
	go run ./tools/fetch-firefox -dst $(DIST)/MangaReader/browser/firefox -cache browser/.cache
	$(COPY_EXE)
	$(COPY_LIC)
	$(RM_ZIP)
	$(ZIP_DIST)

clean:
	$(RM_DIST)
