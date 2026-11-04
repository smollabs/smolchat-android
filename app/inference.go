package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ModelConfig describes how a model should be loaded.
type ModelConfig struct {
	ID          string
	Path        string // main GGUF weights
	ContextSize int
	Threads     int
}

// ChatMessage is one turn passed to an engine. The engine applies the model's
// own chat template.
type ChatMessage struct {
	Role    string // "system", "user" or "assistant"
	Content string
}

// GenerationOptions controls sampling.
type GenerationOptions struct {
	MaxTokens     int
	Temperature   float32
	TopK          int
	TopP          float32
	MinP          float32
	RepeatPenalty float32
	Seed          uint32
}

// Token is one streamed piece of output. The final value on the channel has
// Done set, and carries Err if generation failed.
type Token struct {
	Text  string
	Done  bool
	Err   error
	Stats GenStats
}

// GenStats summarises a finished generation.
type GenStats struct {
	PromptTokens    int
	GeneratedTokens int
	Duration        time.Duration
	TokensPerSecond float64
	StopReason      string // "eos", "length", "cancelled", "error"
}

// InferenceEngine isolates a local inference backend (llama.cpp today).
// Implementations must be safe for use from multiple goroutines; the Runtime
// additionally serialises access so a native context is never used
// concurrently.
type InferenceEngine interface {
	// LoadModel loads weights; progress reports 0..1. Cancelling ctx aborts.
	LoadModel(ctx context.Context, cfg ModelConfig, progress func(float32)) error
	// Generate streams a reply to messages. The channel is closed after the
	// final Done token. Cancelling ctx stops generation.
	Generate(ctx context.Context, messages []ChatMessage, opts GenerationOptions) (<-chan Token, error)
	// Stop interrupts any running generation.
	Stop() error
	// UnloadModel releases all native memory. It is a no-op when nothing is
	// loaded.
	UnloadModel() error
	IsLoaded() bool
}

// ErrEngineUnavailable is returned when the build has no native backend.
var ErrEngineUnavailable = errors.New("the local inference engine is not available in this build")

// RuntimePhase is the lifecycle state of the model runtime.
type RuntimePhase int

const (
	PhaseIdle RuntimePhase = iota
	PhaseLoading
	PhaseReady
	PhaseGenerating
	PhaseError
)

func (p RuntimePhase) String() string {
	switch p {
	case PhaseLoading:
		return "loading"
	case PhaseReady:
		return "ready"
	case PhaseGenerating:
		return "generating"
	case PhaseError:
		return "error"
	}
	return "idle"
}

// RuntimeStatus is a snapshot of the runtime for the UI.
type RuntimeStatus struct {
	Phase    RuntimePhase
	ModelID  string  // loaded (or loading) model
	Progress float32 // load progress
	Err      string
}

// Runtime owns the single loaded model. It guarantees that at most one model
// is resident, that loads never race each other, and that a model is never
// unloaded while generating.
type Runtime struct {
	engine   InferenceEngine
	onChange func()

	op sync.Mutex // serialises load/unload/generate on the engine

	mu     sync.Mutex
	status RuntimeStatus
	cancel context.CancelFunc // cancels the in-flight load or generation
}

// NewRuntime wraps engine. onChange is called (from any goroutine) whenever
// the status changes.
func NewRuntime(engine InferenceEngine, onChange func()) *Runtime {
	if onChange == nil {
		onChange = func() {}
	}
	return &Runtime{engine: engine, onChange: onChange}
}

// Status returns the current status.
func (r *Runtime) Status() RuntimeStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *Runtime) set(f func(s *RuntimeStatus)) {
	r.mu.Lock()
	f(&r.status)
	r.mu.Unlock()
	r.onChange()
}

// Ensure makes cfg.ID the loaded model, unloading any other model first.
// It is cheap when the model is already loaded.
func (r *Runtime) Ensure(ctx context.Context, cfg ModelConfig) error {
	if r.engine == nil {
		return ErrEngineUnavailable
	}
	r.op.Lock()
	defer r.op.Unlock()
	return r.ensureLocked(ctx, cfg)
}

func (r *Runtime) ensureLocked(ctx context.Context, cfg ModelConfig) error {
	st := r.Status()
	if r.engine.IsLoaded() && st.ModelID == cfg.ID && st.Phase != PhaseError {
		return nil
	}
	if r.engine.IsLoaded() {
		if err := r.engine.UnloadModel(); err != nil {
			return fmt.Errorf("unload previous model: %w", err)
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()
	r.set(func(s *RuntimeStatus) { *s = RuntimeStatus{Phase: PhaseLoading, ModelID: cfg.ID} })

	var lastPct int = -1
	err := r.engine.LoadModel(ctx, cfg, func(p float32) {
		// Throttle UI updates to whole percents.
		if pct := int(p * 100); pct != lastPct {
			lastPct = pct
			r.set(func(s *RuntimeStatus) { s.Progress = p })
		}
	})
	r.mu.Lock()
	r.cancel = nil
	r.mu.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			r.set(func(s *RuntimeStatus) { *s = RuntimeStatus{} })
			return ctx.Err()
		}
		r.set(func(s *RuntimeStatus) { *s = RuntimeStatus{Phase: PhaseError, ModelID: cfg.ID, Err: err.Error()} })
		return err
	}
	r.set(func(s *RuntimeStatus) { *s = RuntimeStatus{Phase: PhaseReady, ModelID: cfg.ID, Progress: 1} })
	return nil
}

// Generate loads cfg if needed and streams a reply. onToken is called
// synchronously for every token, including the final Done token.
func (r *Runtime) Generate(ctx context.Context, cfg ModelConfig, msgs []ChatMessage, opts GenerationOptions, onToken func(Token)) error {
	if r.engine == nil {
		return ErrEngineUnavailable
	}
	r.op.Lock()
	defer r.op.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := r.ensureLocked(ctx, cfg); err != nil {
		return err
	}
	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()
	r.set(func(s *RuntimeStatus) { s.Phase = PhaseGenerating })
	defer func() {
		r.mu.Lock()
		r.cancel = nil
		r.mu.Unlock()
		r.set(func(s *RuntimeStatus) {
			if s.Phase == PhaseGenerating {
				s.Phase = PhaseReady
			}
		})
	}()
	ch, err := r.engine.Generate(ctx, msgs, opts)
	if err != nil {
		return err
	}
	for t := range ch {
		onToken(t)
	}
	return nil
}

// Cancel interrupts the in-flight load or generation, if any.
func (r *Runtime) Cancel() {
	r.mu.Lock()
	c := r.cancel
	r.mu.Unlock()
	if c != nil {
		c()
	}
	if r.engine != nil {
		r.engine.Stop()
	}
}

// Unload cancels any work and releases the loaded model.
func (r *Runtime) Unload() error {
	if r.engine == nil {
		return nil
	}
	r.Cancel()
	r.op.Lock()
	defer r.op.Unlock()
	err := r.engine.UnloadModel()
	r.set(func(s *RuntimeStatus) { *s = RuntimeStatus{} })
	return err
}

// Release unloads the model once any running generation has finished,
// without interrupting it (used when settings that affect loading change).
func (r *Runtime) Release() {
	if r.engine == nil {
		return
	}
	r.op.Lock()
	defer r.op.Unlock()
	r.engine.UnloadModel()
	r.set(func(s *RuntimeStatus) { *s = RuntimeStatus{} })
}

// UnloadIf unloads only when id is the loaded model (used before deleting).
func (r *Runtime) UnloadIf(id string) error {
	if r.Status().ModelID != id {
		return nil
	}
	return r.Unload()
}
