package scanner

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	gopsnet "github.com/shirou/gopsutil/v3/net"

	"github.com/jesusuzcategui/soul-app/internal/model"
)

func TestScan_DetectLiveListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock TCP listener: %v", err)
	}
	defer listener.Close()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected *net.TCPAddr, got %T", listener.Addr())
	}
	targetPort := tcpAddr.Port

	scanner := New()
	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan returned unexpected error: %v", err)
	}

	currentPID := int32(os.Getpid())
	var matched *model.PortProcess

	for i := range results {
		if results[i].Port == targetPort {
			matched = &results[i]
			break
		}
	}

	if matched == nil {
		t.Fatalf("Scan failed to detect listening port %d in results (total detected: %d)", targetPort, len(results))
	}

	if matched.PID != currentPID {
		t.Errorf("expected PID %d, got %d", currentPID, matched.PID)
	}

	if matched.Protocol != "TCP" {
		t.Errorf("expected Protocol 'TCP', got %q", matched.Protocol)
	}

	if matched.Name == "" || matched.Name == FallbackProcessName {
		t.Errorf("expected resolved process name for current PID, got %q", matched.Name)
	}

	// Verify package-level Scan() convenience function as well
	pkgResults, err := Scan()
	if err != nil {
		t.Fatalf("package Scan() failed: %v", err)
	}

	var pkgMatched bool
	for _, p := range pkgResults {
		if p.Port == targetPort && p.PID == currentPID {
			pkgMatched = true
			break
		}
	}
	if !pkgMatched {
		t.Errorf("package Scan() failed to find target port %d and PID %d", targetPort, currentPID)
	}
}

func TestIdentifyContainer(t *testing.T) {
	tests := []struct {
		name              string
		procName          string
		cmdline           string
		wantIsContainer   bool
		wantContainerType string
	}{
		{
			name:              "docker-proxy",
			procName:          "docker-proxy",
			cmdline:           "/usr/bin/docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 8080",
			wantIsContainer:   true,
			wantContainerType: "Docker",
		},
		{
			name:              "dockerd",
			procName:          "dockerd",
			cmdline:           "/usr/bin/dockerd",
			wantIsContainer:   true,
			wantContainerType: "Docker",
		},
		{
			name:              "podman",
			procName:          "podman",
			cmdline:           "podman run -p 3000:3000 node",
			wantIsContainer:   true,
			wantContainerType: "Podman",
		},
		{
			name:              "conmon",
			procName:          "conmon",
			cmdline:           "/usr/libexec/podman/conmon --api-version 1 -c abcd",
			wantIsContainer:   true,
			wantContainerType: "Podman",
		},
		{
			name:              "containerd-shim",
			procName:          "containerd-shim-runc-v2",
			cmdline:           "/usr/bin/containerd-shim-runc-v2 -namespace moby",
			wantIsContainer:   true,
			wantContainerType: "Containerd",
		},
		{
			name:              "containerd",
			procName:          "containerd",
			cmdline:           "/usr/bin/containerd",
			wantIsContainer:   true,
			wantContainerType: "Containerd",
		},
		{
			name:              "nerdctl",
			procName:          "nerdctl",
			cmdline:           "nerdctl run -p 8080:80 nginx",
			wantIsContainer:   true,
			wantContainerType: "Containerd",
		},
		{
			name:              "crun binary",
			procName:          "crun",
			cmdline:           "/usr/bin/crun run my-container",
			wantIsContainer:   true,
			wantContainerType: "crun",
		},
		{
			name:              "runc binary",
			procName:          "runc",
			cmdline:           "/usr/bin/runc --root /run/runc run test",
			wantIsContainer:   true,
			wantContainerType: "runc",
		},
		{
			name:              "truncate false positive trap",
			procName:          "truncate",
			cmdline:           "truncate -s 100M largefile.bin",
			wantIsContainer:   false,
			wantContainerType: "",
		},
		{
			name:              "crunch false positive trap",
			procName:          "crunch",
			cmdline:           "crunch 4 4 -o wordlist.txt",
			wantIsContainer:   false,
			wantContainerType: "",
		},
		{
			name:              "regular node process",
			procName:          "node",
			cmdline:           "node /app/index.js",
			wantIsContainer:   false,
			wantContainerType: "",
		},
		{
			name:              "regular python process",
			procName:          "python3",
			cmdline:           "python3 -m http.server 8000",
			wantIsContainer:   false,
			wantContainerType: "",
		},
		{
			name:              "empty process",
			procName:          "",
			cmdline:           "",
			wantIsContainer:   false,
			wantContainerType: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isContainer, containerType := IdentifyContainer(tc.procName, tc.cmdline)
			if isContainer != tc.wantIsContainer {
				t.Errorf("IdentifyContainer(%q, %q) isContainer = %v, want %v",
					tc.procName, tc.cmdline, isContainer, tc.wantIsContainer)
			}
			if containerType != tc.wantContainerType {
				t.Errorf("IdentifyContainer(%q, %q) containerType = %q, want %q",
					tc.procName, tc.cmdline, containerType, tc.wantContainerType)
			}
		})
	}
}

