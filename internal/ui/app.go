package ui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/jesusuzcategui/soul-app/internal/killer"
	"github.com/jesusuzcategui/soul-app/internal/model"
	"github.com/jesusuzcategui/soul-app/internal/scanner"
)

const (
	// DefaultWindowTitle defines the primary application window title.
	DefaultWindowTitle = "Soul App"

	// DefaultWindowWidth defines the initial window width in pixels.
	DefaultWindowWidth = float32(700)

	// DefaultWindowHeight defines the initial window height in pixels.
	DefaultWindowHeight = float32(500)

	// DefaultPollInterval defines how frequently the background scanner runs.
	DefaultPollInterval = 3 * time.Second

	// DefaultSearchDebounce defines the delay before triggering a search filter update.
	DefaultSearchDebounce = 100 * time.Millisecond
)

// viewer defines the subset of View operations called from App's goroutines.
// It is an interface so tests can inject a no-op implementation that avoids
// touching Fyne widgets from background goroutines.
type viewer interface {
	SetScanning(bool)
	SetStatus(string)
	UpdateList([]model.PortProcess)
}

// App manages the main GUI lifecycle, background scanner polling, and process interactions.
type App struct {
	fyneApp fyne.App
	window  fyne.Window
	scanner scanner.Scanner
	killer  killer.Killer

	state *State
	view  viewer

	pollInterval time.Duration
	pollTicker   *time.Ticker
	stopPoll     chan struct{}
	pollMu       sync.Mutex
	pollWg       sync.WaitGroup

	isScanning atomic.Bool
	isQuitting atomic.Bool
	shutdownMu sync.Mutex
	scanWg     sync.WaitGroup

	debounceTimer *time.Timer
	debounceMu    sync.Mutex

	onScanComplete func([]model.PortProcess)
	onQuit         func()
	tray           *TrayController
}

// New creates and initializes a new Soul App using standard Fyne GUI drivers.
func New(s scanner.Scanner, k killer.Killer) *App {
	fa := fyneapp.NewWithID("com.jesusuzcategui.soul")
	return NewWithFyneApp(fa, s, k)
}

// NewWithFyneApp initializes a new Soul App with an injected fyne.App instance (for tests or custom drivers).
func NewWithFyneApp(fa fyne.App, s scanner.Scanner, k killer.Killer) *App {
	if s == nil {
		s = scanner.New()
	}
	if k == nil {
		k = killer.New()
	}

	appInstance := &App{
		fyneApp:      fa,
		scanner:      s,
		killer:       k,
		state:        NewState(),
		pollInterval: DefaultPollInterval,
		onQuit:       fa.Quit,
	}

	fa.SetIcon(appIcon)
	appInstance.buildUI()
	return appInstance
}

// buildUI configures the main window and associates views.
func (a *App) buildUI() {
	a.window = a.fyneApp.NewWindow(DefaultWindowTitle)
	a.window.Resize(fyne.NewSize(DefaultWindowWidth, DefaultWindowHeight))
	a.window.SetIcon(appIcon)

	concreteView := NewView(
		a.OnSearch,
		a.TriggerRefresh,
		a.PromptKill,
	)
	a.view = concreteView

	a.window.SetContent(concreteView.Root)
	a.configureTray()
}

// configureTray enables background tray behavior only for drivers that expose
// the Fyne desktop tray API. Test, mobile, and other drivers retain normal
// window-close behavior.
func (a *App) configureTray() {
	desktopApp, ok := a.fyneApp.(desktop.App)
	if !ok {
		return
	}

	a.tray = NewTrayController(a.window, a.TriggerRefresh, a.Quit)
	a.tray.InstallCloseIntercept()
	desktopApp.SetSystemTrayIcon(appIcon)
	desktopApp.SetSystemTrayMenu(a.tray.Menu)
}

// Run launches the application, starts background polling, and blocks until the window is closed.
func (a *App) Run() {
	a.StartPolling()
	a.Refresh()
	a.window.ShowAndRun()
}

// StartPolling initiates the background ticker to scan for listening ports every pollInterval.
func (a *App) StartPolling() {
	a.shutdownMu.Lock()
	defer a.shutdownMu.Unlock()
	if a.isQuitting.Load() {
		return
	}

	a.pollMu.Lock()
	defer a.pollMu.Unlock()

	if a.pollTicker != nil {
		return
	}

	a.pollTicker = time.NewTicker(a.pollInterval)
	a.stopPoll = make(chan struct{})
	ticker := a.pollTicker
	stopPoll := a.stopPoll

	a.pollWg.Add(1)
	go func() {
		defer a.pollWg.Done()
		for {
			select {
			case <-ticker.C:
				a.Refresh()
			case <-stopPoll:
				return
			}
		}
	}()
}

// StopPolling cancels and terminates background scanner polling and waits for active scans to finish.
func (a *App) StopPolling() {
	a.pollMu.Lock()
	if a.pollTicker != nil {
		a.pollTicker.Stop()
		a.pollTicker = nil
	}
	if a.stopPoll != nil {
		close(a.stopPoll)
		a.stopPoll = nil
	}
	a.pollMu.Unlock()

	a.pollWg.Wait()
	a.scanWg.Wait()
}

// Quit terminates polling and pending UI work before asking Fyne to exit.
func (a *App) Quit() {
	a.shutdownMu.Lock()
	if !a.isQuitting.CompareAndSwap(false, true) {
		a.shutdownMu.Unlock()
		return
	}
	a.shutdownMu.Unlock()

	a.StopPolling()

	a.debounceMu.Lock()
	if a.debounceTimer != nil {
		a.debounceTimer.Stop()
		a.debounceTimer = nil
	}
	a.debounceMu.Unlock()

	if a.onQuit != nil {
		a.onQuit()
	}
}

