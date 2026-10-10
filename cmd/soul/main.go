package main

import (
	"github.com/jesusuzcategui/soul-app/internal/killer"
	"github.com/jesusuzcategui/soul-app/internal/scanner"
	"github.com/jesusuzcategui/soul-app/internal/ui"
)

func main() {
	newApp().Run()
}

func newApp() *ui.App {
	return ui.New(scanner.New(), killer.New())
}
