package ui

import (
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/jesusuzcategui/soul-app/internal/model"
)

func TestFormatContainerBadge(t *testing.T) {
	tests := []struct {
		name     string
		proc     model.PortProcess
		expected string
	}{
		{
			name: "Non-container process",
			proc: model.PortProcess{
				IsContainer:   false,
				ContainerType: "",
			},
			expected: "-",
		},
		{
			name: "Docker container process",
			proc: model.PortProcess{
				IsContainer:   true,
				ContainerType: "docker",
			},
			expected: "[Docker]",
		},
		{
			name: "Podman container process",
			proc: model.PortProcess{
				IsContainer:   true,
				ContainerType: "podman",
			},
			expected: "[Podman]",
		},
		{
			name: "Containerd process",
			proc: model.PortProcess{
				IsContainer:   true,
				ContainerType: "containerd",
			},
			expected: "[Containerd]",
		},
		{
			name: "Generic container without runtime type",
			proc: model.PortProcess{
				IsContainer:   true,
				ContainerType: "",
			},
			expected: "[Container]",
		},
		{
			name: "Custom runtime",
			proc: model.PortProcess{
				IsContainer:   true,
				ContainerType: "crio",
			},
			expected: "[Crio]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatContainerBadge(tt.proc)
			if got != tt.expected {
				t.Errorf("FormatContainerBadge() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestServiceName(t *testing.T) {
	tests := []struct {
		name     string
		proc     model.PortProcess
		expected string
	}{
		{
			name:     "resolved container name wins",
			proc:     model.PortProcess{Name: "conmon", ContainerName: "my-postgres"},
			expected: "my-postgres",
		},
		{
			name:     "falls back to process name",
			proc:     model.PortProcess{Name: "node"},
			expected: "node",
		},
		{
			name:     "blank container name falls back",
			proc:     model.PortProcess{Name: "node", ContainerName: "   "},
			expected: "node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ServiceName(tt.proc); got != tt.expected {
				t.Errorf("ServiceName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTableRowLayout(t *testing.T) {
	layout := NewTableRowLayout()
	objects := make([]fyne.CanvasObject, 6)
	for i := range objects {
		objects[i] = widget.NewLabel("col")
	}

	minSize := layout.MinSize(objects)
	if minSize.Width <= 0 || minSize.Height <= 0 {
		t.Fatalf("expected positive minSize, got %+v", minSize)
	}

	size := fyne.NewSize(700, 40)
	layout.Layout(objects, size)

	// Verify columns are laid out from left to right
	prevX := float32(-1)
	for i, obj := range objects {
		pos := obj.Position()
		if pos.X <= prevX {
			t.Errorf("object %d position X %f is not greater than previous X %f", i, pos.X, prevX)
		}
		prevX = pos.X
	}
}

func TestView_ComponentsAndInteractions(t *testing.T) {
	testApp := test.NewApp()
	_ = testApp

	var searchedText string
	var refreshClicked atomic.Bool
	var killedProc model.PortProcess

	view := NewView(
		func(q string) {
			searchedText = q
		},
		func() {
			refreshClicked.Store(true)
		},
		func(p model.PortProcess) {
			killedProc = p
		},
	)

	if view.Root == nil {
		t.Fatal("expected view.Root not to be nil")
	}

	// Test Search Entry input triggers onSearch callback
	test.Type(view.SearchEntry, "8080")
	if searchedText != "8080" {
		t.Fatalf("expected search query '8080', got %q", searchedText)
	}

	// Test Refresh Button triggers onRefresh callback
	test.Tap(view.RefreshBtn)
	if !refreshClicked.Load() {
		t.Fatal("expected onRefresh to be called when RefreshBtn is tapped")
	}

	// Test UpdateList toggles empty label visibility
	view.UpdateList(nil)
	if !view.EmptyLabel.Visible() {
		t.Fatal("expected EmptyLabel to be visible when list is empty")
	}

	procs := sampleProcesses()
	view.UpdateList(procs)
	if view.EmptyLabel.Visible() {
		t.Fatal("expected EmptyLabel to be hidden when list has items")
	}

	// Test SetStatus
	view.SetStatus("Testing status")
	if view.StatusLabel.Text != "Testing status" {
		t.Fatalf("expected status label 'Testing status', got %q", view.StatusLabel.Text)
	}

	// Test SetScanning
	view.SetScanning(true)
	if !view.ProgressBar.Visible() {
		t.Fatal("expected progress bar visible when scanning")
	}
	if !view.RefreshBtn.Disabled() {
		t.Fatal("expected refresh button disabled when scanning")
	}

	view.SetScanning(false)
	if view.ProgressBar.Visible() {
		t.Fatal("expected progress bar hidden when not scanning")
	}
	if view.RefreshBtn.Disabled() {
		t.Fatal("expected refresh button enabled when not scanning")
	}

	// Test onKill callback wiring
	testProc := procs[1]
	view.onKill(testProc)
	if killedProc.PID != testProc.PID {
		t.Fatalf("expected killedProc PID %d, got %d", testProc.PID, killedProc.PID)
	}
}