// WaitForScan blocks until any currently in-flight scan completes.
func (a *App) WaitForScan() {
	a.scanWg.Wait()
}

// TriggerRefresh triggers an asynchronous scan manually (e.g. from the Refresh button).
func (a *App) TriggerRefresh() {
	a.Refresh()
}

// Refresh performs a non-blocking background network socket scan.
func (a *App) Refresh() {
	a.shutdownMu.Lock()
	defer a.shutdownMu.Unlock()
	if a.isQuitting.Load() {
		return
	}

	// Prevent duplicate concurrent scans.
	if !a.isScanning.CompareAndSwap(false, true) {
		return
	}

	a.scanWg.Add(1)
	// The goroutine is counted before releasing shutdownMu, so Quit cannot
	// begin waiting for scans until this work is observable.
	go func() {
		defer func() {
			a.isScanning.Store(false)
			a.scanWg.Done()
		}()

		a.view.SetScanning(true)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		procs, err := a.scanner.Scan(ctx)

		a.view.SetScanning(false)

		if err != nil {
			msg := fmt.Sprintf("Scan failed: %v", err)
			a.view.SetStatus(msg)
			return
		}

		filtered := a.state.SetProcesses(procs)

		query := a.state.GetQuery()
		var statusMsg string
		if query != "" {
			statusMsg = fmt.Sprintf("Filtered: %d of %d active ports matching %q (%s)",
				len(filtered), len(procs), query, time.Now().Format("15:04:05"))
		} else {
			statusMsg = fmt.Sprintf("Ready • %d active listening ports (%s)",
				len(procs), time.Now().Format("15:04:05"))
		}

		a.view.UpdateList(filtered)
		a.view.SetStatus(statusMsg)

		if a.onScanComplete != nil {
			a.onScanComplete(procs)
		}
	}()
}

// OnSearch handles search query changes with a debounce to prevent excessive UI recalculation.
func (a *App) OnSearch(query string) {
	a.debounceMu.Lock()
	defer a.debounceMu.Unlock()

	if a.debounceTimer != nil {
		a.debounceTimer.Stop()
	}

	a.debounceTimer = time.AfterFunc(DefaultSearchDebounce, func() {
		filtered := a.state.SetQuery(query)

		fCount, tCount := a.state.Count()
		var statusMsg string
		if query != "" {
			statusMsg = fmt.Sprintf("Filtered: %d of %d active ports matching %q", fCount, tCount, query)
		} else {
			statusMsg = fmt.Sprintf("Ready • %d active listening ports", tCount)
		}

		a.view.UpdateList(filtered)
		a.view.SetStatus(statusMsg)
	})
}

// PromptKill displays a confirmation dialog before terminating the targeted process.
func (a *App) PromptKill(proc model.PortProcess) {
	if proc.PID <= 1 {
		dialog.ShowError(errors.New("cannot terminate system or unprivileged process (PID <= 1)"), a.window)
		return
	}

	dialog.ShowConfirm(
		"Kill Process",
		fmt.Sprintf("Are you sure you want to terminate %s (PID %d) listening on port %d?",
			proc.Name, proc.PID, proc.Port),
		func(confirmed bool) {
			if !confirmed {
				return
			}
			a.ExecuteKill(proc, false)
		},
		a.window,
	)
}

// ExecuteKill attempts to terminate a process gracefully (or forced), handling errors with user notifications.
func (a *App) ExecuteKill(proc model.PortProcess, force bool) {
	a.view.SetStatus(fmt.Sprintf("Terminating %s (PID %d)...", proc.Name, proc.PID))

	go func() {
		err := a.killer.Kill(proc.PID, force)
		if err == nil {
			a.view.SetStatus(fmt.Sprintf("Successfully terminated %s (PID %d)", proc.Name, proc.PID))
			a.Refresh()
			return
		}

		if errors.Is(err, killer.ErrPermissionDenied) {
			a.view.SetStatus(fmt.Sprintf("Permission denied killing PID %d", proc.PID))
			dialog.ShowError(
				fmt.Errorf("Permission Denied: Run Soul App with elevated permissions (e.g. sudo) to terminate %s (PID %d)", proc.Name, proc.PID),
				a.window,
			)
			return
		}

		if errors.Is(err, killer.ErrProcessAlreadyDead) {
			a.view.SetStatus(fmt.Sprintf("Process %d is already terminated", proc.PID))
			a.Refresh()
			return
		}

		// If graceful termination failed, prompt the user for forced termination (SIGKILL).
		if !force {
			dialog.ShowConfirm(
				"Termination Failed",
				fmt.Sprintf("Graceful termination (SIGTERM) of %s (PID %d) failed:\n%v\n\nWould you like to force kill (SIGKILL)?",
					proc.Name, proc.PID, err),
				func(forceConfirmed bool) {
					if forceConfirmed {
						a.ExecuteKill(proc, true)
					}
				},
				a.window,
			)
		} else {
			dialog.ShowError(
				fmt.Errorf("Failed to force kill %s (PID %d): %w", proc.Name, proc.PID, err),
				a.window,
			)
		}
	}()
}

// Window returns the main application window.
func (a *App) Window() fyne.Window {
	return a.window
}

// FyneApp returns the underlying Fyne App instance.
func (a *App) FyneApp() fyne.App {
	return a.fyneApp
}

// State returns the application state holder.
func (a *App) State() *State {
	return a.state
}

// View returns the concrete application view when the standard UI is configured.
// It returns nil for headless test configurations that inject a non-Fyne viewer.
func (a *App) View() *View {
	view, _ := a.view.(*View)
	return view
}
