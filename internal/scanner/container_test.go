package scanner

import (
	"context"
	"errors"
	"testing"
	"time"

	gopsnet "github.com/shirou/gopsutil/v3/net"
)

func TestParsePublishedPorts(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []int
	}{
		{"empty", "", nil},
		{"single ipv4", "0.0.0.0:8080->80/tcp", []int{8080}},
		{"dual stack duplicate", "0.0.0.0:8080->80/tcp, :::8080->80/tcp", []int{8080}},
		{"multiple", "0.0.0.0:5432->5432/tcp, 0.0.0.0:6379->6379/tcp", []int{5432, 6379}},
		{"loopback", "127.0.0.1:3000->3000/tcp", []int{3000}},
		{"ipv6 only", ":::9090->9090/tcp", []int{9090}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePublishedPorts(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("parsePublishedPorts(%q) = %v, want %v", tc.input, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("parsePublishedPorts(%q) = %v, want %v", tc.input, got, tc.want)
				}
			}
		})
	}
}

func TestParseContainerListing(t *testing.T) {
	output := "abc123def456\tmy-postgres\t0.0.0.0:5432->5432/tcp, :::5432->5432/tcp\n" +
		"deadbeef0000\tweb\t0.0.0.0:8080->80/tcp\n" +
		"\n" +
		"malformed-line-without-name\n"

	infos := parseContainerListing("docker", output)
	if len(infos) != 2 {
		t.Fatalf("expected 2 containers, got %d: %+v", len(infos), infos)
	}

	if infos[0].ID != "abc123def456" || infos[0].Name != "my-postgres" || infos[0].Engine != "docker" {
		t.Fatalf("unexpected first container: %+v", infos[0])
	}
	if len(infos[0].Ports) != 1 || infos[0].Ports[0] != 5432 {
		t.Fatalf("unexpected first container ports: %+v", infos[0].Ports)
	}

	if infos[1].Name != "web" || len(infos[1].Ports) != 1 || infos[1].Ports[0] != 8080 {
		t.Fatalf("unexpected second container: %+v", infos[1])
	}
}

func TestExtractContainerIDs(t *testing.T) {
	cgroup := "0::/user.slice/user-1000.slice/libpod-1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef.scope"
	ids := extractContainerIDs(cgroup)

	var found bool
	for _, id := range ids {
		if containerIDMatches("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", id) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected to extract the libpod container id, got %v", ids)
	}

	// Docker cgroup v1 form.
	dockerIDs := extractContainerIDs("12:pids:/docker/abcdef1234567890")
	if len(dockerIDs) == 0 || !containerIDMatches("abcdef1234567890", dockerIDs[0]) {
		t.Fatalf("expected docker id, got %v", dockerIDs)
	}
}

func TestCommandContainerLister_List(t *testing.T) {
	lister := &commandContainerLister{
		engines: []string{"docker", "podman-missing"},
		timeout: time.Second,
		lookPath: func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", errors.New("not installed")
		},
		run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("abc123def456\tweb\t0.0.0.0:8080->80/tcp\n"), nil
		},
	}

	infos := lister.List(context.Background())
	if len(infos) != 1 {
		t.Fatalf("expected 1 container, got %d: %+v", len(infos), infos)
	}
	if infos[0].Name != "web" || infos[0].Engine != "docker" {
		t.Fatalf("unexpected container: %+v", infos[0])
	}
}

func TestDedupeByID(t *testing.T) {
	infos := []ContainerInfo{
		{ID: "aaaa", Name: "web", Engine: "docker", Ports: []int{8080}},
		{ID: "bbbb", Name: "db", Engine: "docker", Ports: []int{5432}},
		{ID: "aaaa", Name: "web", Engine: "podman", Ports: []int{8080}},
		{ID: "", Name: "unknown", Engine: "podman"},
	}

	got := dedupeByID(infos)
	if len(got) != 3 {
		t.Fatalf("expected 3 deduped containers, got %d: %+v", len(got), got)
	}
	if got[0].Engine != "docker" {
		t.Fatalf("expected first occurrence to win, got engine %q", got[0].Engine)
	}
	if got[2].Name != "unknown" {
		t.Fatalf("expected id-less entry to be preserved, got %+v", got[2])
	}
}

type fakeContainerLister struct {
	infos []ContainerInfo
	calls int
}

func (f *fakeContainerLister) List(context.Context) []ContainerInfo {
	f.calls++
	return f.infos
}

func newTestResolver(lister ContainerLister, cgroup func(int32) (string, error)) *cachedContainerResolver {
	return &cachedContainerResolver{
		lister:     lister,
		readCgroup: cgroup,
		ttl:        defaultContainerCacheTTL,
	}
}

