package ui

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/jesusuzcategui/soul-app/internal/model"
)

// TableRowLayout manages proportional and fixed column widths for process table rows and headers.
type TableRowLayout struct {
	portWidth      float32
	protoWidth     float32
	containerWidth float32
	pidWidth       float32
	actionWidth    float32
	padding        float32
}

// NewTableRowLayout creates a responsive row layout for table columns.
func NewTableRowLayout() *TableRowLayout {
	return &TableRowLayout{
		portWidth:      80,
		protoWidth:     65,
		containerWidth: 100,
		pidWidth:       75,
		actionWidth:    85,
		padding:        8,
	}
}

// Layout arranges the 6 child objects: Port, Protocol, Process Name, Container, PID, Action.
func (l *TableRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 6 {
		return
	}
	for i := 0; i < 6; i++ {
		if objects[i] == nil {
			return
		}
	}

	fixedWidth := l.portWidth + l.protoWidth + l.containerWidth + l.pidWidth + l.actionWidth
	totalPadding := 5 * l.padding

	nameWidth := size.Width - (fixedWidth + totalPadding)
	if nameWidth < 120 {
		nameWidth = 120
	}

	x := float32(0)
	h := size.Height

	// 0: Port
	objects[0].Move(fyne.NewPos(x, 0))
	objects[0].Resize(fyne.NewSize(l.portWidth, h))
	x += l.portWidth + l.padding

	// 1: Protocol
	objects[1].Move(fyne.NewPos(x, 0))
	objects[1].Resize(fyne.NewSize(l.protoWidth, h))
	x += l.protoWidth + l.padding

	// 2: Process Name (expands to fill available space)
	objects[2].Move(fyne.NewPos(x, 0))
	objects[2].Resize(fyne.NewSize(nameWidth, h))
	x += nameWidth + l.padding

	// 3: Container badge
	objects[3].Move(fyne.NewPos(x, 0))
	objects[3].Resize(fyne.NewSize(l.containerWidth, h))
	x += l.containerWidth + l.padding

	// 4: PID
	objects[4].Move(fyne.NewPos(x, 0))
	objects[4].Resize(fyne.NewSize(l.pidWidth, h))
	x += l.pidWidth + l.padding

	// 5: Action Button (Kill)
	btnMin := objects[5].MinSize()
	btnY := float32(0)
	btnH := h
	if h > btnMin.Height && btnMin.Height > 0 {
		btnY = (h - btnMin.Height) / 2
		btnH = btnMin.Height
	}
	objects[5].Move(fyne.NewPos(x, btnY))
	objects[5].Resize(fyne.NewSize(l.actionWidth, btnH))
}

// MinSize returns the minimum required size for a table row.
func (l *TableRowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	h := float32(36)
	for _, o := range objects {
		if o != nil {
			if mh := o.MinSize().Height; mh > h {
				h = mh
			}
		}
	}
	minW := l.portWidth + l.protoWidth + 120 + l.containerWidth + l.pidWidth + l.actionWidth + 5*l.padding
	return fyne.NewSize(minW, h)
}

// View encapsulates UI widgets, layouts, and user interactions.
type View struct {
	mu sync.RWMutex

	SearchEntry *widget.Entry
	RefreshBtn  *widget.Button
	ProcessList *widget.List
	HeaderRow   *fyne.Container
	StatusLabel *widget.Label
	ProgressBar *widget.ProgressBarInfinite
	EmptyLabel  *widget.Label

	Root fyne.CanvasObject

	items     []model.PortProcess
	onSearch  func(string)
	onRefresh func()
	onKill    func(model.PortProcess)
}

// NewView creates and lays out the application view components.
func NewView(
	onSearch func(string),
	onRefresh func(),
	onKill func(model.PortProcess),
) *View {
	v := &View{
		items:     make([]model.PortProcess, 0),
		onSearch:  onSearch,
		onRefresh: onRefresh,
		onKill:    onKill,
	}

	v.buildComponents()
	v.buildLayout()
	return v
}

