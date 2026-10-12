package main

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/jesusuzcategui/soul-app/internal/ui"
)

// TestNewApp_WiresMainWindow verifies the production wiring using the Fyne test
// driver. The production driver (GLFW) requires a display server, which is not
// available on headless CI runners, so the test app is injected instead.
func TestNewApp_WiresMainWindow(t *testing.T) {
	app := newAppWithFyne(test.NewApp())
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
