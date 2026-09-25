//go:build android

#include <jni.h>
// JNI-вызовы Storage Access Framework (проверены спайком saf-android, см. openspec/changes/archive/2026-09-24-storage-location).
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

// Все функции возвращают NULL/-1 при ошибке и кладут текст исключения в *err
// (освобождает вызывающий через free).

static char *dupjstr(JNIEnv *env, jstring s) {
	if (s == NULL) return NULL;
	const char *utf = (*env)->GetStringUTFChars(env, s, NULL);
	char *r = strdup(utf);
	(*env)->ReleaseStringUTFChars(env, s, utf);
	return r;
}

// takeException очищает исключение и возвращает его текст (или NULL).
static char *takeException(JNIEnv *env) {
	jthrowable ex = (*env)->ExceptionOccurred(env);
	if (ex == NULL) return NULL;
	(*env)->ExceptionClear(env);
	jclass cls = (*env)->GetObjectClass(env, ex);
	jmethodID toString = (*env)->GetMethodID(env, cls, "toString", "()Ljava/lang/String;");
	jstring msg = (jstring)(*env)->CallObjectMethod(env, ex, toString);
	if ((*env)->ExceptionCheck(env)) {
		(*env)->ExceptionClear(env);
		return strdup("exception");
	}
	return dupjstr(env, msg);
}

static jobject resolver(JNIEnv *env, jobject ctx) {
	jclass cls = (*env)->GetObjectClass(env, ctx);
	jmethodID m = (*env)->GetMethodID(env, cls, "getContentResolver", "()Landroid/content/ContentResolver;");
	return (*env)->CallObjectMethod(env, ctx, m);
}

