#!/usr/bin/env bash
# Builds llama.cpp as static libraries that the Go bridge in native/llama links
# against. Usage:
#
#   native/build.sh android   # arm64-v8a, needs ANDROID_NDK_HOME or the SDK's ndk/
#   native/build.sh host      # the current desktop (for development runs/tests)
#
# Output: native/lib/<target>/lib*.a
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
SRC="$ROOT/llama.cpp"
TARGET="${1:-android}"

if [[ ! -f "$SRC/CMakeLists.txt" ]]; then
  echo "llama.cpp sources missing: run 'git submodule update --init --depth 1'" >&2
  exit 1
fi

COMMON=(
  -G Ninja
  -DCMAKE_BUILD_TYPE=Release
  -DCMAKE_POSITION_INDEPENDENT_CODE=ON
  -DBUILD_SHARED_LIBS=OFF
  -DGGML_NATIVE=OFF
  -DGGML_OPENMP=OFF
  -DGGML_CCACHE=OFF
  -DLLAMA_BUILD_COMMON=OFF
  -DLLAMA_BUILD_TESTS=OFF
  -DLLAMA_BUILD_TOOLS=OFF
  -DLLAMA_BUILD_EXAMPLES=OFF
  -DLLAMA_BUILD_SERVER=OFF
  -DLLAMA_BUILD_APP=OFF
  -DLLAMA_OPENSSL=OFF
)

case "$TARGET" in
  android)
    NDK="${ANDROID_NDK_HOME:-}"
    if [[ -z "$NDK" ]]; then
      SDK="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
      NDK="$(ls -d "$SDK"/ndk/* 2>/dev/null | sort -V | tail -1)"
    fi
    [[ -d "$NDK" ]] || { echo "Android NDK not found; set ANDROID_NDK_HOME" >&2; exit 1; }
    OUT="android-arm64"
    # armv8.2-a+dotprod+fp16 covers Cortex-A55/A75 and newer (2018+ phones) and
    # unlocks the fast int8 dot-product kernels. The app checks /proc/cpuinfo
    # for "asimddp" before loading a model, so older CPUs get a clear error
    # instead of SIGILL.
    EXTRA=(
      -DCMAKE_TOOLCHAIN_FILE="$NDK/build/cmake/android.toolchain.cmake"
      -DANDROID_ABI=arm64-v8a
      -DANDROID_PLATFORM=android-28
      -DGGML_CPU_ARM_ARCH=armv8.2-a+dotprod+fp16
      -DGGML_VULKAN=OFF
    )
    ;;
  host)
    OUT="$(go env GOOS)-$(go env GOARCH)"
    EXTRA=(-DGGML_METAL=OFF -DGGML_BLAS=OFF -DGGML_ACCELERATE=OFF)
    ;;
  *)
    echo "unknown target $TARGET (want android|host)" >&2
    exit 1
    ;;
esac

BUILD="$ROOT/build/$OUT"
LIB="$ROOT/lib/$OUT"
cmake -S "$SRC" -B "$BUILD" "${COMMON[@]}" "${EXTRA[@]}"
cmake --build "$BUILD" --target llama -j "$(getconf _NPROCESSORS_ONLN)"

mkdir -p "$LIB"
find "$BUILD" -name 'libllama.a' -o -name 'libggml*.a' | while read -r f; do cp "$f" "$LIB/"; done
ls -la "$LIB"
