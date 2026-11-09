# SmolChat for Android

**Download once. Run locally. Chat anywhere.**

SmolChat is a private, fully offline chat app for Android, part of the
**SmolWorks** ecosystem. It runs Hugging Face's Smol family of language
models directly on your phone with [llama.cpp](https://github.com/ggml-org/llama.cpp).
The app is written in Go with [Gio](https://gioui.org) for the whole UI.

* No model weights in the APK. The app launches and works with no network.
* Download the models you want from Hugging Face, verified with SHA-256.
* Inference runs on-device. Chats are stored locally in SQLite and never sent anywhere.

## Features

| | |
|---|---|
| **Design** | Mobile port of SmolChat for Mac: animated mascot splash, accent-tinted glow backdrop, System/Light/Dark themes with six accent colours, glass composer, quick-action chips |
| **Android-native feel** | Back stack (Back and the system back gesture return to the previous page; overlays close first), swipe right on a page to go back, haptics on long-press |
| **Text** | Long-press a message for Copy / Select text / Share / Regenerate / Edit & resend; touch text selection with handles; Paste / Select all / Cut / Copy in the composer; emoji everywhere (bundled Noto Emoji) |
| **Chat** | ChatGPT-style mobile UI, streaming replies, Markdown with code highlighting, copy, regenerate, stop |
| **History** | SQLite conversations with search, date groups, rename and delete; survives restarts |
| **Model Library** | Verified Smol catalog by category, device-fit hints, download progress/speed/ETA, pause, resume, cancel, delete |
| **Downloads** | Resumable HTTP range downloads, SHA-256 verification, storage checks, retries; an Android foreground service keeps them alive in the background |
| **Runtime** | Lazy loading, live switching without restart, one model in RAM at a time, KV-cache reuse across turns |
| **Model selection** | **Auto** picks a model per message from complexity, device RAM, thermal state, the loaded model and your Speed/Balanced/Quality preference. **Manual** pins a model for the conversation |
| **Hardware aware** | RAM, storage, CPU features, Android version and thermal status feed recommendations and routing |

## Supported models

Every repository id, file name, size and SHA-256 below was checked against the
Hugging Face API. Files are pinned to a commit, and `TestCatalogMatchesHub`
re-verifies them.

| Category | Model | Source | Status |
|---|---|---|---|
| Text | SmolLM2 135M / 360M / 1.7B Instruct | `bartowski/…135M…-GGUF`¹, `HuggingFaceTB/SmolLM2-{360M,1.7B}-Instruct-GGUF` | ✅ runs (llama.cpp) |
| Text | SmolLM3 3B (advanced) | `ggml-org/SmolLM3-3B-GGUF` | ✅ runs (llama.cpp) |
| Text | SmolLM 135M / 360M / 1.7B Instruct v0.2 | `HuggingFaceTB/smollm-*-instruct-v0.2-Q8_0-GGUF` | ✅ runs (llama.cpp) |
| Vision | SmolVLM / SmolVLM2 256M, 500M, 2.2B | `ggml-org/SmolVLM*-GGUF` (+ mmproj) | 🔜 listed, not downloadable: needs libmtmd and image input |
| Document | SmolDocling 256M preview | `docling-project/SmolDocling-256M-preview` | 🔜 no official GGUF, no document workflow yet |
| Embeddings | ColSmol 256M / 500M | `vidore/colSmol-*` | 🔜 ColPali models, no llama.cpp runtime |

¹ HuggingFaceTB publishes no GGUF for SmolLM2-135M-Instruct, so SmolChat uses
bartowski's widely used conversion and labels it as a community conversion.

Unsupported models are shown, but they're clearly marked and can't be
downloaded. SmolChat never pretends they work.

## Building

Requirements: Go 1.24+, Android SDK (platform 35, build-tools), Android NDK
r27+, JDK 17+, CMake and Ninja.

```bash
git clone --recurse-submodules --shallow-submodules https://github.com/smolworks/smolchat-android
cd smolchat-android
go install gioui.org/cmd/gogio@latest     # gogio >= Nov 2026 (foreground-service support)

native/build.sh android                  # llama.cpp static libs for arm64-v8a
./build.sh                               # -> build/SmolChat-debug.apk
adb install -r build/SmolChat-debug.apk
```

`build.sh` runs `gogio -target android -arch arm64 -minsdk 28
-appid ai.smolworks.smolchat`. llama.cpp is linked statically into
`libgio.so`, so the APK has a single native library.

**Release:** create a keystore once, then build a signed APK:

```bash
keytool -genkey -v -keystore smolchat-release.keystore -alias smolchat -keyalg RSA -keysize 4096 -validity 10000
SMOLCHAT_KEYSTORE=smolchat-release.keystore SMOLCHAT_KEYSTORE_PASS=… BUILD_NUMBER=2 ./build.sh release
```

### Desktop development build

The same code runs on macOS and Linux, which is handy for UI work:

```bash
native/build.sh host
go run .                                  # data in ~/Library/Application Support/SmolChat
SMOLCHAT_DATA=/tmp/smolchat go run .      # or a scratch directory
```

## Testing

```bash
go test ./...                                         # unit tests (db, downloader, router, runtime, markdown UTF-8)
SMOLCHAT_ONLINE=1 go test ./tests -run CatalogMatchesHub   # verify the catalog against huggingface.co
SMOLCHAT_TEST_MODEL=SmolLM2-135M-Instruct-Q8_0.gguf go test ./native/llama ./ui -v
SMOLCHAT_SHOTS=/tmp/shots go test ./ui -run Screens   # headless screenshots of every screen
```

`ui/screens_test.go` renders every screen off-screen with Gio's headless GPU
backend. With a test model it also runs a real conversation through the whole
stack: router, runtime, llama.cpp, SQLite and Markdown rendering.

### Verified on Android

Tested on an Android 15 (API 35) arm64 emulator with 3 GB RAM and 4 cores:

* The app launches with no models and recommends one for the device.
* Downloads from Hugging Face show speed and ETA and pass SHA-256 verification. They continue in the background under the foreground service, and after a force-kill mid-download they resume from the partial file.
* A downloaded model is usable immediately, with no restart.
* llama.cpp inference runs on-device and streams at about 11–15 tok/s, including in airplane mode. Stop works.
* Live switching between SmolLM2 360M and 135M (Auto and Manual).
* Chats persist across a force-kill.
* Keyboard insets, swipe and Back navigation, and thermal status via JNI all work.

Not yet tested on a physical phone. Tiny models (135M/360M) sometimes echo
short prompts. That reproduces with the same weights on desktop, so it comes
from the model, not the engine.

## Architecture

```
main.go              window, lifecycle events, foreground-service hook
app/                 platform-independent core (no UI imports)
  app.go             App: wiring, notices, lifecycle, library actions
  chat.go            conversation controller: send, stream, stop, regenerate
  database.go        SQLite store (conversations, messages, metadata)
  models.go          verified catalog, install manifests, model manager
  downloader.go      resumable, verified Hugging Face downloads
  inference.go       InferenceEngine interface + Runtime (single-model lifecycle)
  router.go          automatic model selection
  hardware*.go       RAM/storage/CPU/thermal detection (JNI on Android)
  config.go          storage layout and settings
native/llama/        cgo bridge to llama.cpp (implements app.InferenceEngine)
ui/                  Gio interface: splash, chat, drawer, library, settings, markdown
tests/               unit tests for the core
```

The UI reads snapshots from `app.App` each frame and calls its methods. Slow
work (downloads, loading, generation) runs on goroutines that call
`Window.Invalidate`. Nothing blocks the Gio event loop.

**Storage** (app-private, from `Context.getFilesDir()` via `gioui.org/app.DataDir`):

```
files/models/<id>/*.gguf      weights
files/meta/                   install manifests
files/db/smolchat.db          conversations
files/settings.json           preferences
files/logs/                   app log
files/tmp/downloads/          partial downloads (.part)
```

**Android lifecycle:** downloads run under a `specialUse` foreground service
(via Gio's `app.StartForeground`) so Android doesn't kill them in the
background. If the process dies anyway, the partial file is kept and the
library offers **Resume**, which continues with an HTTP Range request and
re-hashes only what's already on disk. A streaming reply is saved to the
database every 2 s and when the app is backgrounded.

**Permissions:** `INTERNET` (model downloads only), `FOREGROUND_SERVICE`,
`FOREGROUND_SERVICE_SPECIAL_USE`, `POST_NOTIFICATIONS` (download
notification). Nothing else.

## Privacy

SmolChat contacts only `huggingface.co`, and only to download models you choose
or to verify their metadata. It has no analytics and no cloud inference.

## Licenses

SmolChat's code is MIT-licensed (see [LICENSE](LICENSE)). Models are
distributed by their publishers under their own licenses (Apache-2.0 for
SmolLM/SmolVLM, CDLA-Permissive-2.0 for SmolDocling, MIT for ColSmol). llama.cpp is MIT-licensed. Inter and
JetBrains Mono and Noto Emoji are under the SIL Open Font License
(`assets/fonts/OFL-*.txt`). Brand artwork masters live in `assets/brand/`.
