package scanner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ContainerInfo is a lightweight snapshot of a running container discovered
// through a container engine CLI (docker, podman).
type ContainerInfo struct {
	// ID is the full container identifier (64 hex characters when available).
	ID string
	// Name is the human-readable container name.
	Name string
	// Engine is the container engine that reported the container (e.g. "docker").
	Engine string
	// Ports lists the host ports published by the container.
	Ports []int
}

// ContainerLister enumerates currently running containers across supported engines.
type ContainerLister interface {
	List(ctx context.Context) []ContainerInfo
}

// ContainerNameResolver maps an owning PID and listening port to the real name
// of the container exposing it. It returns an empty string when the owner is
// not a resolvable container.
type ContainerNameResolver interface {
	Resolve(ctx context.Context, pid int32, port int) string
}

// containerEngines are the supported engine CLIs, probed in order.
var containerEngines = []string{"docker", "podman"}

// defaultContainerCacheTTL bounds how often the engine CLIs are invoked.
const defaultContainerCacheTTL = 20 * time.Second

// commandContainerLister shells out to the supported engine CLIs.
type commandContainerLister struct {
	engines  []string
	timeout  time.Duration
	run      func(ctx context.Context, name string, args ...string) ([]byte, error)
	lookPath func(string) (string, error)
}

// newCommandContainerLister returns a lister backed by the local docker/podman CLIs.
func newCommandContainerLister() *commandContainerLister {
	return &commandContainerLister{
		engines: containerEngines,
		timeout: 3 * time.Second,
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		lookPath: exec.LookPath,
	}
}

// List queries each available engine CLI for running containers, ignoring
// engines that are absent or that fail.
func (l *commandContainerLister) List(ctx context.Context) []ContainerInfo {
	var out []ContainerInfo
	for _, engine := range l.engines {
		if _, err := l.lookPath(engine); err != nil {
			continue
		}

		cmdCtx, cancel := context.WithTimeout(ctx, l.timeout)
		data, err := l.run(cmdCtx, engine, "ps", "--no-trunc", "--format", "{{.ID}}\t{{.Names}}\t{{.Ports}}")
		cancel()
		if err != nil {
			continue
		}

		out = append(out, parseContainerListing(engine, string(data))...)
	}
	return dedupeByID(out)
}

// dedupeByID removes duplicate containers (e.g. when docker and podman report
// the same backing daemon), keeping the first occurrence.
func dedupeByID(infos []ContainerInfo) []ContainerInfo {
	seen := make(map[string]struct{}, len(infos))
	out := make([]ContainerInfo, 0, len(infos))
	for _, info := range infos {
		if info.ID == "" {
			out = append(out, info)
			continue
		}
		if _, ok := seen[info.ID]; ok {
			continue
		}
		seen[info.ID] = struct{}{}
		out = append(out, info)
	}
	return out
}

// rePublishedPort captures the host port that precedes the "->" port mapping.
var rePublishedPort = regexp.MustCompile(`(\d+)->`)

// reHexID captures container-id-like hex runs from a cgroup path.
var reHexID = regexp.MustCompile(`[0-9a-f]{12,64}`)

// parseContainerListing parses tab-separated `ps --format` output.
func parseContainerListing(engine, output string) []ContainerInfo {
	var infos []ContainerInfo
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 2 {
			continue
		}

		id := strings.TrimSpace(fields[0])
		name := strings.TrimSpace(fields[1])
		if id == "" || name == "" {
			continue
		}

		info := ContainerInfo{ID: id, Name: name, Engine: engine}
		if len(fields) == 3 {
			info.Ports = parsePublishedPorts(fields[2])
		}
		infos = append(infos, info)
	}
	return infos
}

// parsePublishedPorts extracts the distinct host ports from an engine port
// mapping string such as "0.0.0.0:8080->80/tcp, :::8080->80/tcp".
func parsePublishedPorts(ports string) []int {
	seen := make(map[int]bool)
	var out []int
	for _, m := range rePublishedPort.FindAllStringSubmatch(ports, -1) {
		p, err := strconv.Atoi(m[1])
		if err != nil || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// extractContainerIDs returns container-id-like hex runs found in a cgroup path.
func extractContainerIDs(content string) []string {
	return reHexID.FindAllString(strings.ToLower(content), -1)
}

// defaultCgroupReader reads the cgroup membership for a PID from /proc.
func defaultCgroupReader(pid int32) (string, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(int(pid)), "cgroup"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// cachedContainerResolver resolves container names while caching engine listings.
type cachedContainerResolver struct {
	lister     ContainerLister
	readCgroup func(pid int32) (string, error)
	ttl        time.Duration

	mu       sync.Mutex
	cached   []ContainerInfo
	lastList time.Time
}

// NewContainerNameResolver returns the default resolver backed by docker/podman.
func NewContainerNameResolver() ContainerNameResolver {
	return &cachedContainerResolver{
		lister:     newCommandContainerLister(),
		readCgroup: defaultCgroupReader,
		ttl:        defaultContainerCacheTTL,
	}
}

// Resolve matches the PID's cgroup against the running containers first and
// falls back to matching the published listening port.
func (r *cachedContainerResolver) Resolve(ctx context.Context, pid int32, port int) string {
	infos := r.snapshot(ctx)
	if len(infos) == 0 {
		return ""
	}

	if pid > 0 {
		if content, err := r.readCgroup(pid); err == nil {
			if name := matchContainerByCgroup(infos, content); name != "" {
				return name
			}
		}
	}

	if port > 0 {
		if name := matchContainerByPort(infos, port); name != "" {
			return name
		}
	}

	return ""
}

// snapshot returns the cached container list, refreshing it when stale.
func (r *cachedContainerResolver) snapshot(ctx context.Context) []ContainerInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cached != nil && time.Since(r.lastList) < r.ttl {
		return r.cached
	}

	infos := r.lister.List(ctx)
	r.cached = infos
	r.lastList = time.Now()
	return infos
}

// matchContainerByCgroup finds a container whose ID matches one of the cgroup
// path's hex identifiers.
func matchContainerByCgroup(infos []ContainerInfo, cgroupContent string) string {
	ids := extractContainerIDs(cgroupContent)
	for _, info := range infos {
		for _, id := range ids {
			if containerIDMatches(info.ID, id) {
				return info.Name
			}
		}
	}
	return ""
}

// containerIDMatches compares container IDs allowing for truncated forms.
func containerIDMatches(full, candidate string) bool {
	if full == "" || candidate == "" {
		return false
	}
	return strings.HasPrefix(full, candidate) || strings.HasPrefix(candidate, full)
}

// matchContainerByPort finds a container publishing the given host port.
func matchContainerByPort(infos []ContainerInfo, port int) string {
	for _, info := range infos {
		for _, p := range info.Ports {
			if p == port {
				return info.Name
			}
		}
	}
	return ""
}