func TestResolver_ResolvesByCgroup(t *testing.T) {
	lister := &fakeContainerLister{infos: []ContainerInfo{
		{ID: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", Name: "my-postgres", Engine: "podman"},
	}}
	resolver := newTestResolver(lister, func(int32) (string, error) {
		return "0::/user.slice/libpod-1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef.scope", nil
	})

	if got := resolver.Resolve(context.Background(), 4242, 5432); got != "my-postgres" {
		t.Fatalf("Resolve by cgroup = %q, want %q", got, "my-postgres")
	}
}

func TestResolver_FallsBackToPublishedPort(t *testing.T) {
	lister := &fakeContainerLister{infos: []ContainerInfo{
		{ID: "abcdefabcdef0011", Name: "web", Engine: "docker", Ports: []int{8080}},
	}}
	// docker-proxy runs on the host, so its cgroup does not reference the container.
	resolver := newTestResolver(lister, func(int32) (string, error) {
		return "0::/system.slice/docker.service", nil
	})

	if got := resolver.Resolve(context.Background(), 99, 8080); got != "web" {
		t.Fatalf("Resolve by port = %q, want %q", got, "web")
	}
}

func TestResolver_UnresolvedReturnsEmpty(t *testing.T) {
	lister := &fakeContainerLister{infos: []ContainerInfo{
		{ID: "abcdefabcdef0011", Name: "web", Engine: "docker", Ports: []int{8080}},
	}}
	resolver := newTestResolver(lister, func(int32) (string, error) {
		return "", errors.New("permission denied reading /proc")
	})

	if got := resolver.Resolve(context.Background(), 99, 9999); got != "" {
		t.Fatalf("expected empty resolution, got %q", got)
	}

	if got := resolver.Resolve(context.Background(), 99, 0); got != "" {
		t.Fatalf("expected empty resolution for port 0, got %q", got)
	}
}

func TestResolver_CachesListings(t *testing.T) {
	lister := &fakeContainerLister{infos: []ContainerInfo{
		{ID: "abcdefabcdef0011", Name: "web", Engine: "docker", Ports: []int{8080}},
	}}
	resolver := newTestResolver(lister, func(int32) (string, error) {
		return "", errors.New("no cgroup")
	})

	for i := 0; i < 5; i++ {
		resolver.Resolve(context.Background(), 0, 8080)
	}
	if lister.calls != 1 {
		t.Fatalf("expected a single engine listing within the TTL, got %d", lister.calls)
	}

	// Expiring the TTL forces a refresh.
	resolver.ttl = 0
	resolver.Resolve(context.Background(), 0, 8080)
	if lister.calls != 2 {
		t.Fatalf("expected a refresh after TTL expiry, got %d calls", lister.calls)
	}
}

type fakeContainerNameResolver struct {
	name string
}

func (f fakeContainerNameResolver) Resolve(context.Context, int32, int) string { return f.name }

func TestScan_ResolvesContainerName(t *testing.T) {
	mockConns := []gopsnet.ConnectionStat{
		{Status: "LISTEN", Laddr: gopsnet.Addr{IP: "0.0.0.0", Port: 8080}, Pid: 7},
	}

	scanner := &DefaultScanner{
		connProvider: func(context.Context, string) ([]gopsnet.ConnectionStat, error) {
			return mockConns, nil
		},
		procProvider: func(context.Context, int32) (*ProcessDetails, error) {
			return &ProcessDetails{Name: "docker-proxy", Cmdline: "/usr/bin/docker-proxy -host-port 8080"}, nil
		},
		containerResolver: fakeContainerNameResolver{name: "my-postgres"},
	}

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsContainer {
		t.Fatalf("expected container process, got %+v", results[0])
	}
	if results[0].ContainerName != "my-postgres" {
		t.Fatalf("ContainerName = %q, want %q", results[0].ContainerName, "my-postgres")
	}
}

func TestScan_NonContainerSkipsResolver(t *testing.T) {
	called := false
	mockConns := []gopsnet.ConnectionStat{
		{Status: "LISTEN", Laddr: gopsnet.Addr{IP: "0.0.0.0", Port: 3000}, Pid: 11},
	}

	scanner := &DefaultScanner{
		connProvider: func(context.Context, string) ([]gopsnet.ConnectionStat, error) {
			return mockConns, nil
		},
		procProvider: func(context.Context, int32) (*ProcessDetails, error) {
			return &ProcessDetails{Name: "node", Cmdline: "node server.js"}, nil
		},
		containerResolver: fakeNameResolverFunc(func(context.Context, int32, int) string {
			called = true
			return "should-not-be-used"
		}),
	}

	results, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("resolver must not be called for non-container processes")
	}
	if results[0].ContainerName != "" {
		t.Fatalf("expected empty container name, got %q", results[0].ContainerName)
	}
}

type fakeNameResolverFunc func(context.Context, int32, int) string

func (f fakeNameResolverFunc) Resolve(ctx context.Context, pid int32, port int) string {
	return f(ctx, pid, port)
}