func TestScan_Deduplication(t *testing.T) {
	mockConns := []gopsnet.ConnectionStat{
		{
			Status: "LISTEN",
			Laddr:  gopsnet.Addr{IP: "0.0.0.0", Port: 8080},
			Pid:    101,
		},
		{
			Status: "LISTEN",
			Laddr:  gopsnet.Addr{IP: "::", Port: 8080},
			Pid:    101,
		},
		{
			Status: "LISTEN",
			Laddr:  gopsnet.Addr{IP: "127.0.0.1", Port: 8080},
			Pid:    101,
		},
		{
			Status: "LISTEN",
			Laddr:  gopsnet.Addr{IP: "0.0.0.0", Port: 9000},
			Pid:    102,
		},
	}

	scanner := &DefaultScanner{
		connProvider: func(ctx context.Context, kind string) ([]gopsnet.ConnectionStat, error) {
			return mockConns, nil
		},
		procProvider: func(ctx context.Context, pid int32) (*ProcessDetails, error) {
			return &ProcessDetails{
				Name:     "web-server",
				Cmdline:  "web-server --port 8080",
				Username: "appuser",
			}, nil
		},
	}

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 deduplicated results, got %d", len(results))
	}

	if results[0].Port != 8080 || results[0].PID != 101 {
		t.Errorf("unexpected first entry: %+v", results[0])
	}
	if results[0].Protocol != "TCP" {
		t.Errorf("expected Protocol 'TCP' after deduplicating dual-stack bindings, got %q", results[0].Protocol)
	}

	if results[1].Port != 9000 || results[1].PID != 102 {
		t.Errorf("unexpected second entry: %+v", results[1])
	}
}

func TestScan_UnprivilegedAndZeroPID(t *testing.T) {
	mockConns := []gopsnet.ConnectionStat{
		{
			Status: "LISTEN",
			Laddr:  gopsnet.Addr{IP: "0.0.0.0", Port: 80},
			Pid:    0,
		},
		{
			Status: "LISTEN",
			Laddr:  gopsnet.Addr{IP: "0.0.0.0", Port: 443},
			Pid:    -1,
		},
		{
			Status: "LISTEN",
			Laddr:  gopsnet.Addr{IP: "0.0.0.0", Port: 53},
			Pid:    9999, // Fails process resolution
		},
	}

	scanner := &DefaultScanner{
		connProvider: func(ctx context.Context, kind string) ([]gopsnet.ConnectionStat, error) {
			return mockConns, nil
		},
		procProvider: func(ctx context.Context, pid int32) (*ProcessDetails, error) {
			return nil, os.ErrPermission
		},
	}

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(results))
	}

	for _, p := range results {
		if p.Name != FallbackProcessName {
			t.Errorf("port %d expected name %q, got %q", p.Port, FallbackProcessName, p.Name)
		}
		if p.IsContainer {
			t.Errorf("port %d unprivileged entry should not be container", p.Port)
		}
	}
}

