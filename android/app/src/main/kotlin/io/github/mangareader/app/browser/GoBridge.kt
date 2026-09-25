package io.github.mangareader.app.browser

import android.content.Context
import android.os.Handler
import android.os.Looper

// Связь с Go-частью (internal/mobilebrowser). Go открывает браузер Intent'ом
// и вызывает clearData через загрузчик классов приложения; Kotlin сообщает о
// загрузках через nativeOnDownloaded — реализация в libmangareader.so
// (загружена GoNativeActivity через System.loadLibrary).
object GoBridge {
    /** Файл rel (путь относительно папки библиотеки) скачан со страницы page. */
    @JvmStatic
    external fun nativeOnDownloaded(rel: String, page: String)

    /** Очистка данных браузера: kinds — «cookies», «history» через запятую. */
    @JvmStatic
    fun clearData(ctx: Context, kinds: String) {
        val app = ctx.applicationContext
        Handler(Looper.getMainLooper()).post {
            BrowserEngine.init(app)
            BrowserEngine.clearData(kinds.split(",").map { it.trim() }.filter { it.isNotEmpty() })
        }
    }
}
