package io.github.mangareader.spike.browser

// Вызовы Go из Kotlin: реализация — Java_…_GoBridge_nativeOnDownloaded в
// libmangareader.so (загружена GoNativeActivity через System.loadLibrary).
object GoBridge {
    @JvmStatic
    external fun nativeOnDownloaded(name: String, page: String)
}
