// Package app holds SmolChat's platform-independent application logic:
// storage, model catalog and downloads, routing and the chat controller.
// The UI (package ui) reads snapshots from App and calls its methods; all
// slow work runs on background goroutines that call Invalidate when done.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Notice is a transient message for the UI (snackbar).
type Notice struct {
	ID    int
	Text  string
	Error bool
}

// App is the application core.
type App struct {
	Paths     Paths
	Settings  *SettingsStore
	Store     *Store
	Models    *ModelManager
	Downloads *Downloader
	Runtime   *Runtime

	invalidate func()

	mu       sync.Mutex
	device   DeviceInfo
	convs    []Conversation
	search   string
	chat     chatState
	notices  []Notice
	noticeID int
	online   int // -1 unknown, 0 offline, 1 online

	themeChecked time.Time
}

// Options configure New.
type Options struct {
	DataDir    string
	Engine     InferenceEngine // nil: inference unavailable
	Invalidate func()          // request a UI redraw (any goroutine)
	KeepAlive  func(bool)      // keep process alive during downloads
}

// New initialises storage, settings, the database and model discovery.
// It does no network access.
func New(o Options) (*App, error) {
	if o.Invalidate == nil {
		o.Invalidate = func() {}
	}
	paths, err := NewPaths(o.DataDir)
	if err != nil {
		return nil, err
	}
	if f, err := os.OpenFile(filepath.Join(paths.Logs, "smolchat.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(f)
	}
	a := &App{Paths: paths, invalidate: o.Invalidate, online: -1}
	if a.Settings, err = OpenSettings(paths.Settings); err != nil {
		log.Printf("settings: %v", err)
	}
	if a.Store, err = OpenStore(paths.DB); err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	a.Models = NewModelManager(paths)
	a.Downloads = NewDownloader(paths, a.Models, a.invalidate, o.KeepAlive)
	a.Runtime = NewRuntime(o.Engine, a.invalidate)
	a.device = DetectDevice(paths.Root)
	a.refreshConversations()
	a.NewChat()
	return a, nil
}

// Close releases native and database resources.
func (a *App) Close() {
	a.StopGeneration()
	a.Downloads.PauseAll()
	a.Runtime.Unload()
	a.Store.Close()
}

// Invalidate requests a redraw.
func (a *App) Invalidate() { a.invalidate() }

// report surfaces non-nil errors as notices.
func (a *App) report(err error) {
	if err != nil {
		log.Printf("error: %v", err)
		a.Notify(err.Error(), true)
	}
}

// Notify queues a snackbar message.
func (a *App) Notify(text string, isErr bool) {
	a.mu.Lock()
	a.noticeID++
	a.notices = append(a.notices, Notice{ID: a.noticeID, Text: text, Error: isErr})
	a.mu.Unlock()
	a.invalidate()
}

// TakeNotice pops the oldest pending notice.
func (a *App) TakeNotice() (Notice, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.notices) == 0 {
		return Notice{}, false
	}
	n := a.notices[0]
	a.notices = a.notices[1:]
	return n, true
}

// Device returns the latest hardware snapshot.
func (a *App) Device() DeviceInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.device
}

// RefreshDevice re-reads RAM, storage and thermal state in the background.
func (a *App) RefreshDevice() {
	go func() {
		a.mu.Lock()
		d := a.device
		a.mu.Unlock()
		d.RefreshDynamic(a.Paths.Root)
		a.mu.Lock()
		a.device = d
		a.mu.Unlock()
		a.invalidate()
	}()
}

// RefreshSystemTheme re-reads the OS dark-mode setting in the background
// (at most every two seconds); it invalidates the UI when it changed.
func (a *App) RefreshSystemTheme() {
	a.mu.Lock()
	if time.Since(a.themeChecked) < 2*time.Second {
		a.mu.Unlock()
		return
	}
	a.themeChecked = time.Now()
	a.mu.Unlock()
	go func() {
		dark := systemDark()
		a.mu.Lock()
		changed := a.device.SystemDark != dark
		a.device.SystemDark = dark
		a.mu.Unlock()
		if changed {
			a.invalidate()
		}
	}()
}

// Online reports connectivity to the Hub: -1 unknown, 0 offline, 1 online.
func (a *App) Online() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.online
}

