#!/usr/bin/env bash
# Сборка спайка: .so (NDK 27, выравнивание 16 КБ), Java-классы Fyne, Gradle.
set -e
cd "$(dirname "$0")"
FYNE=$(go list -m -f '{{.Dir}}' fyne.io/fyne/v2)
J=android/app/src/main/java/org/golang/app
LIB=android/app/src/main/jniLibs/arm64-v8a
mkdir -p "$J" "$LIB"
rm -f "$J"/*.java
cp "$FYNE/internal/driver/mobile/app/GoNativeActivity.java" "$FYNE/internal/driver/mobile/app/FyneNotificationReceiver.java" "$J/"
CC="$(cygpath -w "$ANDROID_HOME/ndk/27.3.13750724/toolchains/llvm/prebuilt/windows-x86_64/bin/aarch64-linux-android30-clang.cmd")"
CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC="$CC" go build -buildmode=c-shared -tags migrated_fynedo \
  -ldflags "-s -w -extldflags=-Wl,-z,max-page-size=16384" -o "$LIB/libmangareader.so" .
rm -f "$LIB/libmangareader.h"
cd android
JAVA_HOME="/c/Program Files/Android/Android Studio/jbr" ./gradlew assembleDebug --console=plain -q
