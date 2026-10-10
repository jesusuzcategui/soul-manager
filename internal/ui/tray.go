package ui

import (
	"sync"

	"fyne.io/fyne/v2"
)

// lifecycleWindow is the small window surface needed for tray interactions.
// Keeping it narrow makes lifecycle behavior testable without a desktop driver.
type lifecycleWindow interface {
	Show()
	Hide()
	SetCloseIntercept(func())
}

type focusableWindow interface {
	RequestFocus()
}

// TrayController owns tray menu actions and the hidden/shown window state.
type TrayController struct {
	Menu *fyne.Menu

	window  lifecycleWindow
	refresh func()
	quit    func()

	mu     sync.Mutex
	hidden bool
}

// NewTrayController creates the standard Soul App tray menu.
func NewTrayController(window lifecycleWindow, refresh, quit func()) *TrayController {
	t := &TrayController{
		window:  window,
		refresh: refresh,
		quit:    quit,
		// Run shows the primary window after tray setup, so it starts visible.
		hidden: false,
	}

	showHide := fyne.NewMenuItem("Show / Hide", t.ToggleWindow)
	refreshNow := fyne.NewMenuItem("Refresh Now", t.RefreshNow)
	quitItem := fyne.NewMenuItem("Quit", t.Quit)
	quitItem.IsQuit = true
	t.Menu = fyne.NewMenu("Soul App", showHide, refreshNow, quitItem)

	return t
}

// InstallCloseIntercept keeps the application available through the tray when
// the user closes its window.
func (t *TrayController) InstallCloseIntercept() {
	t.window.SetCloseIntercept(t.HideWindow)
}

// ToggleWindow shows a hidden window or hides a visible one.
func (t *TrayController) ToggleWindow() {
	t.mu.Lock()
	hidden := t.hidden
	t.mu.Unlock()

	if hidden {
		t.ShowWindow()
		return
	}
	t.HideWindow()
}

// ShowWindow restores the window and requests focus when the driver supports it.
func (t *TrayController) ShowWindow() {
	t.mu.Lock()
	t.hidden = false
	t.mu.Unlock()

	t.window.Show()
	if window, ok := t.window.(focusableWindow); ok {
		window.RequestFocus()
	}
}

// HideWindow hides the window without terminating the application.
func (t *TrayController) HideWindow() {
	t.mu.Lock()
	t.hidden = true
	t.mu.Unlock()
	t.window.Hide()
}

// RefreshNow invokes the application's asynchronous refresh callback.
func (t *TrayController) RefreshNow() {
	if t.refresh != nil {
		t.refresh()
	}
}

// Quit invokes the application's terminal cleanup callback.
func (t *TrayController) Quit() {
	if t.quit != nil {
		t.quit()
	}
}
