package ui

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/jesusuzcategui/soul-app/internal/killer"
	"github.com/jesusuzcategui/soul-app/internal/model"
)

type mockScanner struct {
	scanFn func(ctx context.Context) ([]model.PortProcess, error)
}

func (m *mockScanner) Scan(ctx context.Context) ([]model.PortProcess, error) {
	if m.scanFn != nil {
		return m.scanFn(ctx)
	}
	return nil, nil
}

type mockKiller struct {
	killFn func(pid int32, force bool) error
}

// noopViewer implements viewer with no-ops, allowing tests to exercise App
// logic without involving Fyne widgets (which are not goroutine-safe in the
// Fyne test driver).
type noopViewer struct{}

func (noopViewer) SetScanning(bool)               {}
func (noopViewer) SetStatus(string)               {}
func (noopViewer) UpdateList([]model.PortProcess) {}

func (m *mockKiller) Kill(pid int32, force bool) error {
	if m.killFn != nil {
		return m.killFn(pid, force)
	}
	return nil
}

func TestApp_Initialization(t *testing.T) {
	testApp := test.NewApp()
	mockS := &mockScanner{}
	mockK := &mockKiller{}

	app := NewWithFyneApp(testApp, mockS, mockK)

	if app.Window() == nil {
		t.Fatal("expected non-nil Window")
	}
	if app.Window().Title() != WindowTitle() {
		t.Fatalf("expected title %q, got %q", WindowTitle(), app.Window().Title())
	}
	if app.State() == nil {
		t.Fatal("expected non-nil State")
	}
	if app.View() == nil {
		t.Fatal("expected non-nil View")
	}
	if app.FyneApp() != testApp {
		t.Fatal("expected matching FyneApp instance")
	}
}

func TestApp_SetsApplicationAndWindowIcon(t *testing.T) {
	// Fyne's test app no-ops SetIcon and its window delegates Icon() to the
	// current app, so a recording fake is used to assert the wiring.
	_ = test.NewApp() // install a current app for widget construction

	rec := newRecordingApp()
	app := NewWithFyneApp(rec, &mockScanner{}, &mockKiller{})

	if rec.icon == nil {
		t.Fatal("expected application icon to be set")
	}
	if rec.window == nil || rec.window.icon == nil {
		t.Fatal("expected window icon to be set")
	}
	if rec.trayIcon == nil {
		t.Fatal("expected system tray icon to be set")
	}
	if app.Window().Icon() == nil {
		t.Fatal("expected window to report an icon")
	}
}

// recordingWindow captures SetIcon calls from the application.
type recordingWindow struct {
	fyne.Window
	icon fyne.Resource
}

func (w *recordingWindow) SetIcon(r fyne.Resource)      { w.icon = r }
func (w *recordingWindow) Icon() fyne.Resource          { return w.icon }
func (w *recordingWindow) Resize(fyne.Size)             {}
func (w *recordingWindow) SetContent(fyne.CanvasObject) {}
func (w *recordingWindow) SetCloseIntercept(func())     {}

// recordingApp implements both fyne.App and desktop.App, recording icon and
// tray configuration so wiring can be verified without a real driver.
type recordingApp struct {
	fyne.App

	icon     fyne.Resource
	trayIcon fyne.Resource
	trayMenu *fyne.Menu
	window   *recordingWindow
}

func newRecordingApp() *recordingApp { return &recordingApp{} }

func (a *recordingApp) SetIcon(r fyne.Resource) { a.icon = r }
func (a *recordingApp) Icon() fyne.Resource     { return a.icon }

func (a *recordingApp) NewWindow(string) fyne.Window {
	a.window = &recordingWindow{}
	return a.window
}

func (a *recordingApp) Quit() {}

func (a *recordingApp) SetSystemTrayIcon(r fyne.Resource) { a.trayIcon = r }
func (a *recordingApp) SetSystemTrayMenu(m *fyne.Menu)    { a.trayMenu = m }

func TestApp_Refresh(t *testing.T) {
	testApp := test.NewApp()
	procs := sampleProcesses()

	var scanCalled atomic.Bool
	doneCh := make(chan struct{}, 1)

	mockS := &mockScanner{
		scanFn: func(ctx context.Context) ([]model.PortProcess, error) {
			scanCalled.Store(true)
			return procs, nil
		},
	}
	mockK := &mockKiller{}

	app := NewWithFyneApp(testApp, mockS, mockK)
	app.onScanComplete = func(p []model.PortProcess) {
		select {
		case doneCh <- struct{}{}:
		default:
		}
	}

	app.Refresh()

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scan to complete")
	}

	if !scanCalled.Load() {
		t.Fatal("expected scanner.Scan to be invoked")
	}

	filteredCount, totalCount := app.State().Count()
	if totalCount != len(procs) || filteredCount != len(procs) {
		t.Fatalf("expected %d procs in state, got %d/%d", len(procs), filteredCount, totalCount)
	}
}

func TestApp_Refresh_ErrorHandling(t *testing.T) {
	testApp := test.NewApp()
	scanErr := errors.New("network socket inspection failure")

	doneCh := make(chan struct{}, 1)

	mockS := &mockScanner{
		scanFn: func(ctx context.Context) ([]model.PortProcess, error) {
			defer func() {
				select {
				case doneCh <- struct{}{}:
				default:
				}
			}()
			return nil, scanErr
		},
	}
	mockK := &mockKiller{}

	app := NewWithFyneApp(testApp, mockS, mockK)
	app.Refresh()

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scan error handler")
	}

	app.WaitForScan()
	if app.View().StatusLabel.Text == "Ready" {
		t.Fatal("expected status label to reflect scan error")
	}
}

