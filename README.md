# Soul App

A minimalist, cross-platform port and process manager built in pure Go with [Fyne](https://fyne.io/).

---

## The Problem
When running local development environments (Node.js, Python, Go, Docker, Podman, and background servers spawned by AI assistants), developers frequently encounter `EADDRINUSE` port collision errors. Terminating these orphaned processes typically requires manual CLI queries (`lsof -i :<port>`, `netstat`, `kill -9 <pid>`).

**Soul App** provides a lightweight, real-time GUI panel and system tray utility to inspect listening TCP ports, identify the associated processes or containers, and terminate them with a single click.

---

## Features
- **TCP Port Monitoring**: Lists active TCP sockets in `LISTEN` mode (`0.0.0.0`, `127.0.0.1`, `::1`).
- **Process & Container Identification**: Resolves PID, executable name, command-line arguments, and flags Docker/Podman container shims (e.g. `docker-proxy`, `conmon`).
- **Real-Time Search & Filtering**: Instant fuzzy filtering by port number, executable name, or PID.
- **One-Click Process Termination**: Sends `SIGTERM` (graceful) and `SIGKILL` (forced) with user feedback for permission issues.
- **System Tray Integration**: Stays docked in the taskbar/tray menu for instant access and minimal desktop clutter.
- **Pure Go & Native**: Zero WebViews, zero Electron, minimal memory footprint (< 50MB idle).

---

## Tech Stack
- **Language**: Go 1.20+
- **GUI Toolkit**: [Fyne v2](https://github.com/fyne-io/fyne)
- **System Metrics & Process Inspection**: [`gopsutil/v3`](https://github.com/shirou/gopsutil)

---

## Prerequisites (Linux)
To compile Fyne on Linux, CGO and graphics development headers are required:

### Ubuntu / Debian:
```bash
sudo apt-get install gcc libgl1-mesa-dev xorg-dev
```

### Fedora:
```bash
sudo dnf install gcc mesa-libGL-devel libX11-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel libXxf86vm-devel
```

### Arch Linux:
```bash
sudo pacman -S base-devel mesa libxcursor libxrandr libxinerama libxi
```

---

## Development
```bash
# Clone and enter directory
cd ~/Workspace/jesusuzcategui/soul-app

# Download dependencies
go mod tidy

# Run application
go run ./cmd/soul
```

---

## Running on macOS (Gatekeeper & Quarantine)
When downloading unsigned or self-compiled binaries/bundles from GitHub Releases on macOS, Apple's Gatekeeper flags them with the `com.apple.quarantine` extended attribute, displaying warnings such as *"Apple cannot check it for malicious software"* or *"Soul is damaged and cannot be opened"*.

To allow running the app without an Apple Developer ID signature:

### Method 1: Remove Quarantine Flag (Recommended)
Open Terminal and run:
```bash
# If running as a standalone binary:
xattr -d com.apple.quarantine /path/to/soul

# If running as an app bundle (.app):
xattr -dr com.apple.quarantine /Applications/Soul.app
```

### Method 2: System Settings
1. Attempt to open the app once (triggering the warning dialog), then close it.
2. Go to **System Settings** > **Privacy & Security**.
3. Scroll down to the **Security** section where it mentions *"Soul was blocked from use because it is not from an identified developer"*.
4. Click **Open Anyway** and confirm.

---

## CI/CD Releases (GitHub Actions)
Soul App uses a multi-platform GitHub Actions workflow (`.github/workflows/release.yml`) to automatically compile and publish binaries for:
- **Linux**: x86_64 tarball (`soul-linux-amd64.tar.gz`)
- **macOS**: Universal / Apple Silicon + Intel (`soul-darwin-universal.zip` or `.app`)
- **Windows**: x86_64 executable (`soul-windows-amd64.zip`)

---

## Documentation
- [SPEC.md](SPEC.md): Full technical specification and architectural breakdown.
- [PLAN.md](PLAN.md): Sub-agent driven development plan and phase breakdown.