func TestScan_Sorting(t *testing.T) {
	mockConns := []gopsnet.ConnectionStat{
		{Status: "LISTEN", Laddr: gopsnet.Addr{IP: "0.0.0.0", Port: 9000}, Pid: 50},
		{Status: "LISTEN", Laddr: gopsnet.Addr{IP: "0.0.0.0", Port: 80}, Pid: 10},
		{Status: "LISTEN", Laddr: gopsnet.Addr{IP: "0.0.0.0", Port: 3000}, Pid: 20},
		{Status: "LISTEN", Laddr: gopsnet.Addr{IP: "0.0.0.0", Port: 8080}, Pid: 30},
	}

	scanner := &DefaultScanner{
		connProvider: func(ctx context.Context, kind string) ([]gopsnet.ConnectionStat, error) {
			return mockConns, nil
		},
		procProvider: func(ctx context.Context, pid int32) (*ProcessDetails, error) {
			return &ProcessDetails{Name: "svc"}, nil
		},
	}

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPorts := []int{80, 3000, 8080, 9000}
	if len(results) != len(expectedPorts) {
		t.Fatalf("expected %d results, got %d", len(expectedPorts), len(results))
	}

	for i, exp := range expectedPorts {
		if results[i].Port != exp {
			t.Errorf("at index %d: expected port %d, got %d", i, exp, results[i].Port)
		}
	}
}

func TestScan_FiltersNonListeningSockets(t *testing.T) {
	mockConns := []gopsnet.ConnectionStat{
		{Status: "ESTABLISHED", Laddr: gopsnet.Addr{IP: "127.0.0.1", Port: 54321}, Pid: 10},
		{Status: "TIME_WAIT", Laddr: gopsnet.Addr{IP: "127.0.0.1", Port: 54322}, Pid: 10},
		{Status: "CLOSE_WAIT", Laddr: gopsnet.Addr{IP: "127.0.0.1", Port: 54323}, Pid: 10},
		{Status: "LISTEN", Laddr: gopsnet.Addr{IP: "127.0.0.1", Port: 3000}, Pid: 10},
	}

	scanner := &DefaultScanner{
		connProvider: func(ctx context.Context, kind string) ([]gopsnet.ConnectionStat, error) {
			return mockConns, nil
		},
		procProvider: func(ctx context.Context, pid int32) (*ProcessDetails, error) {
			return &ProcessDetails{Name: "app"}, nil
		},
	}

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Port != 3000 {
		t.Errorf("expected port 3000, got %d", results[0].Port)
	}
}

func TestScan_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scanner := New()
	_, err := scanner.Scan(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestScan_NilContext(t *testing.T) {
	scanner := &DefaultScanner{
		connProvider: func(ctx context.Context, kind string) ([]gopsnet.ConnectionStat, error) {
			return nil, nil
		},
		procProvider: func(ctx context.Context, pid int32) (*ProcessDetails, error) {
			return nil, nil
		},
	}

	results, err := scanner.Scan(nil)
	if err != nil {
		t.Fatalf("Scan(nil) failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty results, got %d", len(results))
	}
}

func TestDefaultProcessProvider_CurrentProcess(t *testing.T) {
	pid := int32(os.Getpid())
	details, err := defaultProcessProvider(context.Background(), pid)
	if err != nil {
		t.Fatalf("defaultProcessProvider failed for current process: %v", err)
	}

	if details.Name == "" {
		t.Errorf("expected non-empty process name")
	}

	// Executable name should match current test binary
	baseName := filepath.Base(os.Args[0])
	if details.Name != baseName && !filepath.IsAbs(details.Name) && filepath.Base(details.Name) != baseName {
		t.Logf("Process name %q, os.Args[0] base %q", details.Name, baseName)
	}
}
