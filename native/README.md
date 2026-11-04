# Native inference (llama.cpp)

SmolChat runs models with [llama.cpp](https://github.com/ggml-org/llama.cpp),
linked **statically** into the Go shared library that gogio packages
(`lib/arm64-v8a/libgio.so`). There is no separate `.so`, no `libc++_shared.so`
and no server process.

```
native/
├── llama.cpp/     git submodule, pinned to tag b12111
├── build.sh       builds static libs into native/lib/<target>/
└── llama/         Go package: cgo bridge implementing app.InferenceEngine
    ├── bridge.c/h small C shim (progress + abort callbacks, sampler chain, templates)
    └── llama.go   model lifecycle, KV-cache reuse, streaming, cancellation
```

## Building

```bash
git submodule update --init --depth 1
native/build.sh android   # arm64-v8a via the NDK  -> native/lib/android-arm64/
native/build.sh host      # your desktop, for `go test` and desktop runs
```

The Android build targets `armv8.2-a+dotprod+fp16` (Cortex-A55/A75 and newer,
roughly every phone since 2018). At load time the engine checks
`/proc/cpuinfo` for `asimddp` and reports a clear error instead of crashing
with SIGILL on older CPUs. OpenMP, Vulkan, curl and OpenSSL are disabled: CPU
inference only, no network code in the native layer.

## How the bridge works

* **Loading**: `llama_model_load_from_file` with mmap, so weights stay
  file-backed. A C progress callback calls back into Go (`goLoadProgress`) to
  drive the UI's progress bar and to abort when the load is cancelled.
* **Prompting**: the model's own chat template is applied with
  `llama_chat_apply_template` (ChatML fallback, which every Smol model uses).
  When a conversation outgrows the context window the oldest turns are
  dropped (the system prompt is kept).
* **KV-cache reuse**: the engine remembers which tokens are in the cache and
  only decodes the new suffix of each turn, so follow-up messages start
  almost instantly.
* **Streaming**: tokens are sampled one at a time and sent over a Go channel;
  partial UTF-8 sequences are held back until complete.
* **Cancellation**: a context cancel sets an abort flag polled by
  llama.cpp's `abort_callback`, which interrupts even long prompt decodes.
* **Concurrency**: one mutex guards the native context; `app.Runtime` adds
  load/unload/generate serialisation so a model is never freed while in use
  and only one model is ever resident.

## Testing on the host

```bash
native/build.sh host
SMOLCHAT_TEST_MODEL=/path/to/SmolLM2-135M-Instruct-Q8_0.gguf go test ./native/llama -v
```