static jobject parseUri(JNIEnv *env, const char *s) {
	jclass uriCls = (*env)->FindClass(env, "android/net/Uri");
	jmethodID parse = (*env)->GetStaticMethodID(env, uriCls, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
	return (*env)->CallStaticObjectMethod(env, uriCls, parse, (*env)->NewStringUTF(env, s));
}

// safTakePersistable — takePersistableUriPermission(uri, FLAG_GRANT_READ_URI_PERMISSION |
// FLAG_GRANT_WRITE_URI_PERMISSION): запись нужна встроенному браузеру для загрузок.
int safTakePersistable(uintptr_t jenv, uintptr_t jctx, const char *uri, char **err) {
	JNIEnv *env = (JNIEnv *)jenv;
	jobject cr = resolver(env, (jobject)jctx);
	jobject u = parseUri(env, uri);
	jclass crCls = (*env)->GetObjectClass(env, cr);
	jmethodID take = (*env)->GetMethodID(env, crCls, "takePersistableUriPermission", "(Landroid/net/Uri;I)V");
	(*env)->CallVoidMethod(env, cr, take, u, 3);
	*err = takeException(env);
	return *err == NULL ? 0 : -1;
}

// safIsPersisted — есть ли среди getPersistedUriPermissions() uri с правом
// чтения (write == 0) или записи (write != 0).
int safIsPersisted(uintptr_t jenv, uintptr_t jctx, const char *uri, int write) {
	JNIEnv *env = (JNIEnv *)jenv;
	jobject cr = resolver(env, (jobject)jctx);
	jclass crCls = (*env)->GetObjectClass(env, cr);
	jmethodID get = (*env)->GetMethodID(env, crCls, "getPersistedUriPermissions", "()Ljava/util/List;");
	jobject list = (*env)->CallObjectMethod(env, cr, get);
	if ((*env)->ExceptionCheck(env)) { (*env)->ExceptionClear(env); return 0; }
	jclass listCls = (*env)->FindClass(env, "java/util/List");
	jmethodID size = (*env)->GetMethodID(env, listCls, "size", "()I");
	jmethodID at = (*env)->GetMethodID(env, listCls, "get", "(I)Ljava/lang/Object;");
	jclass permCls = (*env)->FindClass(env, "android/content/UriPermission");
	jmethodID getUri = (*env)->GetMethodID(env, permCls, "getUri", "()Landroid/net/Uri;");
	jmethodID isRead = (*env)->GetMethodID(env, permCls, write ? "isWritePermission" : "isReadPermission", "()Z");
	jclass uriCls = (*env)->FindClass(env, "android/net/Uri");
	jmethodID toString = (*env)->GetMethodID(env, uriCls, "toString", "()Ljava/lang/String;");
	int n = (*env)->CallIntMethod(env, list, size);
	int found = 0;
	for (int i = 0; i < n && !found; i++) {
		(*env)->PushLocalFrame(env, 8);
		jobject p = (*env)->CallObjectMethod(env, list, at, i);
		jobject pu = (*env)->CallObjectMethod(env, p, getUri);
		char *s = dupjstr(env, (jstring)(*env)->CallObjectMethod(env, pu, toString));
		if (s != NULL && strcmp(s, uri) == 0 && (*env)->CallBooleanMethod(env, p, isRead)) found = 1;
		free(s);
		(*env)->PopLocalFrame(env, NULL);
	}
	return found;
}

// safTreeDocId — DocumentsContract.getTreeDocumentId(uri).
char *safTreeDocId(uintptr_t jenv, const char *uri, char **err) {
	JNIEnv *env = (JNIEnv *)jenv;
	jobject u = parseUri(env, uri);
	jclass dc = (*env)->FindClass(env, "android/provider/DocumentsContract");
	jmethodID m = (*env)->GetStaticMethodID(env, dc, "getTreeDocumentId", "(Landroid/net/Uri;)Ljava/lang/String;");
	jstring id = (jstring)(*env)->CallStaticObjectMethod(env, dc, m, u);
	*err = takeException(env);
	if (*err != NULL) return NULL;
	return dupjstr(env, id);
}

// safListChildren — дети документа docId дерева tree: записи через \x1e,
// поля через \x1f: documentId, имя, mime, размер, время изменения (мс).
char *safListChildren(uintptr_t jenv, uintptr_t jctx, const char *tree, const char *docId, char **err) {
	JNIEnv *env = (JNIEnv *)jenv;
	jobject cr = resolver(env, (jobject)jctx);
	jobject treeUri = parseUri(env, tree);
	jclass dc = (*env)->FindClass(env, "android/provider/DocumentsContract");
	jmethodID build = (*env)->GetStaticMethodID(env, dc, "buildChildDocumentsUriUsingTree",
		"(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");
	jobject children = (*env)->CallStaticObjectMethod(env, dc, build, treeUri, (*env)->NewStringUTF(env, docId));
	if ((*err = takeException(env)) != NULL) return NULL;

	jclass strCls = (*env)->FindClass(env, "java/lang/String");
	const char *cols[] = {"document_id", "_display_name", "mime_type", "_size", "last_modified"};
	jobjectArray proj = (*env)->NewObjectArray(env, 5, strCls, NULL);
	for (int i = 0; i < 5; i++) (*env)->SetObjectArrayElement(env, proj, i, (*env)->NewStringUTF(env, cols[i]));

	jclass crCls = (*env)->GetObjectClass(env, cr);
	jmethodID query = (*env)->GetMethodID(env, crCls, "query",
		"(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
	jobject cur = (*env)->CallObjectMethod(env, cr, query, children, proj, NULL, NULL, NULL);
	if ((*err = takeException(env)) != NULL) return NULL;
	if (cur == NULL) { *err = strdup("query вернул null"); return NULL; }

	jclass curCls = (*env)->FindClass(env, "android/database/Cursor");
	jmethodID next = (*env)->GetMethodID(env, curCls, "moveToNext", "()Z");
	jmethodID getString = (*env)->GetMethodID(env, curCls, "getString", "(I)Ljava/lang/String;");
	jmethodID getLong = (*env)->GetMethodID(env, curCls, "getLong", "(I)J");
	jmethodID closeM = (*env)->GetMethodID(env, curCls, "close", "()V");

	size_t cap = 4096, len = 0;
	char *out = malloc(cap);
	out[0] = 0;
	while ((*env)->CallBooleanMethod(env, cur, next)) {
		(*env)->PushLocalFrame(env, 8);
		char *id = dupjstr(env, (jstring)(*env)->CallObjectMethod(env, cur, getString, 0));
		char *name = dupjstr(env, (jstring)(*env)->CallObjectMethod(env, cur, getString, 1));
		char *mime = dupjstr(env, (jstring)(*env)->CallObjectMethod(env, cur, getString, 2));
		jlong size = (*env)->CallLongMethod(env, cur, getLong, 3);
		jlong mod = (*env)->CallLongMethod(env, cur, getLong, 4);
		char num[64];
		snprintf(num, sizeof num, "%lld\x1f%lld", (long long)size, (long long)mod);
		size_t need = strlen(id ? id : "") + strlen(name ? name : "") + strlen(mime ? mime : "") + strlen(num) + 8;
		if (len + need >= cap) { while (len + need >= cap) cap *= 2; out = realloc(out, cap); }
		len += sprintf(out + len, "%s\x1f%s\x1f%s\x1f%s\x1e", id ? id : "", name ? name : "", mime ? mime : "", num);
		free(id); free(name); free(mime);
		(*env)->PopLocalFrame(env, NULL);
	}
	(*env)->CallVoidMethod(env, cur, closeM);
	if ((*err = takeException(env)) != NULL) { free(out); return NULL; }
	return out;
}

// safOpenFd — openFileDescriptor(buildDocumentUriUsingTree(tree, docId), "r").detachFd().
int safOpenFd(uintptr_t jenv, uintptr_t jctx, const char *tree, const char *docId, char **err) {
	JNIEnv *env = (JNIEnv *)jenv;
	jobject cr = resolver(env, (jobject)jctx);
	jobject treeUri = parseUri(env, tree);
	jclass dc = (*env)->FindClass(env, "android/provider/DocumentsContract");
	jmethodID build = (*env)->GetStaticMethodID(env, dc, "buildDocumentUriUsingTree",
		"(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");
	jobject doc = (*env)->CallStaticObjectMethod(env, dc, build, treeUri, (*env)->NewStringUTF(env, docId));
	if ((*err = takeException(env)) != NULL) return -1;
	jclass crCls = (*env)->GetObjectClass(env, cr);
	jmethodID open = (*env)->GetMethodID(env, crCls, "openFileDescriptor",
		"(Landroid/net/Uri;Ljava/lang/String;)Landroid/os/ParcelFileDescriptor;");
	jobject pfd = (*env)->CallObjectMethod(env, cr, open, doc, (*env)->NewStringUTF(env, "r"));
	if ((*err = takeException(env)) != NULL) return -1;
	jclass pfdCls = (*env)->GetObjectClass(env, pfd);
	jmethodID detach = (*env)->GetMethodID(env, pfdCls, "detachFd", "()I");
	int fd = (*env)->CallIntMethod(env, pfd, detach);
	if ((*err = takeException(env)) != NULL) return -1;
	return fd;
}
