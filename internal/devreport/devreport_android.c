//go:build android

#include <jni.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

// Kotlin-часть: io.github.mangareader.app.DevReport.
#define REPORT_CLASS "io.github.mangareader.app.DevReport"

static char *exc(JNIEnv *env) {
	if (!(*env)->ExceptionCheck(env)) return NULL;
	(*env)->ExceptionDescribe(env);
	(*env)->ExceptionClear(env);
	return strdup("Java exception (see logcat)");
}

// reportClass — класс приложения через загрузчик классов Activity: из
// потока Go FindClass его не видит.
static jclass reportClass(JNIEnv *env, jobject ctx) {
	jclass ctxCls = (*env)->GetObjectClass(env, ctx);
	jmethodID getCL = (*env)->GetMethodID(env, ctxCls, "getClassLoader", "()Ljava/lang/ClassLoader;");
	jobject cl = (*env)->CallObjectMethod(env, ctx, getCL);
	jclass clCls = (*env)->FindClass(env, "java/lang/ClassLoader");
	jmethodID load = (*env)->GetMethodID(env, clCls, "loadClass", "(Ljava/lang/String;)Ljava/lang/Class;");
	jstring name = (*env)->NewStringUTF(env, REPORT_CLASS);
	jclass cls = (jclass)(*env)->CallObjectMethod(env, cl, load, name);
	(*env)->DeleteLocalRef(env, name);
	return cls;
}

// drSave: DevReport.save(ctx, tree, name, text); текст ошибки Kotlin или NULL.
char *drSave(uintptr_t envp, uintptr_t ctxp, const char *tree, const char *name, const char *text) {
	JNIEnv *env = (JNIEnv *)envp;
	jobject ctx = (jobject)ctxp;
	jclass cls = reportClass(env, ctx);
	if (cls == NULL || (*env)->ExceptionCheck(env)) return exc(env);
	jmethodID save = (*env)->GetStaticMethodID(env, cls, "save",
		"(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;");
	if (save == NULL) return exc(env);
	jstring jt = (*env)->NewStringUTF(env, tree);
	jstring jn = (*env)->NewStringUTF(env, name);
	jstring jx = (*env)->NewStringUTF(env, text);
	jstring res = (jstring)(*env)->CallStaticObjectMethod(env, cls, save, ctx, jt, jn, jx);
	(*env)->DeleteLocalRef(env, jt);
	(*env)->DeleteLocalRef(env, jn);
	(*env)->DeleteLocalRef(env, jx);
	char *e = exc(env);
	if (e != NULL) return e;
	if (res == NULL) return NULL;
	const char *s = (*env)->GetStringUTFChars(env, res, NULL);
	char *out = s[0] ? strdup(s) : NULL;
	(*env)->ReleaseStringUTFChars(env, res, s);
	return out;
}

// drShare: DevReport.share(ctx, subject, text).
char *drShare(uintptr_t envp, uintptr_t ctxp, const char *subject, const char *text) {
	JNIEnv *env = (JNIEnv *)envp;
	jobject ctx = (jobject)ctxp;
	jclass cls = reportClass(env, ctx);
	if (cls == NULL || (*env)->ExceptionCheck(env)) return exc(env);
	jmethodID share = (*env)->GetStaticMethodID(env, cls, "share",
		"(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;)V");
	if (share == NULL) return exc(env);
	jstring js = (*env)->NewStringUTF(env, subject);
	jstring jx = (*env)->NewStringUTF(env, text);
	(*env)->CallStaticVoidMethod(env, cls, share, ctx, js, jx);
	(*env)->DeleteLocalRef(env, js);
	(*env)->DeleteLocalRef(env, jx);
	return exc(env);
}
