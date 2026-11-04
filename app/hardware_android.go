//go:build android

package app

/*
#cgo LDFLAGS: -landroid
#include <jni.h>
#include <stdint.h>

// Calls context.getSystemService("power").getCurrentThermalStatus() (API 29+).
// Returns -1 when unavailable.
static int smol_thermal_status(uintptr_t vmp, uintptr_t ctxp) {
	JavaVM *vm = (JavaVM *)vmp;
	jobject ctx = (jobject)ctxp;
	JNIEnv *env;
	int attached = 0;
	if ((*vm)->GetEnv(vm, (void **)&env, JNI_VERSION_1_6) != JNI_OK) {
		if ((*vm)->AttachCurrentThread(vm, &env, NULL) != JNI_OK) return -1;
		attached = 1;
	}
	int result = -1;
	jclass ver = (*env)->FindClass(env, "android/os/Build$VERSION");
	jint sdk = ver ? (*env)->GetStaticIntField(env, ver, (*env)->GetStaticFieldID(env, ver, "SDK_INT", "I")) : 0;
	if (sdk >= 29) {
		jclass cctx = (*env)->GetObjectClass(env, ctx);
		jmethodID gss = (*env)->GetMethodID(env, cctx, "getSystemService", "(Ljava/lang/String;)Ljava/lang/Object;");
		jstring name = (*env)->NewStringUTF(env, "power");
		jobject pm = (*env)->CallObjectMethod(env, ctx, gss, name);
		if (pm && !(*env)->ExceptionCheck(env)) {
			jclass cpm = (*env)->GetObjectClass(env, pm);
			jmethodID m = (*env)->GetMethodID(env, cpm, "getCurrentThermalStatus", "()I");
			if (m) result = (*env)->CallIntMethod(env, pm, m);
		}
		if ((*env)->ExceptionCheck(env)) { (*env)->ExceptionClear(env); result = -1; }
		(*env)->DeleteLocalRef(env, name);
	}
	if ((*env)->ExceptionCheck(env)) (*env)->ExceptionClear(env);
	if (attached) (*vm)->DetachCurrentThread(vm);
	return result;
}
*/
import "C"

import (
	"runtime"

	gioapp "gioui.org/app"
)

func thermalStatus() ThermalStatus {
	runtime.LockOSThread() // JNI attach/detach must stay on one thread
	defer runtime.UnlockOSThread()
	return ThermalStatus(C.smol_thermal_status(C.uintptr_t(gioapp.JavaVM()), C.uintptr_t(gioapp.AppContext())))
}
