package io.github.mangareader.app.browser

import android.content.Context
import android.content.res.Configuration
import android.content.res.Resources
import java.util.Locale

/**
 * Язык интерфейса браузера — язык приложения из настроек Go (поле lang), а не
 * системы. Выбор языка приложения средствами Android есть только с Android 13
 * (минимум приложения — 11), поэтому строки берутся из ресурсов с
 * конфигурацией нужного языка.
 */
object Lang {
    /** Код языка ("en", "ru"); пусто — язык системы. */
    @Volatile
    var code: String = ""

    @Volatile
    private var cached: Pair<String, Resources>? = null

    /** Ресурсы на языке приложения. */
    fun res(ctx: Context): Resources {
        val c = code
        if (c.isBlank()) return ctx.resources
        cached?.let { (k, r) -> if (k == c) return r }
        val cfg = Configuration(ctx.applicationContext.resources.configuration)
        cfg.setLocale(Locale.forLanguageTag(c))
        val r = ctx.applicationContext.createConfigurationContext(cfg).resources
        cached = c to r
        return r
    }

    /** Строка на языке приложения. */
    fun str(ctx: Context, id: Int, vararg args: Any): String = res(ctx).getString(id, *args)

    /** Строка с формой множественного числа для n на языке приложения. */
    fun plural(ctx: Context, id: Int, n: Int, vararg args: Any): String = res(ctx).getQuantityString(id, n, *args)
}