// CheckOnline probes huggingface.co in the background (no data is sent).
func (a *App) CheckOnline() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		v := 0
		var d net.Dialer
		if c, err := d.DialContext(ctx, "tcp", "huggingface.co:443"); err == nil {
			c.Close()
			v = 1
		}
		a.mu.Lock()
		a.online = v
		a.mu.Unlock()
		a.invalidate()
	}()
}

// OnBackground is called when Android moves the app to the background.
func (a *App) OnBackground() { a.CheckpointStreaming() }

// OnForeground is called when the app returns.
func (a *App) OnForeground() {
	a.Models.Scan()
	a.RefreshDevice()
}

// Conversations returns the (filtered) sidebar list.
func (a *App) Conversations() ([]Conversation, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Conversation(nil), a.convs...), a.search
}

// SetSearch filters the conversation list.
func (a *App) SetSearch(q string) {
	a.mu.Lock()
	a.search = q
	a.mu.Unlock()
	a.refreshConversations()
}

func (a *App) refreshConversations() {
	a.mu.Lock()
	q := a.search
	a.mu.Unlock()
	cs, err := a.Store.SearchConversations(q)
	if err != nil {
		a.report(err)
		return
	}
	a.mu.Lock()
	a.convs = cs
	a.mu.Unlock()
	a.invalidate()
}

// RenameConversation renames id.
func (a *App) RenameConversation(id, title string) {
	if err := a.Store.RenameConversation(id, title); err != nil {
		a.report(err)
		return
	}
	a.mu.Lock()
	if a.chat.view.Conv.ID == id {
		a.chat.view.Conv.Title = title
	}
	a.mu.Unlock()
	a.refreshConversations()
}

// DeleteConversation deletes id; the open chat resets if it was id.
func (a *App) DeleteConversation(id string) {
	a.mu.Lock()
	open := a.chat.view.Conv.ID == id
	a.mu.Unlock()
	if open {
		a.NewChat()
	}
	a.report(a.Store.DeleteConversation(id))
	a.refreshConversations()
}

// DeleteAllConversations clears the history.
func (a *App) DeleteAllConversations() {
	a.NewChat()
	a.report(a.Store.DeleteAllConversations())
	a.refreshConversations()
}

// StartDownload downloads id.
func (a *App) StartDownload(id string) {
	spec, ok := FindSpec(id)
	if !ok {
		return
	}
	if err := a.Downloads.Start(spec); err != nil {
		a.report(err)
	}
}

// DeleteModel unloads (if resident) and deletes id's files.
func (a *App) DeleteModel(id string) {
	go func() {
		a.mu.Lock()
		streamingWith := a.chat.view.Streaming && a.Runtime.Status().ModelID == id
		a.mu.Unlock()
		if streamingWith {
			a.StopGeneration()
		}
		if err := a.Runtime.UnloadIf(id); err != nil {
			a.report(err)
			return
		}
		if err := a.Models.Delete(id); err != nil {
			a.report(err)
		}
		if s := a.Settings.Get(); s.ManualModelID == id {
			a.Settings.Update(func(s *Settings) { s.SelectionMode, s.ManualModelID = SelectAuto, "" })
		}
		a.RefreshDevice()
		a.invalidate()
	}()
}

// Prewarm loads id in the background so the next message starts quickly.
func (a *App) Prewarm(id string) {
	spec, ok := FindSpec(id)
	if !ok || !a.Models.IsInstalled(id) {
		return
	}
	s := a.Settings.Get()
	go func() {
		err := a.Runtime.Ensure(context.Background(), ModelConfig{ID: id, Path: a.Models.WeightsPath(spec), ContextSize: s.ContextSize, Threads: s.Threads})
		if err != nil && !errors.Is(err, context.Canceled) {
			a.report(fmt.Errorf("load %s: %w", spec.Name, err))
		}
	}()
}

// ApplySettings persists s; a changed context size reloads the model lazily.
func (a *App) ApplySettings(f func(*Settings)) {
	before := a.Settings.Get()
	a.report(a.Settings.Update(f))
	after := a.Settings.Get()
	if before.ContextSize != after.ContextSize || before.Threads != after.Threads {
		go a.Runtime.Release() // next message reloads with the new settings
	}
	a.invalidate()
}
