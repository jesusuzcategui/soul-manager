# Soul App - Specification

## 1. Overview
Soul App is a lightweight, cross-platform desktop application built in pure Go using the Fyne toolkit. It allows developers to monitor active network ports (TCP), inspect associated processes (local servers, Docker, Podman), and terminate them quickly via an intuitive graphical interface and system tray integration.

---

## 2. Problem Statement
During local software development and container management (Docker, Podman, background dev servers, AI-spawned processes), developers frequently encounter port collisions (`EADDRINUSE`) or orphaned processes running in the background. Terminating them currently requires terminal commands (`lsof -i :<port>`, `kill -9 <pid>`, `docker stop`), which disrupts flow and lacks immediate visual feedback.

---

## 3. Scope & Target Platforms
- **Primary / Development Platform**: Linux (X11 / Wayland via OpenGL).
- **Cross-Platform Compatibility**: macOS, Windows.
- **Audience**: Personal developer utility.

---

## 4. Architecture & Technology Stack
- **Language**: Go (v1.20+).
- **GUI Toolkit**: Fyne v2 (`fyne.io/fyne/v2`).
- **Process & Socket Inspection**: `github.com/shirou/gopsutil/v3/net` and `github.com/shirou/gopsutil/v3/process` (with fallback to OS utilities/signals).
- **Architecture Pattern**: Container / Presenter pattern:
  - `internal/scanner`: queries TCP listening sockets, resolves PID, executable name, command line, user, and container hints.
  - `internal/killer`: handles process termination (`SIGTERM` followed optionally by `SIGKILL`).
  - `internal/ui`: Fyne window, search bar, process list/table, and tray integration.

---

## 5. Functional Requirements

### 5.1 Port & Process Discovery
- Scan TCP sockets in `LISTEN` state across `0.0.0.0`, `127.0.0.1`, `::1`, and specific interfaces.
- For each listening port, extract:
  - **Port Number** (e.g., `3000`, `8080`, `5432`).
  - **Protocol** (TCP / TCP6).
  - **PID** (Process ID).
  - **Process Name / Executable** (e.g., `node`, `python`, `docker-proxy`, `conmon`).
  - **Container Flag / Type** (detect if process belongs to Docker/Podman where possible).
- Automatic polling interval (configurable, e.g., every 3-5s) with a manual "Refresh" button.

### 5.2 Search & Filtering
- Search input at the top of the interface:
  - Filter in real-time by port number, process name, or PID.

### 5.3 Process Termination
- Action button per row: **Kill**.
- Termination flow:
  1. Send graceful termination (`SIGTERM`).
  2. If process does not terminate within a timeout or if requested, send forced termination (`SIGKILL`).
- Visual feedback on kill status (success toast/notification or error message if permission denied).

### 5.4 System Tray Integration
- Keep app running in the system tray when closed or minimized.
- Tray context menu:
  - "Show / Hide Soul App".
  - Quick refresh.
  - "Quit".

---

## 6. Non-Functional Requirements
- **Resource Footprint**: < 50MB RAM while idle; negligible CPU consumption during background polling.
- **Zero Web Stack**: No Chromium, WebViews, or Node.js runtime dependencies.
- **Single Executable**: Self-contained binary output.

---

## 7. Edge Cases & Considerations
1. **Root-owned Processes**: Some processes (e.g. system services or certain Docker proxies) run as root and cannot be terminated by an unprivileged user without `sudo` / `pkexec`. The app will surface clear permission errors.
2. **Short-lived / Ephemeral Sockets**: If a process terminates between scan and kill action, handle the missing PID cleanly without crashing.
3. **Container Context**: Docker/Podman processes often show up as `docker-proxy`, `containerd-shim`, or `conmon`. We should label these clearly to inform the user that it's a container port forwarder.
