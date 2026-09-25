plugins {
    id("com.android.application") version "9.4.1" apply false
    // встроенный в AGP Kotlin — 2.2; GeckoView требует стандартную библиотеку 2.4
    id("org.jetbrains.kotlin.android") version "2.4.10" apply false
}
