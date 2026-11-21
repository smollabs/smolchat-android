package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smollabs/smolchat-android/app"
)

// fakeHub serves one file with Range support, optionally dropping the
// connection after cut bytes on the first request.
type fakeHub struct {
	data      []byte
	cut       int64
	requests  atomic.Int32
	ranges    []string
	slow      bool
	badRange  bool
	badLength bool
}

func (h *fakeHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := h.requests.Add(1)
	h.ranges = append(h.ranges, r.Header.Get("Range"))
	start := int64(0)
	if rg := r.Header.Get("Range"); rg != "" {
		start, _ = strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"), 10, 64)
		respStart := start
		if h.badRange {
			respStart = 0
			h.badRange = false
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", respStart, len(h.data)-1, len(h.data)))
		w.Header().Set("Content-Length", strconv.Itoa(len(h.data)-int(respStart)))
		w.WriteHeader(http.StatusPartialContent)
		start = respStart
	} else {
		contentLen := len(h.data)
		if h.badLength {
			contentLen -= 100
		}
		w.Header().Set("Content-Length", strconv.Itoa(contentLen))
		w.WriteHeader(http.StatusOK)
	}
	body := h.data[start:]
	if n == 1 && h.cut > 0 {
		w.Write(body[:h.cut])
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler) // simulate a dropped connection
	}
	for len(body) > 0 {
		k := min(len(body), 64<<10)
		if _, err := w.Write(body[:k]); err != nil {
			return
		}
		body = body[k:]
		if h.slow {
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func testSpec(data []byte) app.ModelSpec {
	sum := sha256.Sum256(data)
	return app.ModelSpec{
		ID: "test-model", Name: "Test", Category: app.CategoryText, Backend: app.BackendLlama, Supported: true,
		Repo: "x/y", Revision: "main",
		Files: []app.ModelFile{{Name: "w.gguf", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Role: "weights"}},
	}
}

func setup(t *testing.T, hub *fakeHub) (*app.Downloader, *app.ModelManager, app.Paths, func(app.ModelSpec) app.DownloadProgress) {
	t.Helper()
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)
	p, err := app.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mm := app.NewModelManager(p)
	var alive atomic.Int32
	d := app.NewDownloader(p, mm, nil, func(on bool) {
		if on {
			alive.Add(1)
		} else {
			alive.Add(-1)
		}
	})
	d.URLFor = func(app.ModelSpec, app.ModelFile) string { return srv.URL + "/file" }
	d.TreeURL = nil
	d.Retries = 2
	wait := func(spec app.ModelSpec) app.DownloadProgress {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if pr := d.Progress(spec); !pr.Active() {
				if alive.Load() != 0 {
					t.Errorf("keep-alive not released")
				}
				return pr
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("download did not finish")
		return app.DownloadProgress{}
	}
	return d, mm, p, wait
}

func payload(n int) []byte {
	return bytes.Repeat([]byte("smolchat-"), n/9+1)[:n]
}

// The downloader registers models by catalog id, so tests download into a
// throwaway spec and check the files directly.
func TestDownloadVerifiesAndInstalls(t *testing.T) {
	data := payload(700_000)
	hub := &fakeHub{data: data}
	d, mm, _, wait := setup(t, hub)
	spec := testSpec(data)
	if err := d.Start(spec); err != nil {
		t.Fatal(err)
	}
	pr := wait(spec)
	if pr.Status != app.DLDone {
		t.Fatalf("status %v err %q", pr.Status, pr.Err)
	}
	got, err := os.ReadFile(filepath.Join(mm.Dir(spec.ID), "w.gguf"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("file mismatch: %v", err)
	}
}

func TestDownloadResumesAfterDrop(t *testing.T) {
	data := payload(900_000)
	hub := &fakeHub{data: data, cut: 300_000}
	d, mm, _, wait := setup(t, hub)
	spec := testSpec(data)
	if err := d.Start(spec); err != nil {
		t.Fatal(err)
	}
	if pr := wait(spec); pr.Status != app.DLDone {
		t.Fatalf("status %v err %q", pr.Status, pr.Err)
	}
	if hub.requests.Load() < 2 {
		t.Fatal("expected a retry")
	}
	if last := hub.ranges[len(hub.ranges)-1]; last != "bytes=300000-" {
		t.Errorf("did not resume from partial, Range=%q", last)
	}
	got, _ := os.ReadFile(filepath.Join(mm.Dir(spec.ID), "w.gguf"))
	if !bytes.Equal(got, data) {
		t.Fatal("resumed file corrupt")
	}
}

func TestDownloadRecoversFromBadRange(t *testing.T) {
	data := payload(900_000)
	hub := &fakeHub{data: data, cut: 300_000, badRange: true}
	d, mm, _, wait := setup(t, hub)
	spec := testSpec(data)
	if err := d.Start(spec); err != nil {
		t.Fatal(err)
	}
	if pr := wait(spec); pr.Status != app.DLDone {
		t.Fatalf("status %v err %q", pr.Status, pr.Err)
	}
	if hub.requests.Load() < 3 {
		t.Fatalf("expected at least 3 requests, got %d", hub.requests.Load())
	}
	if len(hub.ranges) < 3 {
		t.Fatalf("expected at least 3 requests recorded, got %d", len(hub.ranges))
	}
	if hub.ranges[1] != "bytes=300000-" {
		t.Errorf("request 2 should be resume, got Range=%q", hub.ranges[1])
	}
	if hub.ranges[2] != "" {
		t.Errorf("request 3 should restart from zero, got Range=%q", hub.ranges[2])
	}
	got, err := os.ReadFile(filepath.Join(mm.Dir(spec.ID), "w.gguf"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("resumed file corrupt")
	}
}

func TestDownloadRejectsMismatchedContentLength(t *testing.T) {
	data := payload(200_000)
	hub := &fakeHub{data: data, badLength: true}
	d, mm, _, wait := setup(t, hub)
	d.Retries = 0
	spec := testSpec(data)
	d.Start(spec)
	pr := wait(spec)
	if pr.Status != app.DLFailed || !strings.Contains(pr.Err, "wrong Content-Length") {
		t.Fatalf("status %v err %q", pr.Status, pr.Err)
	}
	if _, err := os.Stat(filepath.Join(mm.Dir(spec.ID), "w.gguf")); err == nil {
		t.Fatal("file was installed despite mismatched Content-Length")
	}
	if d.PartialBytes(spec) != 0 {
		t.Fatal("partial file was created or kept")
	}
}

func TestParseContentRange(t *testing.T) {
	tests := []struct {
		input string
		start int64
		end   int64
		total int64
		ok    bool
	}{
		{"bytes 0-499/1000", 0, 499, 1000, true},
		{"bytes 300000-899999/900000", 300000, 899999, 900000, true},
		{"bytes 0-0/1", 0, 0, 1, true},
		{"bytes 100-200/*", 100, 200, -1, true},
		{"Bytes 0-499/1000", 0, 499, 1000, true},
		{"  bytes 0-499/1000  ", 0, 499, 1000, true},
		{"", 0, 0, 0, false},
		{"bytes", 0, 0, 0, false},
		{"bytes ", 0, 0, 0, false},
		{"items 0-499/1000", 0, 0, 0, false},
		{"bytes -499/1000", 0, 0, 0, false},
		{"bytes 499-/1000", 0, 0, 0, false},
		{"bytes 500-400/1000", 0, 0, 0, false},
		{"bytes 0-1000/1000", 0, 0, 0, false},
		{"bytes 0-499/0", 0, 0, 0, false},
		{"bytes 0-499/-1", 0, 0, 0, false},
		{"bytes abc-def/1000", 0, 0, 0, false},
		{"bytes 0-100/xyz", 0, 0, 0, false},
	}
	for _, tc := range tests {
		start, end, total, ok := app.ParseContentRange(tc.input)
		if ok != tc.ok || start != tc.start || end != tc.end || total != tc.total {
			t.Errorf("ParseContentRange(%q) = (%d, %d, %d, %v), want (%d, %d, %d, %v)",
				tc.input, start, end, total, ok, tc.start, tc.end, tc.total, tc.ok)
		}
	}
}

func TestDownloadRejectsBadChecksum(t *testing.T) {
	data := payload(200_000)
	hub := &fakeHub{data: data}
	d, mm, _, wait := setup(t, hub)
	spec := testSpec(data)
	spec.Files[0].SHA256 = strings.Repeat("0", 64)
	d.Start(spec)
	pr := wait(spec)
	if pr.Status != app.DLFailed || !strings.Contains(pr.Err, "SHA-256") {
		t.Fatalf("status %v err %q", pr.Status, pr.Err)
	}
	if _, err := os.Stat(filepath.Join(mm.Dir(spec.ID), "w.gguf")); err == nil {
		t.Fatal("corrupt file was installed")
	}
	if d.PartialBytes(spec) != 0 {
		t.Fatal("corrupt partial kept")
	}
}

func TestPauseKeepsPartialAndCancelDeletes(t *testing.T) {
	data := payload(4_000_000)
	hub := &fakeHub{data: data, slow: true}
	d, _, _, wait := setup(t, hub)
	spec := testSpec(data)

	d.Start(spec)
	waitFor(t, func() bool { return d.Progress(spec).Downloaded > 100_000 })
	d.Pause(spec.ID)
	pr := wait(spec)
	if pr.Status != app.DLPaused || d.PartialBytes(spec) == 0 {
		t.Fatalf("pause: status %v partial %d", pr.Status, d.PartialBytes(spec))
	}

	d.Start(spec)
	waitFor(t, func() bool { return d.Progress(spec).Status == app.DLDownloading })
	d.Cancel(spec.ID)
	pr = wait(spec)
	if pr.Status != app.DLNone || d.PartialBytes(spec) != 0 {
		t.Fatalf("cancel: status %v partial %d", pr.Status, d.PartialBytes(spec))
	}
}

func TestRejectsUnsupportedModels(t *testing.T) {
	d, _, _, _ := setup(t, &fakeHub{})
	vlm, ok := app.FindSpec("smolvlm-256m-instruct-q8")
	if !ok {
		t.Fatal("catalog missing SmolVLM")
	}
	if err := d.Start(vlm); err == nil {
		t.Fatal("unsupported model download allowed")
	}
}

func TestCatalogIntegrity(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range app.Catalog {
		if seen[m.ID] {
			t.Errorf("duplicate id %s", m.ID)
		}
		seen[m.ID] = true
		if m.Repo == "" || len(m.Revision) != 40 {
			t.Errorf("%s: repo/revision must be pinned", m.ID)
		}
		if m.Supported && (len(m.Files) == 0 || m.Weights().Name == "" || m.Arch.Layers == 0) {
			t.Errorf("%s: supported model without weights/arch", m.ID)
		}
		if !m.Supported && m.Unsupported == "" {
			t.Errorf("%s: unsupported model needs a reason", m.ID)
		}
		for _, f := range m.Files {
			if len(f.SHA256) != 64 || f.Size <= 0 || !strings.HasSuffix(f.Name, ".gguf") {
				t.Errorf("%s: bad file %+v", m.ID, f)
			}
		}
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestCatalogMatchesHub checks every downloadable catalog entry against the
// live Hugging Face API. Opt-in: SMOLCHAT_ONLINE=1.
func TestCatalogMatchesHub(t *testing.T) {
	if os.Getenv("SMOLCHAT_ONLINE") == "" {
		t.Skip("set SMOLCHAT_ONLINE=1 to query huggingface.co")
	}
	p, _ := app.NewPaths(t.TempDir())
	d := app.NewDownloader(p, app.NewModelManager(p), nil, nil)
	for _, m := range app.Catalog {
		if len(m.Files) == 0 {
			continue
		}
		if err := d.VerifyRemote(context.Background(), m); err != nil {
			t.Errorf("%s: %v", m.ID, err)
		}
	}
}

// TestVerifyRemoteRetriesServerError ensures temporary Hub failures are retried.
func TestVerifyRemoteRetriesServerError(t *testing.T) {
        data := []byte("test model weights")
        spec := testSpec(data)

        var requests atomic.Int32
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if requests.Add(1) == 1 {
                        http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
                        return
                }

                fmt.Fprintf(w, `[{"path":"w.gguf","size":%d,"lfs":{"oid":"%s"}}]`,
                        len(data), spec.Files[0].SHA256)
        }))
        defer server.Close()

        d, _, _, _ := setup(t, &fakeHub{})
        d.TreeURL = func(app.ModelSpec) string { return server.URL }

        if err := d.VerifyRemote(context.Background(), spec); err != nil {
                t.Fatalf("VerifyRemote failed after temporary 503: %v", err)
        }

        if got := requests.Load(); got != 2 {
                t.Fatalf("got %d requests, want 2", got)
        }
}
