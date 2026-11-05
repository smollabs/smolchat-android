// Package llama implements app.InferenceEngine on top of llama.cpp, linked
// statically from native/lib/<goos>-<goarch> (see native/build.sh).
package llama

/*
#cgo CFLAGS: -O2 -I${SRCDIR}/../llama.cpp/include -I${SRCDIR}/../llama.cpp/ggml/include
#cgo android,arm64 LDFLAGS: -L${SRCDIR}/../lib/android-arm64 -lllama -lggml -lggml-cpu -lggml-base -lc++_static -lc++abi -lm -ldl -llog
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/../lib/darwin-arm64 -lllama -lggml -lggml-cpu -lggml-base -lc++
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/../lib/darwin-amd64 -lllama -lggml -lggml-cpu -lggml-base -lc++
#cgo linux,!android LDFLAGS: -L${SRCDIR}/../lib/linux-${GOARCH} -lllama -lggml -lggml-cpu -lggml-base -lstdc++ -lm -lpthread
#include <stdlib.h>
#include "bridge.h"

static void smol_set_abort(smol_ctx *s, int v) { s->abort = v; }
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/smolworks/smolchat-android/app"
)

var initOnce sync.Once

// Progress listeners for in-flight loads, keyed by an id passed through C.
var (
	progressMu   sync.Mutex
	progressFns  = map[uintptr]func(float32) bool{}
	progressNext uintptr
)

//export goLoadProgress
func goLoadProgress(id C.uintptr_t, p C.float) C._Bool {
	progressMu.Lock()
	fn := progressFns[uintptr(id)]
	progressMu.Unlock()
	if fn == nil {
		return true
	}
	return C._Bool(fn(float32(p)))
}

// Engine is a llama.cpp backed app.InferenceEngine. One Engine holds at most
// one model; mu guarantees the native context is used by one goroutine at a
// time.
type Engine struct {
	mu     sync.Mutex
	s      *C.smol_ctx
	nBatch int
	cached []C.int32_t // tokens currently held in the KV cache (sequence 0)

	abortMu sync.Mutex
	active  *C.smol_ctx // context of the running generation, for Stop
}

// New returns an engine with nothing loaded.
func New() *Engine { return &Engine{} }

var _ app.InferenceEngine = (*Engine)(nil)

// LoadModel loads cfg.Path. Any previously loaded model is released first.
func (e *Engine) LoadModel(ctx context.Context, cfg app.ModelConfig, progress func(float32)) error {
	if err := checkCPU(); err != nil {
		return err
	}
	if _, err := os.Stat(cfg.Path); err != nil {
		return fmt.Errorf("model file missing: %w", err)
	}
	initOnce.Do(func() { C.smol_backend_init() })

	e.mu.Lock()
	defer e.mu.Unlock()
	e.unloadLocked()

	progressMu.Lock()
	progressNext++
	id := progressNext
	progressFns[id] = func(p float32) bool {
		if progress != nil {
			progress(p)
		}
		return ctx.Err() == nil
	}
	progressMu.Unlock()
	defer func() {
		progressMu.Lock()
		delete(progressFns, id)
		progressMu.Unlock()
	}()

	nCtx := cfg.ContextSize
	if nCtx <= 0 {
		nCtx = 2048
	}
	threads := cfg.Threads
	if threads <= 0 {
		threads = DefaultThreads()
	}
	const nBatch = 256 // small batches keep peak compute buffers low on phones

	cpath := C.CString(cfg.Path)
	defer C.free(unsafe.Pointer(cpath))
	var errBuf [256]C.char
	s := C.smol_load(cpath, C.int(nCtx), C.int(threads), C.int(nBatch), C.uintptr_t(id), &errBuf[0], C.int(len(errBuf)))
	if s == nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New(C.GoString(&errBuf[0]))
	}
	e.s = s
	e.nBatch = nBatch
	e.cached = nil
	return nil
}

// DefaultThreads picks a thread count that favours big cores on phones:
// most SoCs have 4 efficiency cores that slow a synchronised decode down.
func DefaultThreads() int {
	n := runtime.NumCPU()
	switch {
	case n >= 8:
		return 4
	case n >= 4:
		return n - 1
	default:
		return max(1, n)
	}
}

// checkCPU guards against SIGILL: the Android build targets
// armv8.2-a+dotprod.
func checkCPU() error {
	if runtime.GOOS != "android" || runtime.GOARCH != "arm64" {
		return nil
	}
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return nil // can't tell; assume fine
	}
	if !strings.Contains(string(b), "asimddp") {
		return errors.New("this device's CPU lacks the ARMv8.2 dot-product instructions SmolChat's inference engine requires")
	}
	return nil
}

func (e *Engine) IsLoaded() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.s != nil
}

func (e *Engine) UnloadModel() error {
	e.Stop()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.unloadLocked()
	return nil
}

func (e *Engine) unloadLocked() {
	if e.s != nil {
		C.smol_free(e.s)
		e.s = nil
		e.cached = nil
	}
}

// Stop aborts the running generation, if any.
func (e *Engine) Stop() error {
	e.abortMu.Lock()
	if e.active != nil {
		C.smol_set_abort(e.active, 1)
	}
	e.abortMu.Unlock()
	return nil
}

// Generate formats msgs with the model's chat template and streams a reply.
func (e *Engine) Generate(ctx context.Context, msgs []app.ChatMessage, opts app.GenerationOptions) (<-chan app.Token, error) {
	e.mu.Lock()
	if e.s == nil {
		e.mu.Unlock()
		return nil, errors.New("no model loaded")
	}
	s := e.s
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 512
	}
	nCtx := int(C.smol_n_ctx(s))
	// Leave room for the reply; trim the oldest turns until the prompt fits.
	budget := nCtx - min(opts.MaxTokens, nCtx/2)
	tokens, err := e.fitPrompt(msgs, budget)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	C.smol_set_sampler(s, C.float(opts.Temperature), C.int(opts.TopK), C.float(opts.TopP),
		C.float(opts.MinP), C.float(opts.RepeatPenalty), 64, C.uint32_t(opts.Seed))

	e.abortMu.Lock()
	e.active = s
	C.smol_set_abort(s, 0)
	e.abortMu.Unlock()

	out := make(chan app.Token, 32)
	go func() {
		defer close(out)
		defer e.mu.Unlock()
		defer func() {
			e.abortMu.Lock()
			e.active = nil
			e.abortMu.Unlock()
		}()
		stop := context.AfterFunc(ctx, func() { e.Stop() })
		defer stop()
		out <- e.run(ctx, s, tokens, opts, nCtx, out)
	}()
	return out, nil
}

// fitPrompt tokenizes msgs, dropping the oldest non-system turns until the
// prompt fits in budget tokens.
func (e *Engine) fitPrompt(msgs []app.ChatMessage, budget int) ([]C.int32_t, error) {
	for {
		toks, err := e.tokenizeChat(msgs)
		if err != nil {
			return nil, err
		}
		if len(toks) <= budget {
			return toks, nil
		}
		// Find the first droppable message (keep system + last user turn).
		drop := -1
		for i, m := range msgs[:len(msgs)-1] {
			if m.Role != "system" {
				drop = i
				break
			}
		}
		if drop < 0 {
			return nil, fmt.Errorf("message too long for the model's context (%d tokens, limit %d)", len(toks), budget)
		}
		msgs = append(append([]app.ChatMessage{}, msgs[:drop]...), msgs[drop+1:]...)
	}
}

func (e *Engine) tokenizeChat(msgs []app.ChatMessage) ([]C.int32_t, error) {
	n := len(msgs)
	roles := C.malloc(C.size_t(n) * C.size_t(unsafe.Sizeof(uintptr(0))))
	contents := C.malloc(C.size_t(n) * C.size_t(unsafe.Sizeof(uintptr(0))))
	defer C.free(roles)
	defer C.free(contents)
	rs := unsafe.Slice((**C.char)(roles), n)
	cs := unsafe.Slice((**C.char)(contents), n)
	total := 0
	for i, m := range msgs {
		rs[i] = C.CString(m.Role)
		cs[i] = C.CString(m.Content)
		total += len(m.Content) + 64
		defer C.free(unsafe.Pointer(rs[i]))
		defer C.free(unsafe.Pointer(cs[i]))
	}
	buf := make([]byte, 2*total+256)
	r := C.smol_apply_template(e.s, &rs[0], &cs[0], C.int(n), (*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
	if int(r) > len(buf) {
		buf = make([]byte, int(r)+1)
		r = C.smol_apply_template(e.s, &rs[0], &cs[0], C.int(n), (*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
	}
	if r < 0 {
		return nil, errors.New("could not apply chat template")
	}
	prompt := buf[:r]
	toks := make([]C.int32_t, len(prompt)+16)
	nt := C.smol_tokenize(e.s, (*C.char)(unsafe.Pointer(&prompt[0])), C.int(len(prompt)), &toks[0], C.int(len(toks)), true)
	if nt < 0 {
		return nil, errors.New("tokenization failed")
	}
	return toks[:nt], nil
}

func (e *Engine) run(ctx context.Context, s *C.smol_ctx, tokens []C.int32_t, opts app.GenerationOptions, nCtx int, out chan<- app.Token) app.Token {
	start := time.Now()
	stats := app.GenStats{PromptTokens: len(tokens)}
	var genStart time.Time // after the prompt is decoded
	finish := func(reason string, err error) app.Token {
		stats.StopReason = reason
		stats.Duration = time.Since(start)
		// Decode speed only: prompt processing would dominate short replies.
		if secs := time.Since(genStart).Seconds(); !genStart.IsZero() && secs > 0 && stats.GeneratedTokens > 0 {
			stats.TokensPerSecond = float64(stats.GeneratedTokens) / secs
		}
		return app.Token{Done: true, Err: err, Stats: stats}
	}

	// Reuse the KV cache for the shared prefix with the previous turn.
	prefix := 0
	for prefix < len(e.cached) && prefix < len(tokens) && e.cached[prefix] == tokens[prefix] {
		prefix++
	}
	if prefix == len(tokens) { // need at least one token to get fresh logits
		prefix--
	}
	C.smol_kv_truncate(s, C.int(prefix))
	e.cached = e.cached[:prefix]
	if err := e.decode(s, tokens[prefix:]); err != nil {
		e.cached = nil
		C.smol_kv_clear(s)
		if ctx.Err() != nil {
			return finish("cancelled", nil)
		}
		return finish("error", err)
	}

	genStart = time.Now()
	var pending []byte // bytes of an incomplete UTF-8 sequence
	piece := make([]byte, 256)
	for stats.GeneratedTokens < opts.MaxTokens {
		if ctx.Err() != nil {
			return finish("cancelled", nil)
		}
		if len(e.cached) >= nCtx {
			return finish("length", nil)
		}
		tok := C.smol_sample(s)
		if C.smol_is_eog(s, tok) {
			return finish("eos", nil)
		}
		n := C.smol_token_piece(s, tok, (*C.char)(unsafe.Pointer(&piece[0])), C.int(len(piece)))
		if n > 0 {
			pending = append(pending, piece[:n]...)
			if text := validPrefix(&pending); text != "" {
				select {
				case out <- app.Token{Text: text}:
				case <-ctx.Done():
				}
			}
		}
		stats.GeneratedTokens++
		if err := e.decode(s, []C.int32_t{tok}); err != nil {
			if ctx.Err() != nil {
				return finish("cancelled", nil)
			}
			return finish("error", err)
		}
	}
	return finish("length", nil)
}

// decode feeds tokens in nBatch chunks and records them as cached.
func (e *Engine) decode(s *C.smol_ctx, tokens []C.int32_t) error {
	for len(tokens) > 0 {
		n := min(len(tokens), e.nBatch)
		switch rc := C.smol_decode(s, &tokens[0], C.int(n)); rc {
		case 0:
		case 2:
			// llama.cpp drops the partial ubatch on abort; resync with it.
			if keep := int(C.smol_kv_pos_max(s)) + 1; keep < len(e.cached) {
				e.cached = e.cached[:max(keep, 0)]
			}
			return errors.New("aborted")
		default:
			return fmt.Errorf("llama_decode failed (%d)", int(rc))
		}
		e.cached = append(e.cached, tokens[:n]...)
		tokens = tokens[n:]
	}
	return nil
}

// validPrefix returns and consumes the longest valid UTF-8 prefix of *b,
// keeping a trailing incomplete rune for the next token.
func validPrefix(b *[]byte) string {
	buf := *b
	end := len(buf)
	// Back off at most 3 bytes looking for an incomplete trailing rune.
	for i := 1; i <= 3 && i <= len(buf); i++ {
		c := buf[len(buf)-i]
		if c&0xC0 == 0x80 { // continuation byte
			continue
		}
		if c >= 0xC0 { // start byte: is the rune complete?
			if !utf8.FullRune(buf[len(buf)-i:]) {
				end = len(buf) - i
			}
		}
		break
	}
	s := string(buf[:end])
	*b = append(buf[:0], buf[end:]...)
	return s
}
