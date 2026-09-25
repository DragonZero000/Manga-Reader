//go:build android

#include <jni.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

// Класс Kotlin-части браузера и экран браузера.
#define BRIDGE_CLASS "io.github.mangareader.app.browser.GoBridge"
#define BROWSER_ACTIVITY "io.github.mangareader.app.browser.BrowserActivity"

static char *exc(JNIEnv *env) {
	if (!(*env)->ExceptionCheck(env)) return NULL;
	(*env)->ExceptionDescribe(env);
	(*env)->ExceptionClear(env);
	return strdup("исключение Java (подробности в logcat)");
}

static void putExtra(JNIEnv *env, jobject intent, jmethodID m, const char *k, const char *v) {
	jstring jk = (*env)->NewStringUTF(env, k);
	jstring jv = (*env)->NewStringUTF(env, v);
	(*env)->CallObjectMethod(env, intent, m, jk, jv);
	(*env)->DeleteLocalRef(env, jk);
	(*env)->DeleteLocalRef(env, jv);
}

// mbOpen: Intent на BrowserActivity по имени класса — из потока Go классы
// приложения через FindClass не видны, а setClassName их не требует.
char *mbOpen(uintptr_t envp, uintptr_t ctxp, const char *url, const char *tree, const char *settings) {
	JNIEnv *env = (JNIEnv *)envp;
	jobject ctx = (jobject)ctxp;
	jclass intentCls = (*env)->FindClass(env, "android/content/Intent");
	jobject intent = (*env)->NewObject(env, intentCls, (*env)->GetMethodID(env, intentCls, "<init>", "()V"));
	jmethodID setClassName = (*env)->GetMethodID(env, intentCls, "setClassName",
		"(Landroid/content/Context;Ljava/lang/String;)Landroid/content/Intent;");
	jstring cls = (*env)->NewStringUTF(env, BROWSER_ACTIVITY);
	(*env)->CallObjectMethod(env, intent, setClassName, ctx, cls);
	jmethodID put = (*env)->GetMethodID(env, intentCls, "putExtra",
		"(Ljava/lang/String;Ljava/lang/String;)Landroid/content/Intent;");
	putExtra(env, intent, put, "url", url);
	putExtra(env, intent, put, "tree", tree);
	putExtra(env, intent, put, "settings", settings);
	jmethodID addFlags = (*env)->GetMethodID(env, intentCls, "addFlags", "(I)Landroid/content/Intent;");
	(*env)->CallObjectMethod(env, intent, addFlags, (jint)0x00020000); // FLAG_ACTIVITY_REORDER_TO_FRONT
	jclass ctxCls = (*env)->GetObjectClass(env, ctx);
	jmethodID start = (*env)->GetMethodID(env, ctxCls, "startActivity", "(Landroid/content/Intent;)V");
	(*env)->CallVoidMethod(env, ctx, start, intent);
	return exc(env);
}

// bridgeClass — GoBridge через загрузчик классов приложения (ctx.getClassLoader()).
static jclass bridgeClass(JNIEnv *env, jobject ctx) {
	jclass ctxCls = (*env)->GetObjectClass(env, ctx);
	jmethodID getCL = (*env)->GetMethodID(env, ctxCls, "getClassLoader", "()Ljava/lang/ClassLoader;");
	jobject cl = (*env)->CallObjectMethod(env, ctx, getCL);
	jclass clCls = (*env)->FindClass(env, "java/lang/ClassLoader");
	jmethodID load = (*env)->GetMethodID(env, clCls, "loadClass", "(Ljava/lang/String;)Ljava/lang/Class;");
	jstring name = (*env)->NewStringUTF(env, BRIDGE_CLASS);
	return (jclass)(*env)->CallObjectMethod(env, cl, load, name);
}

// mbClear: GoBridge.clearData(context, "cookies,history").
char *mbClear(uintptr_t envp, uintptr_t ctxp, const char *kinds) {
	JNIEnv *env = (JNIEnv *)envp;
	jobject ctx = (jobject)ctxp;
	jclass bridge = bridgeClass(env, ctx);
	if (bridge == NULL || (*env)->ExceptionCheck(env)) return exc(env);
	jmethodID clear = (*env)->GetStaticMethodID(env, bridge, "clearData", "(Landroid/content/Context;Ljava/lang/String;)V");
	if (clear == NULL) return exc(env);
	(*env)->CallStaticVoidMethod(env, bridge, clear, ctx, (*env)->NewStringUTF(env, kinds));
	return exc(env);
}

// Kotlin → Go: GoBridge.nativeOnDownloaded(rel, page). Библиотеку загрузил
// System.loadLibrary (GoNativeActivity), поэтому JNI находит этот символ.
// Строки передаются в UTF-16 (GetStringUTFChars отдаёт модифицированный UTF-8).
extern void goMbDownloaded(uint16_t *rel, int relLen, uint16_t *page, int pageLen);

JNIEXPORT void JNICALL
Java_io_github_mangareader_app_browser_GoBridge_nativeOnDownloaded(JNIEnv *env, jclass cls, jstring rel, jstring page) {
	jsize rn = (*env)->GetStringLength(env, rel), pn = (*env)->GetStringLength(env, page);
	uint16_t *r = malloc((rn + 1) * sizeof(uint16_t)), *p = malloc((pn + 1) * sizeof(uint16_t));
	(*env)->GetStringRegion(env, rel, 0, rn, (jchar *)r);
	(*env)->GetStringRegion(env, page, 0, pn, (jchar *)p);
	goMbDownloaded(r, rn, p, pn);
	free(r);
	free(p);
}
