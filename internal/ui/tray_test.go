package ui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jesusuzcategui/soul-app/internal/model"
)

type fakeLifecycleWindow struct {
	showCalls int
	hideCalls int
	intercept func()
}

func (w *fakeLifecycleWindow) Show()                       { w.showCalls++ }
func (w *fakeLifecycleWindow) Hide()                       { w.hideCalls++ }
func (w *fakeLifecycleWindow) SetCloseIntercept(fn func()) { w.intercept = fn }

func TestTrayController_MenuActionsAndCloseIntercept(t *testing.T) {
	window := &fakeLifecycleWindow{}
	var refreshCalls atomic.Int32
	var quitCalls atomic.Int32

	tray := NewTrayController(window, func() { refreshCalls.Add(1) }, func() { quitCalls.Add(1) })
	tray.InstallCloseIntercept()

	if tray.Menu == nil {
		t.Fatal("expected a tray menu")
	}
	if len(tray.Menu.Items) != 3 {
		t.Fatalf("expected three tray menu items, got %d", len(tray.Menu.Items))
	}

	items := tray.Menu.Items
	if items[0].Label != "Show / Hide" || items[1].Label != "Refresh Now" || items[2].Label != "Quit" {
		t.Fatalf("unexpected menu labels: %q, %q, %q", items[0].Label, items[1].Label, items[2].Label)
	}

	items[0].Action()
	if window.hideCalls != 1 {
		t.Fatalf("expected visible window to hide on first toggle, got %d calls", window.hideCalls)
	}

	items[0].Action()
	if window.showCalls != 1 {
		t.Fatalf("expected second toggle to show the window, got %d calls", window.showCalls)
	}

	items[1].Action()
	if refreshCalls.Load() != 1 {
		t.Fatalf("expected refresh action once, got %d", refreshCalls.Load())
	}

	if window.intercept == nil {
		t.Fatal("expected close interceptor to be configured")
	}
	window.intercept()
	if window.hideCalls != 2 {
		t.Fatalf("expected close interception to hide the window, got %d calls", window.hideCalls)
	}

	items[2].Action()
	if quitCalls.Load() != 1 {
		t.Fatalf("expected quit action once, got %d", quitCalls.Load())
	}
}

func TestApp_QuitStopsPollingBeforeExiting(t *testing.T) {
	var quitCalls atomic.Int32
	var scanCalls atomic.Int32
	app := &App{
		scanner: &mockScanner{scanFn: func(context.Context) ([]model.PortProcess, error) {
			scanCalls.Add(1)
			return nil, nil
		}},
		killer:       &mockKiller{},
		state:        NewState(),
		view:         noopViewer{},
		pollInterval: time.Hour,
		onQuit:       func() { quitCalls.Add(1) },
	}

	app.StartPolling()
	app.Quit()

	if app.pollTicker != nil || app.stopPoll != nil {
		t.Fatal("expected Quit to stop polling")
	}
	if quitCalls.Load() != 1 {
		t.Fatalf("expected application quit once, got %d", quitCalls.Load())
	}

	app.StartPolling()
	app.Refresh()
	if scanCalls.Load() != 0 {
		t.Fatalf("expected no work after Quit, got %d scans", scanCalls.Load())
	}
}
