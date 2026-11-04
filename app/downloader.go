package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// DownloadStatus is the state of a model download.
type DownloadStatus int

const (
	DLNone DownloadStatus = iota
	DLQueued
	DLDownloading
	DLVerifying
	DLDone
	DLFailed
	DLPaused // stopped with a resumable partial file on disk
)

// DownloadProgress is a snapshot of one model download.
type DownloadProgress struct {
	ModelID     string
	Status      DownloadStatus
	Downloaded  int64
	Total       int64
	BytesPerSec float64
	Err         string
}

// Active reports whether the download is still running.
func (p DownloadProgress) Active() bool {
	return p.Status == DLQueued || p.Status == DLDownloading || p.Status == DLVerifying
}

// Fraction is the completed share in 0..1.
func (p DownloadProgress) Fraction() float32 {
	if p.Total <= 0 {
		return 0
	}
	return float32(float64(p.Downloaded) / float64(p.Total))
}

// ErrInsufficientStorage is returned when the volume is too full.
var ErrInsufficientStorage = errors.New("not enough free storage")

// errPermanent marks failures that retrying will not fix.
type errPermanent struct{ error }

// Downloader fetches model files from the Hugging Face Hub into the
// app-private model directory.
type Downloader struct {
	paths  Paths
	models *ModelManager
	client *http.Client
	// URLFor maps a file to its download URL (overridable in tests).
	URLFor func(ModelSpec, ModelFile) string
	// TreeURL returns the Hub API listing for the pinned revision, used to
	// confirm the catalog still matches the repository before downloading.
	// Nil skips the check.
	TreeURL func(ModelSpec) string
	// Retries per file before a download is reported as failed.
	Retries   int
	onChange  func()
	keepAlive func(active bool)

	mu   sync.Mutex
	jobs map[string]*dlJob
}

type dlJob struct {
	cancel  context.CancelFunc
	discard bool // delete partial files when stopping (cancel vs pause)
	prog    DownloadProgress
}

// NewDownloader creates a downloader. onChange is called on progress updates;
// keepAlive(true/false) is called when the first download starts and the last
// one ends, so the platform can keep the process alive (Android foreground
// service).
func NewDownloader(p Paths, m *ModelManager, onChange func(), keepAlive func(bool)) *Downloader {
	if onChange == nil {
		onChange = func() {}
	}
	if keepAlive == nil {
		keepAlive = func(bool) {}
	}
	return &Downloader{
		paths:  p,
		models: m,
		client: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				IdleConnTimeout:       60 * time.Second,
			},
		},
		URLFor: ModelSpec.FileURL,
		TreeURL: func(m ModelSpec) string {
			return fmt.Sprintf("https://huggingface.co/api/models/%s/tree/%s", m.Repo, m.Revision)
		},
		Retries:   4,
		onChange:  onChange,
		keepAlive: keepAlive,
		jobs:      map[string]*dlJob{},
	}
}

func (d *Downloader) partDir(id string) string { return filepath.Join(d.paths.Tmp, id) }

// PartialBytes is how much of id is already on disk from an earlier,
// interrupted download.
func (d *Downloader) PartialBytes(spec ModelSpec) int64 {
	var n int64
	for _, f := range spec.Files {
		if st, err := os.Stat(filepath.Join(d.models.Dir(spec.ID), f.Name)); err == nil {
			n += st.Size()
		} else if st, err := os.Stat(filepath.Join(d.partDir(spec.ID), f.Name+".part")); err == nil {
			n += st.Size()
		}
	}
	return n
}

// Progress returns the state of id's download. Interrupted downloads from a
// previous run are reported as DLPaused.
func (d *Downloader) Progress(spec ModelSpec) DownloadProgress {
	d.mu.Lock()
	j, ok := d.jobs[spec.ID]
	d.mu.Unlock()
	if ok {
		d.mu.Lock()
		defer d.mu.Unlock()
		return j.prog
	}
	if d.models.IsInstalled(spec.ID) {
		return DownloadProgress{ModelID: spec.ID, Status: DLDone, Downloaded: spec.DownloadSize(), Total: spec.DownloadSize()}
	}
	if n := d.PartialBytes(spec); n > 0 {
		return DownloadProgress{ModelID: spec.ID, Status: DLPaused, Downloaded: n, Total: spec.DownloadSize()}
	}
	return DownloadProgress{ModelID: spec.ID}
}

// AnyActive reports whether any download is running.
func (d *Downloader) AnyActive() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, j := range d.jobs {
		if j.prog.Active() {
			return true
		}
	}
	return false
}

