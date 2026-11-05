//go:build !android

package app

// SetAndroidView is a no-op off Android.
func SetAndroidView(uintptr) {}

// Haptic is a no-op off Android.
func Haptic() {}

// CanShare is false off Android; the UI copies to the clipboard instead.
func CanShare() bool { return false }

// ShareText is unavailable off Android.
func ShareText(string) bool { return false }
