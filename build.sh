#!/usr/bin/env bash
# Builds the SmolChat APK.
#
#   ./build.sh            debug APK  -> build/SmolChat-debug.apk
#   ./build.sh release    signed APK -> build/SmolChat-<version>.apk
#
# Release signing reads SMOLCHAT_KEYSTORE and SMOLCHAT_KEYSTORE_PASS.
set -euo pipefail
cd "$(dirname "$0")"

VERSION="1.0.0"
BUILD_NUMBER="${BUILD_NUMBER:-1}"
APPID="ai.smolworks.smolchat"
MODE="${1:-debug}"

export ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
if [[ -z "${ANDROID_NDK_HOME:-}" ]]; then
  ANDROID_NDK_HOME="$(ls -d "$ANDROID_HOME"/ndk/* 2>/dev/null | sort -V | tail -1)"
  export ANDROID_NDK_HOME
fi
command -v gogio >/dev/null || go install gioui.org/cmd/gogio@latest

if [[ ! -f native/lib/android-arm64/libllama.a ]]; then
  echo "==> building llama.cpp for Android arm64"
  native/build.sh android
fi

mkdir -p build
ARGS=(-target android -arch arm64 -minsdk 28 -appid "$APPID" -name SmolChat
      -version "$VERSION.$BUILD_NUMBER" -icon assets/icons/icon.png)

case "$MODE" in
  debug)
    OUT=build/SmolChat-debug.apk
    gogio "${ARGS[@]}" -o "$OUT" .
    ;;
  release)
    : "${SMOLCHAT_KEYSTORE:?set SMOLCHAT_KEYSTORE to your release keystore}"
    : "${SMOLCHAT_KEYSTORE_PASS:?set SMOLCHAT_KEYSTORE_PASS}"
    OUT="build/SmolChat-$VERSION.apk"
    gogio "${ARGS[@]}" -signkey "$SMOLCHAT_KEYSTORE" -signpass "$SMOLCHAT_KEYSTORE_PASS" -o "$OUT" .
    ;;
  *)
    echo "usage: $0 [debug|release]" >&2
    exit 1
    ;;
esac
echo "==> $OUT ($(du -h "$OUT" | cut -f1))"
