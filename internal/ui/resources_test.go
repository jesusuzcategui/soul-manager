package ui

import "testing"

func TestAppIconEmbedded(t *testing.T) {
	icon := AppIcon()
	if icon == nil {
		t.Fatal("expected embedded app icon")
	}
	if icon.Name() != "soul.png" {
		t.Fatalf("icon name = %q, want %q", icon.Name(), "soul.png")
	}
	if len(icon.Content()) == 0 {
		t.Fatal("expected non-empty icon content")
	}
}
