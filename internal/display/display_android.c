//go:build android

#include <jni.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

// Kotlin-часть: io.github.mangareader.app.DisplayRate.
#define DISPLAY_CLASS "io.github.mangareader.app.DisplayRate"

static char *exc(JNIEnv *env) {
	if (!(*env)->ExceptionCheck(env)) return NULL;
	(*env)->ExceptionDescribe(env);
	(*env)->ExceptionClear(env);
	return strdup("исключение Java (подробности в logcat)");
}

// dispApply: DisplayRate.apply(activity, max60). Класс приложения — через
// загрузчик классов Activity: из потока Go FindClass его не видит.
char *dispApply(uintptr_t envp, uintptr_t ctxp, int max60) {
	JNIEnv *env = (JNIEnv *)envp;
	jobject ctx = (jobject)ctxp;
	jclass ctxCls = (*env)->GetObjectClass(env, ctx);
	jmethodID getCL = (*env)->GetMethodID(env, ctxCls, "getClassLoader", "()Ljava/lang/ClassLoader;");
	jobject cl = (*env)->CallObjectMethod(env, ctx, getCL);
	jclass clCls = (*env)->FindClass(env, "java/lang/ClassLoader");
	jmethodID load = (*env)->GetMethodID(env, clCls, "loadClass", "(Ljava/lang/String;)Ljava/lang/Class;");
	jstring name = (*env)->NewStringUTF(env, DISPLAY_CLASS);
	jclass cls = (jclass)(*env)->CallObjectMethod(env, cl, load, name);
	(*env)->DeleteLocalRef(env, name);
	if (cls == NULL || (*env)->ExceptionCheck(env)) return exc(env);
	jmethodID apply = (*env)->GetStaticMethodID(env, cls, "apply", "(Landroid/content/Context;Z)V");
	if (apply == NULL) return exc(env);
	(*env)->CallStaticVoidMethod(env, cls, apply, ctx, (jboolean)(max60 != 0));
	return exc(env);
}
