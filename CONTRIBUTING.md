# Contributing to SmolChat for Android

Thanks for helping out. This guide covers how to set up the project, the
conventions the code follows, and what a pull request needs before it can be
merged.

## Ground rules

SmolChat's promises to users shape every change:

* **Offline first.** The app must launch and work with no network. The only
  host it may contact is `huggingface.co`, and only to download or verify
  models the user picked. No analytics, telemetry or cloud inference.
* **No weights in the APK.** Models are downloaded at runtime.
* **Honest model support.** A model is downloadable only if it actually runs.
  Anything else is listed and clearly marked as unsupported.
* **Minimal permissions.** Don't add an Android permission without discussing
  it in an issue first.

## Getting set up

Requirements: Go 1.24+, Android SDK (platform 35, build-tools), Android NDK
r27+, JDK 17+, CMake and Ninja.

```bash
git clone --recurse-submodules --shallow-submodules https://github.com/smolworks/smolchat-android
cd smolchat-android
go install gioui.org/cmd/gogio@latest
```

Most UI and core work is fastest on the desktop build, which runs the same code
on macOS and Linux:

```bash
native/build.sh host
SMOLCHAT_DATA=/tmp/smolchat go run .
```

To try it on a device or emulator:

```bash
native/build.sh android
./build.sh
adb install -r build/SmolChat-debug.apk
```

See the [README](README.md#building) for release builds and more detail.

## Project layout

* `app/` is the platform-independent core. It must not import any UI package.
* `ui/` is the Gio interface. It reads snapshots from `app.App` each frame and
  calls its methods.
* `native/llama/` is the cgo bridge to llama.cpp. `native/llama.cpp` is a git
  submodule; don't edit it in place.
* `tests/` holds unit tests for the core.

Never block the Gio event loop. Slow work (downloads, model loading,
generation) runs on a goroutine that calls `Window.Invalidate` when it has
something new to show.

## Making changes

1. Open an issue first for anything bigger than a small fix, so the approach
   can be agreed before you write the code.
2. Create a branch from `main`.
3. Keep each pull request focused on one change.
4. Match the style of the surrounding code. Run `gofmt` and `go vet ./...`
   before committing.
5. Add or update tests for behaviour you change.

### Tests

```bash
go test ./...
```

Depending on what you touched, also run:

```bash
# Catalog changes: verify every entry against the Hugging Face API
SMOLCHAT_ONLINE=1 go test ./tests -run CatalogMatchesHub

# Engine or chat-flow changes: run a real conversation end to end
SMOLCHAT_TEST_MODEL=SmolLM2-135M-Instruct-Q8_0.gguf go test ./native/llama ./ui -v

# UI changes: render every screen headlessly and check the screenshots
SMOLCHAT_SHOTS=/tmp/shots go test ./ui -run Screens
```

For UI changes, include before/after screenshots in the pull request. If you
tested on Android, say which device or emulator and API level.

### Adding a model to the catalog

Every catalog entry must be pinned to a commit and match the Hub exactly:
repository id, file name, size and SHA-256. Run `CatalogMatchesHub` with
`SMOLCHAT_ONLINE=1` to confirm. Note the model's license, and if the GGUF is a
community conversion rather than the publisher's own, label it as one.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/) with a short,
lowercase summary:

```
feat: add pause and resume to model downloads
fix: keep partial file when download is cancelled
docs: document desktop build
```

Common types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`,
`chore`.

## Pull request checklist

- [ ] `go test ./...` passes
- [ ] `gofmt` and `go vet ./...` are clean
- [ ] New behaviour has tests
- [ ] UI changes include screenshots
- [ ] No new network hosts, permissions or telemetry
- [ ] README updated if user-facing behaviour or build steps changed

## Reporting bugs

Open an issue with:

* Device or emulator, Android version and RAM
* The model in use and the Auto/Manual setting
* Steps to reproduce, and what you expected instead
* Relevant lines from the app log (`files/logs/` in app storage)

Remove anything personal from logs and chat content before you post them.

## Security

Please don't report security problems in public issues. Contact the SmolWorks
maintainers privately so the issue can be fixed before it is disclosed.
