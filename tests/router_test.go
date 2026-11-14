package tests

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/smolworks/smolchat-android/app"
)

func spec(t *testing.T, id string) app.ModelSpec {
	t.Helper()
	m, ok := app.FindSpec(id)
	if !ok {
		t.Fatalf("missing %s", id)
	}
	return m
}

const gb = int64(1) << 30

func TestClassify(t *testing.T) {
	cases := map[string]app.Complexity{
		"hi":                               app.Simple,
		"Thanks!":                          app.Simple,
		"What is the capital of Peru?":     app.Simple,
		"Explain how photosynthesis works": app.Moderate,
		"Write a Go function that reverses a linked list and explain the complexity": app.Complex,
		"```python\ndef f(x):\n    return x\n```\nwhy does this fail?":               app.Complex,
	}
	for q, want := range cases {
		if got := app.Classify(q, 0); got != want {
			t.Errorf("Classify(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestClassifyCodeDetectionIsCaseSensitiveAndContextual(t *testing.T) {
	cases := map[string]app.Complexity{
		// Plain English that the case-insensitive keyword list used to score
		// as code, and a sentence that merely ends in a semicolon (#4).
		"let me know a good name for a cat": app.Simple,
		"what is the return policy":         app.Simple,
		"what time is it;":                  app.Simple,
		// "import" and "return" here are English words, not statements.
		"import tariffs on steel and return the book tomorrow": app.Simple,
		// Real code still carries the code signal.
		"```python\nprint(1)\n```": app.Moderate,
		"func main() {":            app.Moderate,
		"def foo(x):":              app.Moderate,
		"SELECT * FROM users":      app.Moderate,
		"select * from users":      app.Moderate, // SQL keywords are case-insensitive
		"#include <stdio.h>":       app.Moderate,
	}
	for q, want := range cases {
		if got := app.Classify(q, 0); got != want {
			t.Errorf("Classify(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestSelectModelByComplexity(t *testing.T) {
	small, mid, big := spec(t, "smollm2-135m-instruct-q8"), spec(t, "smollm2-360m-instruct-q8"), spec(t, "smollm2-1.7b-instruct-q4km")
	in := app.RouteInput{Installed: []app.ModelSpec{small, mid, big}, Device: app.DeviceInfo{TotalRAM: 8 * gb, AvailRAM: 4 * gb}, ContextSize: 2048}

	for q, want := range map[string]string{
		"hello":                     small.ID,
		"Explain how vaccines work": mid.ID,
		"Write a Python function that parses CSV files and explain each step in detail": big.ID,
	} {
		r, err := app.SelectModel(app.RouteRequest{Query: q}, in)
		if err != nil {
			t.Fatal(err)
		}
		if r.Model.ID != want {
			t.Errorf("%q -> %s (%s), want %s", q, r.Model.ID, r.Reason, want)
		}
	}
}

func TestSelectModelRespectsMemory(t *testing.T) {
	big, mid, sm3 := spec(t, "smollm2-1.7b-instruct-q4km"), spec(t, "smollm2-360m-instruct-q8"), spec(t, "smollm3-3b-q4km")
	in := app.RouteInput{Installed: []app.ModelSpec{mid, big, sm3}, Device: app.DeviceInfo{TotalRAM: 3 * gb, AvailRAM: 1 * gb}, ContextSize: 2048}
	r, err := app.SelectModel(app.RouteRequest{Query: "Write a detailed essay comparing the economic policies of two countries step by step"}, in)
	if err != nil {
		t.Fatal(err)
	}
	if r.Model.ID == sm3.ID {
		t.Fatalf("3B model chosen on a 3 GB phone")
	}
	if app.AssessFit(sm3, in.Device, 2048) != app.FitNo {
		t.Errorf("3B should not fit in 3 GB")
	}
}

func TestSelectModelFallsBackAndExplains(t *testing.T) {
	small := spec(t, "smollm2-135m-instruct-q8")
	in := app.RouteInput{Installed: []app.ModelSpec{small}, Device: app.DeviceInfo{TotalRAM: 8 * gb}}
	r, err := app.SelectModel(app.RouteRequest{Query: "Write a Rust function implementing quicksort and explain it"}, in)
	if err != nil || r.Model.ID != small.ID {
		t.Fatalf("fallback: %v %v", r.Model.ID, err)
	}
	if r.Reason == "" || r.Complexity != app.Complex {
		t.Errorf("expected explanation, got %+v", r)
	}
	if _, err := app.SelectModel(app.RouteRequest{Query: "hi"}, app.RouteInput{}); !errors.Is(err, app.ErrNoModel) {
		t.Errorf("no models: %v", err)
	}
	if _, err := app.SelectModel(app.RouteRequest{Query: "what's in this photo", NeedsVision: true}, in); err == nil {
		t.Error("vision request routed to a text model")
	}
}

func TestSelectModelPrefersLoaded(t *testing.T) {
	a, b := spec(t, "smollm-360m-instruct-q8"), spec(t, "smollm2-360m-instruct-q8")
	in := app.RouteInput{Installed: []app.ModelSpec{a, b}, Device: app.DeviceInfo{TotalRAM: 8 * gb}}
	r, _ := app.SelectModel(app.RouteRequest{Query: "Explain how tides work"}, in)
	if r.Model.ID != b.ID {
		t.Errorf("expected newer family, got %s", r.Model.ID)
	}
	in.LoadedID = a.ID
	r, _ = app.SelectModel(app.RouteRequest{Query: "Explain how tides work"}, in)
	if r.Model.ID != a.ID {
		t.Errorf("expected loaded model to avoid a reload, got %s", r.Model.ID)
	}
}

// fakeEngine records engine calls; it is a test double, not used by the app.
type fakeEngine struct {
	mu      sync.Mutex
	loaded  string
	loads   int
	unloads int
}

func (f *fakeEngine) LoadModel(ctx context.Context, cfg app.ModelConfig, progress func(float32)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loaded != "" {
		panic("load while another model is resident")
	}
	progress(0.5)
	progress(1)
	f.loaded, f.loads = cfg.ID, f.loads+1
	return nil
}

func (f *fakeEngine) Generate(ctx context.Context, msgs []app.ChatMessage, o app.GenerationOptions) (<-chan app.Token, error) {
	ch := make(chan app.Token, 3)
	ch <- app.Token{Text: "hi"}
	ch <- app.Token{Done: true, Stats: app.GenStats{GeneratedTokens: 1, StopReason: "eos"}}
	close(ch)
	return ch, nil
}
func (f *fakeEngine) Stop() error { return nil }
func (f *fakeEngine) UnloadModel() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loaded != "" {
		f.unloads++
	}
	f.loaded = ""
	return nil
}
func (f *fakeEngine) IsLoaded() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.loaded != "" }

func TestRuntimeKeepsOneModel(t *testing.T) {
	e := &fakeEngine{}
	r := app.NewRuntime(e, nil)
	ctx := context.Background()
	if err := r.Ensure(ctx, app.ModelConfig{ID: "a"}); err != nil {
		t.Fatal(err)
	}
	r.Ensure(ctx, app.ModelConfig{ID: "a"}) // no reload
	if e.loads != 1 {
		t.Fatalf("reloaded same model: %d loads", e.loads)
	}
	var wg sync.WaitGroup
	for _, id := range []string{"b", "c", "b", "a"} { // concurrent switches must serialise
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			var out string
			r.Generate(ctx, app.ModelConfig{ID: id}, nil, app.GenerationOptions{}, func(t app.Token) { out += t.Text })
		}(id)
	}
	wg.Wait()
	if st := r.Status(); st.Phase != app.PhaseReady || !e.IsLoaded() {
		t.Fatalf("status %+v", st)
	}
	if err := r.UnloadIf("zzz"); err != nil || !e.IsLoaded() {
		t.Fatal("UnloadIf unloaded the wrong model")
	}
	if err := r.UnloadIf(r.Status().ModelID); err != nil || e.IsLoaded() {
		t.Fatal("UnloadIf did not unload")
	}
}

func TestPreferenceShiftsTier(t *testing.T) {
	small, mid, big := spec(t, "smollm2-135m-instruct-q8"), spec(t, "smollm2-360m-instruct-q8"), spec(t, "smollm2-1.7b-instruct-q4km")
	in := app.RouteInput{Installed: []app.ModelSpec{small, mid, big}, Device: app.DeviceInfo{TotalRAM: 8 * gb}}
	q := app.RouteRequest{Query: "Explain how vaccines work"}
	for pref, want := range map[string]string{"speed": small.ID, "balanced": mid.ID, "quality": big.ID} {
		in.Preference = pref
		if r, _ := app.SelectModel(q, in); r.Model.ID != want {
			t.Errorf("%s: got %s, want %s", pref, r.Model.ID, want)
		}
	}
}