func TestApp_OnSearch_Debounce(t *testing.T) {
	testApp := test.NewApp()
	procs := sampleProcesses()

	mockS := &mockScanner{}
	mockK := &mockKiller{}

	app := NewWithFyneApp(testApp, mockS, mockK)
	app.State().SetProcesses(procs)

	// Trigger debounce search
	app.OnSearch("node")

	// Immediately, debounce shouldn't have fired yet
	// Wait slightly longer than DefaultSearchDebounce
	time.Sleep(DefaultSearchDebounce + 50*time.Millisecond)

	filtered := app.State().GetFiltered()
	if len(filtered) != 1 || filtered[0].Name != "node" {
		t.Fatalf("expected 1 process 'node' after debounce, got %d", len(filtered))
	}
}

func TestApp_PromptKill_InvalidPID(t *testing.T) {
	testApp := test.NewApp()
	var killCalled atomic.Bool

	mockS := &mockScanner{}
	mockK := &mockKiller{
		killFn: func(pid int32, force bool) error {
			killCalled.Store(true)
			return nil
		},
	}

	app := NewWithFyneApp(testApp, mockS, mockK)

	// PID 1 should be rejected immediately without calling killer
	app.PromptKill(model.PortProcess{PID: 1, Port: 80, Name: "systemd"})
	if killCalled.Load() {
		t.Fatal("killer should not have been called for PID 1")
	}

	// PID 0 should be rejected
	app.PromptKill(model.PortProcess{PID: 0, Port: 80, Name: "kernel"})
	if killCalled.Load() {
		t.Fatal("killer should not have been called for PID 0")
	}
}

func TestApp_ExecuteKill_Success(t *testing.T) {
	testApp := test.NewApp()

	var killedPID int32
	var killedForce bool
	scanDone := make(chan struct{}, 1)

	mockS := &mockScanner{
		scanFn: func(ctx context.Context) ([]model.PortProcess, error) {
			select {
			case scanDone <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}

	mockK := &mockKiller{
		killFn: func(pid int32, force bool) error {
			killedPID = pid
			killedForce = force
			return nil
		},
	}

	app := NewWithFyneApp(testApp, mockS, mockK)
	proc := model.PortProcess{PID: 3456, Port: 8080, Name: "devserver"}

	app.ExecuteKill(proc, false)

	// Success triggers background refresh
	select {
	case <-scanDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expected refresh to be called after successful kill")
	}

	if killedPID != 3456 {
		t.Fatalf("expected killed PID 3456, got %d", killedPID)
	}
	if killedForce {
		t.Fatal("expected graceful kill (force=false)")
	}
}

func TestApp_ExecuteKill_PermissionDenied(t *testing.T) {
	testApp := test.NewApp()

	mockS := &mockScanner{}
	mockK := &mockKiller{
		killFn: func(pid int32, force bool) error {
			return killer.ErrPermissionDenied
		},
	}

	app := NewWithFyneApp(testApp, mockS, mockK)
	proc := model.PortProcess{PID: 9999, Port: 443, Name: "root-proxy"}

	app.ExecuteKill(proc, false)

	// Wait for goroutine
	time.Sleep(50 * time.Millisecond)

	if app.View().StatusLabel.Text != "Permission denied killing PID 9999" {
		t.Fatalf("expected permission denied status, got %q", app.View().StatusLabel.Text)
	}
}

func TestApp_ExecuteKill_ProcessAlreadyDead(t *testing.T) {
	testApp := test.NewApp()
	scanDone := make(chan struct{}, 1)

	mockS := &mockScanner{
		scanFn: func(ctx context.Context) ([]model.PortProcess, error) {
			select {
			case scanDone <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}
	mockK := &mockKiller{
		killFn: func(pid int32, force bool) error {
			return killer.ErrProcessAlreadyDead
		},
	}

	app := NewWithFyneApp(testApp, mockS, mockK)
	proc := model.PortProcess{PID: 7777, Port: 3000, Name: "ephemeral"}

	app.ExecuteKill(proc, false)

	select {
	case <-scanDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expected refresh to be triggered when process is already dead")
	}
}

// TestApp_Polling_StartAndStop validates that the background polling ticker
// correctly schedules and stops background scans. The App is constructed
// directly (without Fyne window creation) and a noopViewer is injected to
// avoid touching Fyne widgets from goroutines, which would race with the
// Fyne test driver's layout callbacks.
func TestApp_Polling_StartAndStop(t *testing.T) {
	var scanCount atomic.Int32
	scanDone := make(chan struct{}, 10)

	mockS := &mockScanner{
		scanFn: func(ctx context.Context) ([]model.PortProcess, error) {
			scanCount.Add(1)
			return nil, nil
		},
	}
	mockK := &mockKiller{}

	// Construct App directly to avoid Fyne window setup touching widgets.
	app := &App{
		scanner:      mockS,
		killer:       mockK,
		state:        NewState(),
		view:         noopViewer{},
		pollInterval: 50 * time.Millisecond,
	}
	app.onScanComplete = func(_ []model.PortProcess) {
		select {
		case scanDone <- struct{}{}:
		default:
		}
	}

	app.StartPolling()
	// Second call must be idempotent.
	app.StartPolling()

	// Wait for the first scan completion signal.
	select {
	case <-scanDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expected background polling to trigger at least one scan within 2s")
	}

	// Drain all goroutines BEFORE any further assertions.
	app.StopPolling()
	app.StopPolling() // must be idempotent

	if scanCount.Load() == 0 {
		t.Fatal("expected at least one background scan to have executed")
	}

	captured := scanCount.Load()
	time.Sleep(100 * time.Millisecond)
	if scanCount.Load() > captured+1 {
		t.Fatal("ticker must not continue scanning after StopPolling")
	}
}
