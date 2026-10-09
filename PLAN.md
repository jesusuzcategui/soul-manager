# Soul App - Development Plan (Sub-Agent Driven)

This development plan breaks down the implementation of **Soul App** into independent, decoupled phases following the Spec-Driven Development (SDD) / Sub-Agent execution model. Each phase defines clear deliverables, dependencies, and strict acceptance criteria.

---

## Architecture Overview
```
soul-app/
├── cmd/
│   └── soul/
│       └── main.go           # Application entrypoint
├── internal/
│   ├── model/
│   │   └── process.go        # PortProcess domain structures
│   ├── scanner/
│   │   ├── scanner.go        # Port/Process detection interface & implementation
│   │   └── scanner_test.go   # Unit and integration tests
│   ├── killer/
│   │   ├── killer.go         # Process termination logic & permission handling
│   │   └── killer_test.go    # Termination unit tests
│   └── ui/
│       ├── app.go            # Fyne app setup, window & lifecycle
│       ├── view.go           # Search bar, process table/list, kill interactions
│       └── tray.go           # System tray icon and menu integration
├── SPEC.md                   # Technical specification
├── PLAN.md                   # This sub-agent execution plan
└── README.md                 # Project documentation
```

---

## Phase 1: Environment Bootstrapping & Scaffolding
- **Objective**: Initialize Go module, define core domain models, and set up package structure.
- **Dependencies**: None.
- **Tasks**:
  1. Initialize `go.mod` (module `github.com/jesusuzcategui/soul-app`).
  2. Add external dependencies: `fyne.io/fyne/v2` and `github.com/shirou/gopsutil/v3`.
  3. Define `internal/model/process.go`:
     - Struct `PortProcess` (`Port int`, `Protocol string`, `PID int32`, `Name string`, `Cmdline string`, `IsContainer bool`, `ContainerType string`).
  4. Create skeleton for `cmd/soul/main.go`.
- **Acceptance Criteria**:
  - `go mod tidy` resolves successfully without errors.
  - Domain models compiled cleanly with `go build ./...`.

---

## Phase 2: Core Scanner Engine (`internal/scanner`)
- **Objective**: Implement TCP listening socket inspection and map active ports to system processes.
- **Dependencies**: Phase 1.
- **Tasks**:
  1. Create interface `Scanner` with `Scan() ([]model.PortProcess, error)`.
  2. Implement scan logic using `gopsutil/v3/net.Connections("tcp")` filtering for state `LISTEN`.
  3. Map socket PIDs to executable details via `gopsutil/v3/process.NewProcess(pid)`.
  4. Add heuristic identification for containers:
     - Check process name / cmdline against `docker-proxy`, `containerd-shim`, `conmon`, `podman`.
  5. Deduplicate multiple listening sockets bound to the same PID/Port (e.g. IPv4 + IPv6 bindings).
- **Acceptance Criteria**:
  - `scanner_test.go` starts a mock TCP listener (e.g. on port 49152) and asserts that `Scan()` captures the port, PID, and process name accurately.
  - Graceful handling when socket PID is 0 or inaccessible (system services).

---

## Phase 3: Core Killer Engine (`internal/killer`)
- **Objective**: Implement safe and reliable process termination.
- **Dependencies**: Phase 1.
- **Tasks**:
  1. Create interface `Killer` with `Kill(pid int32, force bool) error`.
  2. Implement graceful termination (`SIGTERM`) and forced termination (`SIGKILL`) via `os.FindProcess` / `syscall`.
  3. Add platform-agnostic permission/error classification (distinguish `ProcessAlreadyDead` vs `PermissionDenied`).
- **Acceptance Criteria**:
  - `killer_test.go` spawns a dummy background process (e.g. `sleep 100`), calls `Kill(pid, false)`, and verifies the process is terminated.
  - Proper error returned when attempting to kill an invalid PID or unprivileged root process.

---

## Phase 4: UI Development (`internal/ui`)
- **Objective**: Build the Fyne GUI panel with search, table list, and action buttons.
- **Dependencies**: Phase 2, Phase 3.
- **Tasks**:
  1. Set up Fyne `App` and main `Window` with appropriate title, size (e.g. 640x480), and icon.
  2. Implement search input (`widget.NewEntry`) with debounce/on-change filtering.
  3. Implement process table / list (`widget.NewTable` or custom container list) displaying:
     - Columns: `Port`, `Process / Container`, `PID`, `Action`.
     - Highlight or tag container processes.
  4. Implement "Kill" action per item:
     - Trigger `killer.Kill`.
     - Show notification or modal error on failure (e.g., "Permission Denied: Run with elevated permissions").
  5. Add manual "Refresh" button and automatic background polling ticker (every 3 seconds).
- **Acceptance Criteria**:
  - App launches without UI freezing during background scans (uses goroutines for scan/kill).
  - Filtering updates the view immediately.
  - UI state updates reactively after killing a process.

---

## Phase 5: System Tray & Window Lifecycle (`internal/ui/tray.go`)
- **Objective**: Support minimizing to taskbar/tray and background execution.
- **Dependencies**: Phase 4.
- **Tasks**:
  1. Implement Fyne desktop driver tray menu (`desktop.App` from `fyne.io/fyne/v2/driver/desktop`).
  2. Add tray menu items: "Show / Hide", "Refresh Now", and "Quit".
  3. Intercept window close event to hide/minimize to tray instead of quitting if tray is available.
- **Acceptance Criteria**:
  - App can be hidden to tray and reopened by clicking the tray menu.
  - Quit option properly exits the process and cleans up background tickers.

---

## Phase 6: E2E Integration & Verification
- **Objective**: Full end-to-end integration and smoke testing on Linux.
- **Dependencies**: Phase 1 through 5.
- **Tasks**:
  1. Wire all components together in `cmd/soul/main.go`.
  2. Build release binary `go build -o bin/soul ./cmd/soul`.
  3. Perform manual/automated validation with real local servers (e.g. Python `http.server`, Node server, or Docker container).
  4. Validate memory footprint (< 50MB RSS).
- **Acceptance Criteria**:
  - Binary builds cleanly and executes on the target desktop environment.
  - Successfully detects and kills test servers.

---

## Phase 7: CI/CD (GitHub Actions) & macOS Packaging Documentation
- **Objective**: Automate multi-platform release builds and document macOS Gatekeeper/quarantine resolution.
- **Dependencies**: Phase 6.
- **Tasks**:
  1. Set up GitHub Actions workflow (`.github/workflows/release.yml`):
     - Matrix build for `ubuntu-latest`, `macos-latest`, and `windows-latest`.
     - Install OS graphics libraries on Linux runners (`libgl1-mesa-dev`, `xorg-dev`).
     - Package binaries/bundles into release assets (Linux `.tar.gz`, macOS `.zip`/`.app`, Windows `.zip`).
  2. Document macOS Gatekeeper quarantine handling:
     - Clear instructions in `README.md` on how to bypass Apple quarantine for unsigned binaries using `xattr -d com.apple.quarantine`.
- **Acceptance Criteria**:
  - GitHub Actions workflow syntax validates cleanly.
  - Clear, tested instructions for running on macOS without Gatekeeper blockage.

