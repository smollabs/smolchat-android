# Contributing to SmolChat for Android

Thanks for helping out! This guide walks you through everything from picking
an issue to getting your pull request merged. If this is your first open source
contribution, welcome. Follow the steps in order and you'll be fine. If you get
stuck at any point, ask in a comment on your issue. Questions are always
welcome.

## Contents

- [Ground rules](#ground-rules)
- [Contribution process at a glance](#contribution-process-at-a-glance)
- [Finding and claiming an issue](#finding-and-claiming-an-issue)
- [Requirements](#requirements)
- [Setting up the project](#setting-up-the-project)
- [Project layout](#project-layout)
- [Code style and conventions](#code-style-and-conventions)
- [Testing](#testing)
- [Commit messages](#commit-messages)
- [Creating a pull request](#creating-a-pull-request)
- [Review and merging](#review-and-merging)
- [Reporting bugs](#reporting-bugs)
- [Security](#security)
- [License](#license)

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

Please also be kind and patient in issues and reviews. Everyone here is
learning something.

## Contribution process at a glance

1. **Find an issue** you'd like to work on.
2. **Comment to claim it** and wait until a maintainer assigns it to you.
3. **Fork** the repository and **clone** your fork.
4. **Set up** the project and make sure it builds and the tests pass.
5. **Create a branch** for your change.
6. **Make your change**, with tests.
7. **Run the checks** (`gofmt`, `go vet`, `go test`).
8. **Commit** using Conventional Commits and **push** to your fork.
9. **Open a pull request** that links the issue.
10. **Respond to review** until it's approved and merged.

Each step is explained below.

## Finding and claiming an issue

### Finding an issue

All available work is in
[GitHub Issues](https://github.com/smollabs/smolchat-android/issues). Every
issue has a difficulty label:

| Label | What to expect |
|---|---|
| [`level 1`](https://github.com/smollabs/smolchat-android/issues?q=is%3Aissue+is%3Aopen+label%3A%22level+1%22) | Beginner. A small, well-scoped change, usually in one file, with clear steps. Start here if this is your first contribution. |
| [`level 2`](https://github.com/smollabs/smolchat-android/issues?q=is%3Aissue+is%3Aopen+label%3A%22level+2%22) | Intermediate. You'll need to read and understand one part of the app (downloads, database, Model Library…) and write tests for it. |
| [`level 3`](https://github.com/smollabs/smolchat-android/issues?q=is%3Aissue+is%3Aopen+label%3A%22level+3%22) | Advanced. Concurrency, the cgo bridge to llama.cpp, or changes across several packages. |

Other useful labels: `good first issue`, `bug`, `enhancement`, `docs`,
`performance`, and area labels such as `component:models`,
`component:runtime` and `area:inference`.

Each issue lists the **expected outcome** and the **relevant files**, so read
the issue fully and open those files before you claim it.

### Claiming an issue

1. Make sure the issue is **not already assigned** to someone.
2. Leave a comment such as: *"I'd like to work on this."* If you already have
   an idea of the approach, describe it in a sentence or two.
3. **Wait to be assigned** before you start coding. This avoids two people
   doing the same work.
4. Once assigned, try to open a pull request (a draft is fine) within
   **7 days**. If you need more time, just comment with an update.
5. If there's no activity for **7 days** without an update, the issue may be
   unassigned so someone else can pick it up.
6. If you can't continue, comment so the issue can be reassigned. That's
   completely fine.

Please work on **one issue at a time**. When your pull request is merged, pick
the next one.

### Proposing something new

Want to fix a bug or add a feature that has no issue yet? Open an issue first
and describe the problem and your proposed approach. For anything bigger than a
small fix, wait for a maintainer to agree on the approach before writing the
code.

## Requirements

**For desktop development (macOS or Linux):** this is enough for most issues.

* [Go](https://go.dev/dl/) 1.26 or newer (`go version` to check)
* [Git](https://git-scm.com/)
* A C/C++ compiler: Xcode Command Line Tools on macOS
  (`xcode-select --install`), or `gcc` and `g++` on Linux
* [CMake](https://cmake.org/) and [Ninja](https://ninja-build.org/)
  (`brew install cmake ninja` or `sudo apt install cmake ninja-build`)

**Additionally, for building the Android app:**

* Android SDK (platform 35, build-tools), via Android Studio or the command-line tools
* Android NDK r27 or newer
* JDK 17 or newer
* `adb` and an arm64 device or emulator

Go dependencies (Gio, `go-sqlite3`, …) are listed in `go.mod` and are
downloaded automatically by `go build` / `go run`. llama.cpp is a git submodule
in `native/llama.cpp`.

> **On Windows:** to run the full app, use WSL2 with a Linux setup
> (`native/build.sh` is a Bash script). The core package and its tests build
> natively, which is enough for issues in `app/` and `tests/`: install Go and a
> 64-bit `gcc` (for example [MSYS2](https://www.msys2.org/) MinGW-w64, needed by
> `go-sqlite3`), then run `go test ./tests`.

## Setting up the project

### 1. Fork and clone

Click **Fork** at the top of the
[repository page](https://github.com/smollabs/smolchat-android), then clone
**your fork** (replace `<your-username>`):

```bash
git clone --recurse-submodules --shallow-submodules https://github.com/<your-username>/smolchat-android
cd smolchat-android
git remote add upstream https://github.com/smollabs/smolchat-android
```

If you cloned without `--recurse-submodules`, run
`git submodule update --init --depth 1`.

### 2. Build and run on desktop

Most UI and core work is fastest on the desktop build, which runs the same code
on macOS and Linux:

```bash
native/build.sh host                   # build llama.cpp for your machine (once, takes a few minutes)
SMOLCHAT_DATA=/tmp/smolchat go run .   # run the app with a scratch data directory
```

`native/build.sh host` must run before `go run .` or `go test ./...`,
otherwise the build fails with linker errors about missing `llama`/`ggml`
libraries.

### 3. Check that the tests pass

```bash
go test ./...
```

If this passes, your setup is ready.

### 4. (Optional) Run on Android

```bash
go install gioui.org/cmd/gogio@latest
native/build.sh android
./build.sh
adb install -r build/SmolChat-debug.apk
```

See the [README](README.md#getting-started) for release builds and more detail.

### Keeping your fork up to date

Before starting new work, sync with the main repository:

```bash
git checkout main
git fetch upstream
git merge upstream/main
git push origin main
```

## Project layout

* `app/` is the platform-independent core: chat, database, downloads, models,
  router, hardware detection. It **must not import any UI package**.
* `ui/` is the Gio interface. It reads snapshots from `app.App` each frame and
  calls its methods.
* `native/llama/` is the cgo bridge to llama.cpp. `native/llama.cpp` is a git
  submodule; **don't edit it in place**.
* `tests/` holds unit tests for the core.
* `assets/` holds fonts, icons and brand artwork.

The [Architecture section of the README](README.md#architecture) describes each
file.

## Code style and conventions

* **Format with `gofmt`.** Every Go file must be `gofmt`-clean.
* **Run `go vet ./...`** and fix anything it reports.
* **Match the surrounding code.** Follow the naming, comment style and
  structure of the file you're editing. Keep comments short and explain *why*,
  not *what*.
* **Never block the Gio event loop.** Slow work (downloads, model loading,
  generation) runs on a goroutine that calls `Window.Invalidate` when it has
  something new to show.
* **Keep `app/` UI-free.** Logic goes in `app/`; `ui/` only draws and forwards
  user actions.
* **Guard shared state.** Fields shared between goroutines are protected by a
  mutex. Don't hold a lock while calling into the store, the network or
  llama.cpp unless the surrounding code already does.
* **Handle errors.** Don't ignore returned errors; show users a friendly
  message rather than a raw error where it reaches the UI.
* **No new dependencies** without discussing it in the issue first.
* **Keep changes focused.** Don't reformat or refactor unrelated code in the
  same pull request.

## Testing

Add or update tests for any behaviour you change. Tests for the core live in
`tests/` and are table-driven where possible. Look at the existing tests in
`tests/router_test.go`, `tests/downloader_test.go` and
`tests/database_test.go` as examples.

Always run:

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

## Creating a pull request

1. **Create a branch** from an up-to-date `main`, named after the issue:

   ```bash
   git checkout -b fix/42-router-plain-english
   ```

2. **Make your change** and add tests.

3. **Run the checks:**

   ```bash
   gofmt -l .        # should print nothing
   go vet ./...
   go test ./...
   ```

4. **Commit and push** to your fork:

   ```bash
   git add <files you changed>
   git commit -m "fix: stop flagging plain English as code in the router"
   git push origin fix/42-router-plain-english
   ```

5. **Open the pull request.** GitHub shows a **Compare & pull request** button
   on your fork. Make sure the base is `smollabs/smolchat-android` → `main`.

6. **Fill in the description:**
   * What the change does and why
   * `Closes #<issue-number>` so the issue closes automatically when merged
   * How you tested it (commands you ran; device/emulator and API level if
     you tested on Android)
   * Before/after screenshots for any UI change

7. Use a **draft pull request** if you'd like early feedback before it's
   finished.

### Pull request checklist

- [ ] The PR links its issue (`Closes #…`) and the issue was assigned to me
- [ ] `go test ./...` passes
- [ ] `gofmt` and `go vet ./...` are clean
- [ ] New behaviour has tests
- [ ] UI changes include screenshots
- [ ] No new network hosts, permissions, dependencies or telemetry
- [ ] README updated if user-facing behaviour or build steps changed

## Review and merging

* A maintainer will review your pull request and may ask for changes. This is
  normal and part of the process, not a sign that something is wrong.
* Push further commits to the **same branch** to update the pull request.
  There's no need to open a new one.
* Reply to each review comment, or resolve it once you've addressed it.
* If `main` moves on and there are conflicts, merge or rebase on the latest
  `upstream/main`.
* Once approved, a maintainer merges it. Thank you!

## Reporting bugs

Open an issue with:

* Device or emulator, Android version and RAM
* The model in use and the Auto/Manual setting
* Steps to reproduce, and what you expected instead
* Relevant lines from the app log (`files/logs/` in app storage)

Remove anything personal from logs and chat content before you post them.

## Security

Please don't report security problems in public issues. Report them privately
through the repository's **Security** tab (**Report a vulnerability**), so the
issue can be fixed before it is disclosed.

## License

SmolChat is released under the [MIT License](LICENSE). By submitting a pull
request, you agree that your contribution is licensed under the same terms.
Code copied from elsewhere must come under a compatible license; keep its
copyright notice and mention its source in the pull request.
