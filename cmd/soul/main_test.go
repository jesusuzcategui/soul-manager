package main

import (
	"testing"

	"github.com/jesusuzcategui/soul-app/internal/ui"
)

func TestNewApp_WiresMainWindow(t *testing.T) {
	app := newApp()
	t.Cleanup(func() {
		app.StopPolling()
		app.FyneApp().Quit()
	})

	if app.Window() == nil {
		t.Fatal("expected application window")
	}
	if got := app.Window().Title(); got != ui.DefaultWindowTitle {
		t.Fatalf("window title = %q, want %q", got, ui.DefaultWindowTitle)
	}
	if app.State() == nil || app.View() == nil {
		t.Fatal("expected UI state and view to be initialized")
	}
}
