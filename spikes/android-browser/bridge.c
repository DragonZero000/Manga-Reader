#include <jni.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static char *exc(JNIEnv *env) {
	if (!(*env)->ExceptionCheck(env)) return NULL;
	(*env)->ExceptionDescribe(env);
	(*env)->ExceptionClear(env);
	return strdup("исключение Java (см. logcat)");
}

// openBrowser: Intent на BrowserActivity по имени класса (без FindClass
// классов приложения — из потока Go виден только системный загрузчик).
char *openBrowser(uintptr_t envp, uintptr_t ctxp, const char *url, const char *tree) {
	JNIEnv *env = (JNIEnv *)envp;
	jobject ctx = (jobject)ctxp;
	jclass intentCls = (*env)->FindClass(env, "android/content/Intent");
	jobject intent = (*env)->NewObject(env, intentCls, (*env)->GetMethodID(env, intentCls, "<init>", "()V"));
	jmethodID setClassName = (*env)->GetMethodID(env, intentCls, "setClassName", "(Landroid/content/Context;Ljava/lang/String;)Landroid/content/Intent;");
	(*env)->CallObjectMethod(env, intent, setClassName, ctx, (*env)->NewStringUTF(env, "io.github.mangareader.spike.browser.BrowserActivity"));
	jmethodID putExtra = (*env)->GetMethodID(env, intentCls, "putExtra", "(Ljava/lang/String;Ljava/lang/String;)Landroid/content/Intent;");
	(*env)->CallObjectMethod(env, intent, putExtra, (*env)->NewStringUTF(env, "url"), (*env)->NewStringUTF(env, url));
	(*env)->CallObjectMethod(env, intent, putExtra, (*env)->NewStringUTF(env, "tree"), (*env)->NewStringUTF(env, tree));
	jmethodID addFlags = (*env)->GetMethodID(env, intentCls, "addFlags", "(I)Landroid/content/Intent;");
	(*env)->CallObjectMethod(env, intent, addFlags, (jint)0x00020000); // FLAG_ACTIVITY_REORDER_TO_FRONT
	jclass ctxCls = (*env)->GetObjectClass(env, ctx);
	jmethodID start = (*env)->GetMethodID(env, ctxCls, "startActivity", "(Landroid/content/Intent;)V");
	(*env)->CallVoidMethod(env, ctx, start, intent);
	return exc(env);
}

// Kotlin → Go: GoBridge.nativeOnDownloaded(name, page). Библиотеку загрузил
// System.loadLibrary (GoNativeActivity), поэтому JNI находит этот символ.
extern void goOnDownloaded(char *name, char *page);

JNIEXPORT void JNICALL
Java_io_github_mangareader_spike_browser_GoBridge_nativeOnDownloaded(JNIEnv *env, jclass cls, jstring name, jstring page) {
	const char *n = (*env)->GetStringUTFChars(env, name, NULL);
	const char *p = (*env)->GetStringUTFChars(env, page, NULL);
	char *nc = strdup(n), *pc = strdup(p);
	(*env)->ReleaseStringUTFChars(env, name, n);
	(*env)->ReleaseStringUTFChars(env, page, p);
	goOnDownloaded(nc, pc);
	free(nc);
	free(pc);
}
