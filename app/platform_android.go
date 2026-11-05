//go:build android

package app

/*
#include <jni.h>
#include <stdint.h>
#include <stdlib.h>

static JNIEnv *smol_env(JavaVM *vm, int *attached) {
	JNIEnv *env;
	*attached = 0;
	if ((*vm)->GetEnv(vm, (void **)&env, JNI_VERSION_1_6) != JNI_OK) {
		if ((*vm)->AttachCurrentThread(vm, &env, NULL) != JNI_OK) return NULL;
		*attached = 1;
	}
	return env;
}

// view.performHapticFeedback(HapticFeedbackConstants.LONG_PRESS = 0)
static void smol_haptic(uintptr_t vmp, uintptr_t viewp) {
	JavaVM *vm = (JavaVM *)vmp;
	int attached;
	JNIEnv *env = smol_env(vm, &attached);
	if (!env) return;
	jobject view = (jobject)viewp;
	jclass cls = (*env)->GetObjectClass(env, view);
	jmethodID m = (*env)->GetMethodID(env, cls, "performHapticFeedback", "(I)Z");
	if (m) (*env)->CallBooleanMethod(env, view, m, 0);
	if ((*env)->ExceptionCheck(env)) (*env)->ExceptionClear(env);
	if (attached) (*vm)->DetachCurrentThread(vm);
}

// Opens the system share sheet with text/plain content.
static int smol_share(uintptr_t vmp, uintptr_t ctxp, const char *text, const char *title) {
	JavaVM *vm = (JavaVM *)vmp;
	int attached;
	JNIEnv *env = smol_env(vm, &attached);
	if (!env) return 0;
	jobject ctx = (jobject)ctxp;
	int ok = 0;
	jclass ic = (*env)->FindClass(env, "android/content/Intent");
	if (ic) {
		jobject intent = (*env)->NewObject(env, ic, (*env)->GetMethodID(env, ic, "<init>", "(Ljava/lang/String;)V"),
			(*env)->NewStringUTF(env, "android.intent.action.SEND"));
		(*env)->CallObjectMethod(env, intent, (*env)->GetMethodID(env, ic, "setType", "(Ljava/lang/String;)Landroid/content/Intent;"),
			(*env)->NewStringUTF(env, "text/plain"));
		(*env)->CallObjectMethod(env, intent, (*env)->GetMethodID(env, ic, "putExtra", "(Ljava/lang/String;Ljava/lang/String;)Landroid/content/Intent;"),
			(*env)->NewStringUTF(env, "android.intent.extra.TEXT"), (*env)->NewStringUTF(env, text));
		jobject chooser = (*env)->CallStaticObjectMethod(env, ic,
			(*env)->GetStaticMethodID(env, ic, "createChooser", "(Landroid/content/Intent;Ljava/lang/CharSequence;)Landroid/content/Intent;"),
			intent, (*env)->NewStringUTF(env, title));
		(*env)->CallObjectMethod(env, chooser, (*env)->GetMethodID(env, ic, "addFlags", "(I)Landroid/content/Intent;"), 0x10000000); // NEW_TASK
		jclass cc = (*env)->GetObjectClass(env, ctx);
		(*env)->CallVoidMethod(env, ctx, (*env)->GetMethodID(env, cc, "startActivity", "(Landroid/content/Intent;)V"), chooser);
		ok = !(*env)->ExceptionCheck(env);
	}
	if ((*env)->ExceptionCheck(env)) (*env)->ExceptionClear(env);
	if (attached) (*vm)->DetachCurrentThread(vm);
	return ok;
}
*/
import "C"

import (
	"runtime"
	"sync/atomic"
	"unsafe"

	gioapp "gioui.org/app"
)

var androidView atomic.Uintptr // current android.view.View global ref

// SetAndroidView records the window's View (from app.AndroidViewEvent).
func SetAndroidView(v uintptr) { androidView.Store(v) }

// Haptic gives a short "long press" vibration (no permission needed).
func Haptic() {
	v := androidView.Load()
	if v == 0 {
		return
	}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		C.smol_haptic(C.uintptr_t(gioapp.JavaVM()), C.uintptr_t(v))
	}()
}

// CanShare reports whether ShareText opens a system share sheet.
func CanShare() bool { return true }

// ShareText opens the Android share sheet.
func ShareText(text string) bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ct, cl := C.CString(text), C.CString("Share")
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cl))
	return C.smol_share(C.uintptr_t(gioapp.JavaVM()), C.uintptr_t(gioapp.AppContext()), ct, cl) != 0
}
