package main

import (
	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"

	"github.com/jesusuzcategui/soul-app/internal/killer"
	"github.com/jesusuzcategui/soul-app/internal/scanner"
	"github.com/jesusuzcategui/soul-app/internal/ui"
)

func main() {
	newApp().Run()
}

// newApp builds the application with the production Fyne driver.
func newApp() *ui.App {
	return newAppWithFyne(fyneapp.NewWithID("com.jesusuzcategui.soul"))
}

// newAppWithFyne wires the application against an injected Fyne app instance.
// Tests use this seam with the Fyne test driver so no real window (and thus no
// display server) is required.
func newAppWithFyne(fa fyne.App) *ui.App {
	return ui.NewWithFyneApp(fa, scanner.New(), killer.New())
}