// Start begins (or resumes) downloading spec in the background.
func (d *Downloader) Start(spec ModelSpec) error {
	if !spec.Supported || len(spec.Files) == 0 {
		return fmt.Errorf("%s cannot run in SmolChat yet", spec.Name)
	}
	if d.models.IsInstalled(spec.ID) {
		return nil
	}
	need := spec.DownloadSize() - d.PartialBytes(spec) + 64<<20
	if free := FreeSpace(d.paths.Root); free > 0 && free < need {
		return fmt.Errorf("%w: %s needs %s, %s free", ErrInsufficientStorage, spec.Name, HumanBytes(need), HumanBytes(free))
	}

	d.mu.Lock()
	if j, ok := d.jobs[spec.ID]; ok && j.prog.Active() {
		d.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &dlJob{cancel: cancel, prog: DownloadProgress{ModelID: spec.ID, Status: DLQueued, Total: spec.DownloadSize()}}
	d.jobs[spec.ID] = j
	first := d.activeLocked() == 1
	d.mu.Unlock()
	if first {
		d.keepAlive(true)
	}
	d.onChange()

	go d.run(ctx, j, spec)
	return nil
}

func (d *Downloader) activeLocked() int {
	n := 0
	for _, j := range d.jobs {
		if j.prog.Active() {
			n++
		}
	}
	return n
}

// Cancel stops id and deletes its partial files.
func (d *Downloader) Cancel(id string) { d.stop(id, true) }

// Pause stops id but keeps partial files for a later resume.
func (d *Downloader) Pause(id string) { d.stop(id, false) }

func (d *Downloader) stop(id string, discard bool) {
	d.mu.Lock()
	j, ok := d.jobs[id]
	if ok {
		j.discard = discard
	}
	d.mu.Unlock()
	if ok {
		j.cancel()
		return
	}
	if discard {
		os.RemoveAll(d.partDir(id))
		d.onChange()
	}
}

// PauseAll pauses every running download (e.g. before the process exits).
func (d *Downloader) PauseAll() {
	d.mu.Lock()
	var ids []string
	for id, j := range d.jobs {
		if j.prog.Active() {
			ids = append(ids, id)
		}
	}
	d.mu.Unlock()
	for _, id := range ids {
		d.Pause(id)
	}
}

func (d *Downloader) update(j *dlJob, f func(p *DownloadProgress)) {
	d.mu.Lock()
	f(&j.prog)
	d.mu.Unlock()
	d.onChange()
}

func (d *Downloader) run(ctx context.Context, j *dlJob, spec ModelSpec) {
	err := d.download(ctx, j, spec)
	if err == nil {
		err = d.models.register(spec)
	}
	d.mu.Lock()
	discard := j.discard
	switch {
	case err == nil:
		j.prog.Status = DLDone
		j.prog.Downloaded = j.prog.Total
		os.RemoveAll(d.partDir(spec.ID))
	case ctx.Err() != nil && discard:
		j.prog = DownloadProgress{ModelID: spec.ID}
		os.RemoveAll(d.partDir(spec.ID))
	case ctx.Err() != nil:
		j.prog.Status = DLPaused
		j.prog.BytesPerSec = 0
	default:
		j.prog.Status = DLFailed
		j.prog.Err = err.Error()
		j.prog.BytesPerSec = 0
	}
	// Finished jobs are forgotten so Progress falls back to disk state;
	// failures stay visible until retried.
	if j.prog.Status != DLFailed {
		delete(d.jobs, spec.ID)
	}
	last := d.activeLocked() == 0
	d.mu.Unlock()
	if last {
		d.keepAlive(false)
	}
	d.onChange()
}

func (d *Downloader) download(ctx context.Context, j *dlJob, spec ModelSpec) error {
	if err := os.MkdirAll(d.partDir(spec.ID), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(d.models.Dir(spec.ID), 0o755); err != nil {
		return err
	}
	if d.TreeURL != nil {
		if err := d.VerifyRemote(ctx, spec); err != nil {
			return err
		}
	}
	var doneBefore int64 // bytes of files already completed
	for _, f := range spec.Files {
		final := filepath.Join(d.models.Dir(spec.ID), f.Name)
		if st, err := os.Stat(final); err == nil && st.Size() == f.Size {
			doneBefore += f.Size // verified when it was moved into place
			continue
		}
		base := doneBefore
		err := d.downloadFile(ctx, j, spec, f, final, func(n int64) {
			d.update(j, func(p *DownloadProgress) { p.Downloaded = base + n })
		})
		if err != nil {
			return err
		}
		doneBefore += f.Size
	}
	return nil
}

// VerifyRemote checks the Hub's file listing for the pinned revision against
// the catalog (names, sizes, LFS SHA-256), so a moved or replaced file is
// caught before any bytes are downloaded.
func (d *Downloader) VerifyRemote(ctx context.Context, spec ModelSpec) error {
	var lastErr error
	for attempt := 0; attempt <= d.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(1<<(attempt-1)) * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.TreeURL(spec), nil)
		if err != nil {
			return err
		}
		resp, err := d.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			continue
		}
		var tree []struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
			LFS  *struct {
				OID string `json:"oid"`
			} `json:"lfs"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&tree)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Hugging Face repository %s is unavailable (HTTP %d)", spec.Repo, resp.StatusCode)
		}
		if err != nil {
			lastErr = err
			continue
		}
		for _, f := range spec.Files {
			found := false
			for _, t := range tree {
				if t.Path != f.Name {
					continue
				}
				found = true
				if t.Size != f.Size || (t.LFS != nil && f.SHA256 != "" && t.LFS.OID != f.SHA256) {
					return fmt.Errorf("%s on Hugging Face no longer matches the verified catalog entry", f.Name)
				}
			}
			if !found {
				return fmt.Errorf("%s was not found in %s", f.Name, spec.Repo)
			}
		}
		return nil
	}
	return fmt.Errorf("couldn't reach Hugging Face, tap to retry: %w", lastErr)
}

// downloadFile fetches one file with resume + retries and verifies it.
func (d *Downloader) downloadFile(ctx context.Context, j *dlJob, spec ModelSpec, f ModelFile, final string, progress func(int64)) error {
	part := filepath.Join(d.partDir(spec.ID), f.Name+".part")
	var lastErr error
	for attempt := 0; attempt <= d.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(1<<(attempt-1)) * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		h, have, err := hashExisting(part)
		if err != nil {
			return err
		}
		progress(have)
		err = d.fetch(ctx, j, d.URLFor(spec, f), part, have, f.Size, h, progress)
		if err == nil {
			d.update(j, func(p *DownloadProgress) { p.Status = DLVerifying })
			if f.SHA256 != "" && hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
				os.Remove(part)
				return fmt.Errorf("%s failed its SHA-256 check; the download was discarded", f.Name)
			}
			return os.Rename(part, final)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var perm errPermanent
		if errors.As(err, &perm) {
			return perm.error
		}
		lastErr = err
	}
	return fmt.Errorf("network error, tap to resume: %w", lastErr)
}

// hashExisting hashes an existing partial file so verification can continue
// incrementally after a resume.
func hashExisting(part string) (hash.Hash, int64, error) {
	h := sha256.New()
	fp, err := os.Open(part)
	if errors.Is(err, os.ErrNotExist) {
		return h, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer fp.Close()
	n, err := io.Copy(h, fp)
	return h, n, err
}

func (d *Downloader) fetch(ctx context.Context, j *dlJob, url, part string, have, size int64, h hash.Hash, progress func(int64)) error {
	if size > 0 && have == size {
		return nil
	}
	if size > 0 && have > size { // stale or foreign file
		os.Remove(part)
		h.Reset()
		have = 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errPermanent{err}
	}
	req.Header.Set("User-Agent", AppName+"/"+AppVersion+" ("+OrgName+"; offline LLM chat)")
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	flags := os.O_CREATE | os.O_WRONLY
	switch {
	case resp.StatusCode == http.StatusPartialContent && have > 0:
		flags |= os.O_APPEND
	case resp.StatusCode == http.StatusOK:
		flags |= os.O_TRUNC // server ignored Range: start over
		h.Reset()
		have = 0
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		os.Remove(part)
		return errors.New("server rejected resume; restarting")
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return errPermanent{fmt.Errorf("file not available on Hugging Face (HTTP %d)", resp.StatusCode)}
	default:
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return errPermanent{err}
	}
	defer out.Close()
	d.update(j, func(p *DownloadProgress) { p.Status = DLDownloading })

	buf := make([]byte, 256<<10)
	written := have
	lastReport, lastBytes := time.Now(), written
	var speed float64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				if errors.Is(werr, syscall.ENOSPC) {
					return errPermanent{fmt.Errorf("%w while writing %s", ErrInsufficientStorage, filepath.Base(part))}
				}
				return errPermanent{werr}
			}
			h.Write(buf[:n])
			written += int64(n)
			if dt := time.Since(lastReport); dt >= 300*time.Millisecond {
				inst := float64(written-lastBytes) / dt.Seconds()
				if speed == 0 {
					speed = inst
				} else {
					speed = 0.7*speed + 0.3*inst
				}
				lastReport, lastBytes = time.Now(), written
				s := speed
				d.update(j, func(p *DownloadProgress) { p.Status = DLDownloading; p.BytesPerSec = s })
				progress(written)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	progress(written)
	if size > 0 && written != size {
		return fmt.Errorf("incomplete download (%d of %d bytes)", written, size)
	}
	return out.Sync()
}

// HumanBytes formats a byte count like "386 MB".
func HumanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	v, suffix := float64(n), []string{"KB", "MB", "GB", "TB"}
	i := -1
	for v >= unit && i < len(suffix)-1 {
		v /= unit
		i++
	}
	if v >= 100 || i < 2 {
		return fmt.Sprintf("%.0f %s", v, suffix[i])
	}
	return fmt.Sprintf("%.1f %s", v, suffix[i])
}
