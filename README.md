# SmolChat for Android

**Download once. Run locally. Chat anywhere.**

SmolChat is a private, fully offline AI chat app for Android, part of the
**SmolLabs** ecosystem. It runs Hugging Face's Smol family of small language
models directly on your phone with [llama.cpp](https://github.com/ggml-org/llama.cpp).
The whole app, including the UI, is written in Go with [Gio](https://gioui.org).

* No model weights in the APK. The app launches and works with no network.
* Download the models you want from Hugging Face, verified with SHA-256.
* Inference runs on-device. Chats are stored locally in SQLite and never sent anywhere.

**New here and want to contribute?** Jump to [Contributing](#contributing).

## Contents

- [Why SmolChat matters](#why-smolchat-matters)
- [Features](#features)
- [Tech stack](#tech-stack)
- [Supported models](#supported-models)
- [Getting started](#getting-started)
- [Testing](#testing)
- [Architecture](#architecture)
- [Contributing](#contributing)
- [Privacy](#privacy)
- [Licenses](#licenses)

## Why SmolChat matters

Most AI chat apps send every message to a server. That means you need a
connection, your conversations leave your device, and someone else decides
what the model can do and how long it stays available.

SmolChat takes the opposite approach:

* **Privacy by design.** Your messages never leave your phone. There is no
  account, no analytics and no cloud inference.
* **Works anywhere.** Once a model is downloaded, SmolChat works in airplane
  mode, on a train, or in places with poor or expensive connectivity.
* **Runs on ordinary phones.** Small models (135M–3B parameters) and a
  hardware-aware model picker make on-device AI usable on mid-range and
  older devices, not just flagships.
* **Honest and open.** The code is MIT-licensed, every model in the catalog is
  verified against Hugging Face, and models that can't run yet are clearly
  marked instead of failing silently.

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

## Tech stack

| Area | Technology |
|---|---|
| Language | [Go](https://go.dev) 1.26 (with cgo) |
| UI | [Gio](https://gioui.org): immediate-mode, GPU-rendered UI; `gogio` packages the APK |
| Inference | [llama.cpp](https://github.com/ggml-org/llama.cpp) (C/C++), linked statically through a small cgo bridge in `native/llama/` |
| Models | GGUF builds of SmolLM, SmolLM2 and SmolLM3 from [Hugging Face](https://huggingface.co/HuggingFaceTB) |
| Storage | SQLite via [`mattn/go-sqlite3`](https://github.com/mattn/go-sqlite3) |
| Android | Android SDK/NDK, JNI for hardware and thermal info, a foreground service for downloads |
| Build | Bash scripts, CMake and Ninja (for llama.cpp) |

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

## Getting started

### 1. Clone the repository

llama.cpp is a git submodule, so clone with `--recurse-submodules`:

```bash
git clone --recurse-submodules --shallow-submodules https://github.com/smollabs/smolchat-android
cd smolchat-android
```

If you already cloned without it, run `git submodule update --init --depth 1`.

### 2. Run it on your computer (quickest way to start)

The same code runs on **macOS and Linux**, which is the fastest way to work on
the UI and core logic. You don't need Android tooling for this.

Requirements:

* Go 1.26+
* A C/C++ compiler (Xcode Command Line Tools on macOS, `gcc`/`g++` on Linux)
* CMake and Ninja

```bash
native/build.sh host                      # build llama.cpp for your machine (once)
go run .                                  # data in your OS's app data folder
SMOLCHAT_DATA=/tmp/smolchat go run .      # or use a scratch directory
```

Then open the Model Library and download a small model (SmolLM2 135M or 360M)
to start chatting.

> `native/build.sh host` must run before `go run .` or `go test ./...`. Without
> it the build fails with linker errors about missing `llama`/`ggml` libraries.

> **Windows:** run the app under WSL2. For work in `app/` only, the core tests
> also run natively with Go and a MinGW-w64 `gcc`: `go test ./tests`. See
> [CONTRIBUTING](CONTRIBUTING.md#requirements).

### 3. Build the Android app

Additional requirements: Android SDK (platform 35, build-tools), Android NDK
r27+ and JDK 17+. The build script looks for the SDK in `$ANDROID_HOME`
(default `~/Library/Android/sdk`) and uses the newest NDK inside it unless
`ANDROID_NDK_HOME` is set.

```bash
go install gioui.org/cmd/gogio@latest     # gogio >= Nov 2026 (foreground-service support)
native/build.sh android                   # llama.cpp static libs for arm64-v8a
./build.sh                                # -> build/SmolChat-debug.apk
adb install -r build/SmolChat-debug.apk
```

`build.sh` runs `gogio -target android -arch arm64 -minsdk 28
-appid ai.smolworks.smolchat`. llama.cpp is linked statically into
`libgio.so`, so the APK has a single native library. The Android build needs
an arm64 device or emulator with ARMv8.2 dot-product support (most phones from
2018 onwards).

**Release:** create a keystore once, then build a signed APK:

```bash
keytool -genkey -v -keystore smolchat-release.keystore -alias smolchat -keyalg RSA -keysize 4096 -validity 10000
SMOLCHAT_KEYSTORE=smolchat-release.keystore SMOLCHAT_KEYSTORE_PASS=… BUILD_NUMBER=2 ./build.sh release
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
native/llama.cpp/    llama.cpp git submodule (don't edit in place)
ui/                  Gio interface: splash, chat, drawer, library, settings, markdown
tests/               unit tests for the core
assets/              fonts, icons and brand artwork
```

The UI reads snapshots from `app.App` each frame and calls its methods. Slow
work (downloads, loading, generation) runs on goroutines that call
`Window.Invalidate`. Nothing blocks the Gio event loop. See
[`native/README.md`](native/README.md) for how the llama.cpp bridge works.

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

## Contributing

Contributions of every size are welcome, including from people contributing to
open source for the first time. Please read **[CONTRIBUTING.md](CONTRIBUTING.md)**
before you start. It explains how to claim an issue, set up the project and open
a pull request.

### Where to start

1. Get the desktop build running ([Getting started](#getting-started), step 2).
2. Read the [Architecture](#architecture) section to see where things live.
3. Pick an issue that matches your experience (see below), and comment on it to
   ask to be assigned.

### Finding an issue

All available work is tracked in
[GitHub Issues](https://github.com/smollabs/smolchat-android/issues). Each
issue explains the problem, the expected outcome and the files involved, and has
a difficulty label:

| Label | Good for | Examples |
|---|---|---|
| [`level 1`](https://github.com/smollabs/smolchat-android/issues?q=is%3Aissue+is%3Aopen+label%3A%22level+1%22) | First contributions: small, well-scoped changes in one place | renaming, regexp fixes with unit tests |
| [`level 2`](https://github.com/smollabs/smolchat-android/issues?q=is%3Aissue+is%3Aopen+label%3A%22level+2%22) | Some Go experience: you need to understand one subsystem | download retries, search, Model Library UI |
| [`level 3`](https://github.com/smollabs/smolchat-android/issues?q=is%3Aissue+is%3Aopen+label%3A%22level+3%22) | Experienced contributors: concurrency, cgo/llama.cpp, cross-cutting changes | streaming state, context trimming |

You can also filter by
[`good first issue`](https://github.com/smollabs/smolchat-android/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22),
or by area labels such as `component:models`, `component:runtime` and
`area:inference`.

Found a bug or have an idea that isn't tracked yet? Open a new issue and
describe it. See [Reporting bugs](CONTRIBUTING.md#reporting-bugs).

## Privacy

SmolChat contacts only `huggingface.co`, and only to download models you choose
or to verify their metadata. It has no analytics and no cloud inference.

## Licenses

SmolChat's code is MIT-licensed (see [LICENSE](LICENSE)). Models are
distributed by their publishers under their own licenses (Apache-2.0 for
SmolLM/SmolVLM, CDLA-Permissive-2.0 for SmolDocling, MIT for ColSmol). llama.cpp is MIT-licensed. Inter and
JetBrains Mono and Noto Emoji are under the SIL Open Font License
(`assets/fonts/OFL-*.txt`). Brand artwork masters live in `assets/brand/`.
