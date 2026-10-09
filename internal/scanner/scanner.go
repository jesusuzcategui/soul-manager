package scanner

import (
	"context"
	"net"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	gopsnet "github.com/shirou/gopsutil/v3/net"
	gopsproc "github.com/shirou/gopsutil/v3/process"

	"github.com/jesusuzcategui/soul-app/internal/model"
)

// FallbackProcessName is assigned when a process name cannot be inspected due to permissions or OS restrictions.
const FallbackProcessName = "[System / Unprivileged]"

var (
	reRunc = regexp.MustCompile(`\brunc\b`)
	reCrun = regexp.MustCompile(`\bcrun\b`)
)

// Scanner defines the capability to discover network listening ports and their associated processes.
type Scanner interface {
	Scan(ctx context.Context) ([]model.PortProcess, error)
}

// ProcessDetails encapsulates metadata resolved for an active process.
type ProcessDetails struct {
	Name     string
	Cmdline  string
	Username string
}

// ConnectionsProvider defines the function signature for fetching network socket statistics.
type ConnectionsProvider func(ctx context.Context, kind string) ([]gopsnet.ConnectionStat, error)

// ProcessProvider defines the function signature for resolving process metadata for a PID.
type ProcessProvider func(ctx context.Context, pid int32) (*ProcessDetails, error)

// DefaultScanner implements Scanner by inspecting network sockets and resolving system processes.
type DefaultScanner struct {
	connProvider ConnectionsProvider
	procProvider ProcessProvider
}

// New creates and returns a new Scanner using default system providers.
func New() Scanner {
	return NewDefaultScanner()
}

// NewDefaultScanner returns a concrete *DefaultScanner configured with system providers.
func NewDefaultScanner() *DefaultScanner {
	return &DefaultScanner{
		connProvider: gopsnet.ConnectionsWithContext,
		procProvider: defaultProcessProvider,
	}
}

// defaultProcessProvider queries gopsutil for process name, cmdline, and username.
func defaultProcessProvider(ctx context.Context, pid int32) (*ProcessDetails, error) {
	p, err := gopsproc.NewProcessWithContext(ctx, pid)
	if err != nil {
		return nil, err
	}

	name, err := p.NameWithContext(ctx)
	if err != nil || strings.TrimSpace(name) == "" {
		if exe, err := p.ExeWithContext(ctx); err == nil && exe != "" {
			name = filepath.Base(exe)
		}
	}

	cmdline, _ := p.CmdlineWithContext(ctx)
	username, _ := p.UsernameWithContext(ctx)

	// Fallback for username via UIDs if UsernameWithContext is empty
	if username == "" {
		if uids, err := p.UidsWithContext(ctx); err == nil && len(uids) > 0 {
			if u, err := user.LookupId(strconv.Itoa(int(uids[0]))); err == nil && u.Username != "" {
				username = u.Username
			}
		}
	}

	return &ProcessDetails{
		Name:     name,
		Cmdline:  cmdline,
		Username: username,
	}, nil
}

// Scan performs a system scan for TCP listening ports and maps them to their respective processes.
func (s *DefaultScanner) Scan(ctx context.Context) ([]model.PortProcess, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	connections, err := s.connProvider(ctx, "tcp")
	if err != nil {
		return nil, err
	}

	procCache := make(map[int32]*ProcessDetails)
	type portPIDKey struct {
		port int
		pid  int32
	}

	dedupMap := make(map[portPIDKey]model.PortProcess)
	var keys []portPIDKey

	for _, conn := range connections {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Filter for TCP listening state
		if !strings.EqualFold(conn.Status, "LISTEN") {
			continue
		}

		port := int(conn.Laddr.Port)
		if port <= 0 {
			continue
		}

		protocol := "TCP"
		if strings.Contains(conn.Laddr.IP, ":") || net.ParseIP(conn.Laddr.IP).To4() == nil && conn.Laddr.IP != "" {
			protocol = "TCP6"
		}

		pid := conn.Pid
		procDetails := s.resolveProcess(ctx, pid, procCache)

		isContainer, containerType := IdentifyContainer(procDetails.Name, procDetails.Cmdline)

		item := model.PortProcess{
			Port:          port,
			Protocol:      protocol,
			PID:           pid,
			Name:          procDetails.Name,
			Cmdline:       procDetails.Cmdline,
			User:          procDetails.Username,
			IsContainer:   isContainer,
			ContainerType: containerType,
		}

		key := portPIDKey{port: port, pid: pid}
		if existing, exists := dedupMap[key]; exists {
			// If duplicate socket exists (e.g. dual-stack IPv4 and IPv6 bindings),
			// prefer generic "TCP" over "TCP6" if both are present.
			if existing.Protocol == "TCP6" && item.Protocol == "TCP" {
				dedupMap[key] = item
			}
			continue
		}

		dedupMap[key] = item
		keys = append(keys, key)
	}

	results := make([]model.PortProcess, 0, len(dedupMap))
	for _, key := range keys {
		results = append(results, dedupMap[key])
	}

	// Sort results ascending by port, then ascending by PID
	sort.Slice(results, func(i, j int) bool {
		if results[i].Port != results[j].Port {
			return results[i].Port < results[j].Port
		}
		return results[i].PID < results[j].PID
	})

	return results, nil
}

// resolveProcess obtains cached or newly queried process details, handling unprivileged/zero PIDs gracefully.
func (s *DefaultScanner) resolveProcess(ctx context.Context, pid int32, cache map[int32]*ProcessDetails) *ProcessDetails {
	if pid <= 0 {
		return &ProcessDetails{
			Name:     FallbackProcessName,
			Cmdline:  "",
			Username: "",
		}
	}

	if cached, ok := cache[pid]; ok {
		return cached
	}

	details, err := s.procProvider(ctx, pid)
	if err != nil || details == nil {
		details = &ProcessDetails{
			Name:     FallbackProcessName,
			Cmdline:  "",
			Username: "",
		}
	} else if strings.TrimSpace(details.Name) == "" {
		details.Name = FallbackProcessName
	}

	cache[pid] = details
	return details
}

// IdentifyContainer inspects executable name and command line against known container engines and runtimes.
func IdentifyContainer(name, cmdline string) (bool, string) {
	n := strings.ToLower(name)
	c := strings.ToLower(cmdline)
	combined := n + " " + c

	switch {
	case strings.Contains(combined, "docker-proxy") || strings.Contains(n, "dockerd"):
		return true, "Docker"
	case strings.Contains(combined, "podman") || strings.Contains(combined, "conmon"):
		return true, "Podman"
	case strings.Contains(combined, "containerd-shim") || strings.Contains(combined, "containerd") || strings.Contains(combined, "nerdctl"):
		return true, "Containerd"
	case reCrun.MatchString(combined):
		return true, "crun"
	case reRunc.MatchString(combined):
		return true, "runc"
	default:
		return false, ""
	}
}

// Scan is a package-level convenience function that executes a scan using DefaultScanner and context.Background().
func Scan() ([]model.PortProcess, error) {
	return NewDefaultScanner().Scan(context.Background())
}