// buildComponents initializes the individual widgets.
func (v *View) buildComponents() {
	// Search Entry
	v.SearchEntry = widget.NewEntry()
	v.SearchEntry.SetPlaceHolder("Search port, process, PID, container (e.g. 8080, node, docker)...")
	v.SearchEntry.OnChanged = func(query string) {
		if v.onSearch != nil {
			v.onSearch(query)
		}
	}

	// Manual Refresh Button
	v.RefreshBtn = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		if v.onRefresh != nil {
			v.onRefresh()
		}
	})

	// Table Header
	portH := widget.NewLabelWithStyle("Port", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	protoH := widget.NewLabelWithStyle("Protocol", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	nameH := widget.NewLabelWithStyle("Process Name", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	containerH := widget.NewLabelWithStyle("Container", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	pidH := widget.NewLabelWithStyle("PID", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	actionH := widget.NewLabelWithStyle("Action", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	v.HeaderRow = container.New(NewTableRowLayout(), portH, protoH, nameH, containerH, pidH, actionH)

	// Process List with custom row template
	v.ProcessList = widget.NewList(
		func() int {
			v.mu.RLock()
			defer v.mu.RUnlock()
			return len(v.items)
		},
		func() fyne.CanvasObject {
			portL := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			protoL := widget.NewLabel("")
			nameL := widget.NewLabel("")
			nameL.Truncation = fyne.TextTruncateEllipsis
			badgeL := widget.NewLabel("")
			pidL := widget.NewLabel("")
			killB := widget.NewButtonWithIcon("Kill", theme.DeleteIcon(), nil)
			killB.Importance = widget.DangerImportance

			return container.New(NewTableRowLayout(), portL, protoL, nameL, badgeL, pidL, killB)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			v.mu.RLock()
			if id < 0 || id >= len(v.items) {
				v.mu.RUnlock()
				return
			}
			p := v.items[id]
			v.mu.RUnlock()

			c, ok := obj.(*fyne.Container)
			if !ok || len(c.Objects) < 6 {
				return
			}

			portLabel := c.Objects[0].(*widget.Label)
			protoLabel := c.Objects[1].(*widget.Label)
			nameLabel := c.Objects[2].(*widget.Label)
			badgeLabel := c.Objects[3].(*widget.Label)
			pidLabel := c.Objects[4].(*widget.Label)
			killBtn := c.Objects[5].(*widget.Button)

			portLabel.SetText(fmt.Sprintf(":%d", p.Port))
			protoLabel.SetText(p.Protocol)
			nameLabel.SetText(ServiceName(p))
			badgeLabel.SetText(FormatContainerBadge(p))

			if p.PID > 0 {
				pidLabel.SetText(strconv.Itoa(int(p.PID)))
				killBtn.Enable()
			} else {
				pidLabel.SetText("-")
				killBtn.Disable()
			}

			// Capture process in closure for action tap
			killBtn.OnTapped = func() {
				if v.onKill != nil {
					v.onKill(p)
				}
			}
		},
	)

	// Empty State Placeholder
	v.EmptyLabel = widget.NewLabelWithStyle("No active listening ports detected.", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	// Status Bar
	v.StatusLabel = widget.NewLabel("Ready")
	v.ProgressBar = widget.NewProgressBarInfinite()
	v.ProgressBar.Hide()
	v.ProgressBar.Stop()
}

// buildLayout arranges the top bar, header, list, and status bar into the Root canvas object.
func (v *View) buildLayout() {
	// Top Search & Refresh Bar
	topBar := container.NewBorder(nil, nil, nil, v.RefreshBtn, v.SearchEntry)

	// Header with separator
	headerSection := container.NewVBox(
		topBar,
		v.HeaderRow,
		widget.NewSeparator(),
	)

	// Center table content with empty placeholder
	centerSection := container.NewStack(
		v.EmptyLabel,
		v.ProcessList,
	)

	// Bottom Status Bar
	statusSection := container.NewBorder(
		widget.NewSeparator(),
		nil,
		nil,
		container.NewHBox(v.ProgressBar),
		v.StatusLabel,
	)

	// Complete View Root
	v.Root = container.NewBorder(
		headerSection,
		statusSection,
		nil,
		nil,
		centerSection,
	)
}

// UpdateList updates the items rendered in the process table.
func (v *View) UpdateList(items []model.PortProcess) {
	v.mu.Lock()
	copied := make([]model.PortProcess, len(items))
	copy(copied, items)
	v.items = copied
	itemCount := len(v.items)
	v.mu.Unlock()

	if itemCount == 0 {
		v.EmptyLabel.Show()
	} else {
		v.EmptyLabel.Hide()
	}
	v.ProcessList.Refresh()
}

// SetStatus updates the status label text.
func (v *View) SetStatus(status string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.StatusLabel.SetText(status)
}

// SetScanning updates the loading indicator state.
func (v *View) SetScanning(scanning bool) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if scanning {
		v.ProgressBar.Show()
		v.ProgressBar.Start()
		v.RefreshBtn.Disable()
	} else {
		v.ProgressBar.Stop()
		v.ProgressBar.Hide()
		v.RefreshBtn.Enable()
	}
}

// SetEmptyMessage customizes the message displayed when no items are present.
func (v *View) SetEmptyMessage(msg string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.EmptyLabel.SetText(msg)
}

// ServiceName returns the label shown in the process name column. For container
// workloads it prefers the resolved real container name; otherwise it falls back
// to the executable name.
func ServiceName(p model.PortProcess) string {
	if name := strings.TrimSpace(p.ContainerName); name != "" {
		return name
	}
	return p.Name
}

// FormatContainerBadge converts process container metadata into a display badge.
func FormatContainerBadge(p model.PortProcess) string {
	if !p.IsContainer && strings.TrimSpace(p.ContainerType) == "" {
		return "-"
	}

	t := strings.TrimSpace(p.ContainerType)
	if t == "" {
		return "[Container]"
	}

	if strings.EqualFold(t, "docker") {
		return "[Docker]"
	}
	if strings.EqualFold(t, "podman") {
		return "[Podman]"
	}
	if strings.EqualFold(t, "containerd") {
		return "[Containerd]"
	}

	return fmt.Sprintf("[%s%s]", strings.ToUpper(t[:1]), strings.ToLower(t[1:]))
}
