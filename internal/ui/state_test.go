package ui

import (
	"fmt"
	"sync"
	"testing"

	"github.com/jesusuzcategui/soul-app/internal/model"
)

func sampleProcesses() []model.PortProcess {
	return []model.PortProcess{
		{
			Port:          80,
			Protocol:      "TCP",
			PID:           1001,
			Name:          "nginx",
			Cmdline:       "/usr/sbin/nginx -g daemon on;",
			IsContainer:   false,
			ContainerType: "",
		},
		{
			Port:          3000,
			Protocol:      "TCP",
			PID:           2002,
			Name:          "node",
			Cmdline:       "node server.js",
			IsContainer:   false,
			ContainerType: "",
		},
		{
			Port:          5432,
			Protocol:      "TCP",
			PID:           3003,
			Name:          "docker-proxy",
			Cmdline:       "/usr/bin/docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 5432",
			IsContainer:   true,
			ContainerType: "docker",
		},
		{
			Port:          8080,
			Protocol:      "TCP6",
			PID:           4004,
			Name:          "conmon",
			Cmdline:       "conmon -s -c abc123def456",
			IsContainer:   true,
			ContainerType: "podman",
		},
		{
			Port:          9090,
			Protocol:      "TCP",
			PID:           5005,
			Name:          "prometheus",
			Cmdline:       "/bin/prometheus --config.file=/etc/prometheus.yml",
			IsContainer:   false,
			ContainerType: "",
		},
	}
}

func TestState_EmptyInitial(t *testing.T) {
	s := NewState()
	if filtered, total := s.Count(); filtered != 0 || total != 0 {
		t.Fatalf("expected 0/0, got %d/%d", filtered, total)
	}
	if len(s.GetFiltered()) != 0 {
		t.Fatalf("expected empty filtered list, got %d", len(s.GetFiltered()))
	}
	if len(s.GetAll()) != 0 {
		t.Fatalf("expected empty all list, got %d", len(s.GetAll()))
	}
	if s.GetQuery() != "" {
		t.Fatalf("expected empty query, got %q", s.GetQuery())
	}
}

func TestState_SetProcesses_WithoutFilter(t *testing.T) {
	s := NewState()
	procs := sampleProcesses()

	filtered := s.SetProcesses(procs)
	if len(filtered) != len(procs) {
		t.Fatalf("expected %d filtered items, got %d", len(procs), len(filtered))
	}
	if fCount, tCount := s.Count(); fCount != len(procs) || tCount != len(procs) {
		t.Fatalf("expected count %d/%d, got %d/%d", len(procs), len(procs), fCount, tCount)
	}
}

func TestState_FilterByPort(t *testing.T) {
	s := NewState()
	s.SetProcesses(sampleProcesses())

	filtered := s.SetQuery("5432")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 process for port 5432, got %d", len(filtered))
	}
	if filtered[0].Port != 5432 || filtered[0].Name != "docker-proxy" {
		t.Fatalf("unexpected process matched: %+v", filtered[0])
	}

	// Partial port match "80" should match 80 and 8080
	filtered = s.SetQuery("80")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 processes for port prefix 80, got %d", len(filtered))
	}
}

func TestState_FilterByProcessName(t *testing.T) {
	s := NewState()
	s.SetProcesses(sampleProcesses())

	filtered := s.SetQuery("node")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 process for 'node', got %d", len(filtered))
	}
	if filtered[0].Name != "node" {
		t.Fatalf("expected node, got %s", filtered[0].Name)
	}

	// Case insensitive test
	filtered = s.SetQuery("NoDe")
	if len(filtered) != 1 || filtered[0].Name != "node" {
		t.Fatalf("expected case-insensitive match for NoDe, got %d items", len(filtered))
	}
}

func TestState_FilterByPID(t *testing.T) {
	s := NewState()
	s.SetProcesses(sampleProcesses())

	filtered := s.SetQuery("4004")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 process for PID 4004, got %d", len(filtered))
	}
	if filtered[0].PID != 4004 {
		t.Fatalf("expected PID 4004, got %d", filtered[0].PID)
	}
}

func TestState_FilterByContainer(t *testing.T) {
	s := NewState()
	s.SetProcesses(sampleProcesses())

	// Filter by container type "docker"
	filtered := s.SetQuery("docker")
	if len(filtered) != 1 || filtered[0].ContainerType != "docker" {
		t.Fatalf("expected 1 docker container process, got %d", len(filtered))
	}

	// Filter by container type "podman"
	filtered = s.SetQuery("podman")
	if len(filtered) != 1 || filtered[0].ContainerType != "podman" {
		t.Fatalf("expected 1 podman container process, got %d", len(filtered))
	}

	// Filter by generic "container" keyword
	filtered = s.SetQuery("container")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 container processes for 'container', got %d", len(filtered))
	}
}

func TestState_FilterByContainerName(t *testing.T) {
	s := NewState()
	s.SetProcesses([]model.PortProcess{
		{Port: 5432, PID: 1, Name: "conmon", IsContainer: true, ContainerType: "podman", ContainerName: "my-postgres"},
		{Port: 8080, PID: 2, Name: "docker-proxy", IsContainer: true, ContainerType: "docker", ContainerName: "web-frontend"},
	})

	filtered := s.SetQuery("my-postgres")
	if len(filtered) != 1 || filtered[0].ContainerName != "my-postgres" {
		t.Fatalf("expected 1 process matching container name, got %d", len(filtered))
	}

	// Case-insensitive match.
	filtered = s.SetQuery("WEB-FRONT")
	if len(filtered) != 1 || filtered[0].ContainerName != "web-frontend" {
		t.Fatalf("expected case-insensitive container name match, got %d", len(filtered))
	}
}

func TestState_FilterByProtocol(t *testing.T) {
	s := NewState()
	s.SetProcesses(sampleProcesses())

	filtered := s.SetQuery("tcp6")
	if len(filtered) != 1 || filtered[0].Protocol != "TCP6" {
		t.Fatalf("expected 1 TCP6 process, got %d", len(filtered))
	}
}

func TestState_ResetFilter(t *testing.T) {
	s := NewState()
	procs := sampleProcesses()
	s.SetProcesses(procs)

	s.SetQuery("node")
	if len(s.GetFiltered()) != 1 {
		t.Fatalf("expected 1 item with query")
	}

	// Reset with empty query
	filtered := s.SetQuery("")
	if len(filtered) != len(procs) {
		t.Fatalf("expected all %d items after reset, got %d", len(procs), len(filtered))
	}
}

func TestState_ConcurrencyRace(t *testing.T) {
	s := NewState()
	procs := sampleProcesses()
	s.SetProcesses(procs)

	var wg sync.WaitGroup
	workers := 10
	iterations := 100

	for i := 0; i < workers; i++ {
		wg.Add(3)

		// Reader 1: GetFiltered
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = s.GetFiltered()
				_ = s.GetQuery()
			}
		}(i)

		// Reader 2: Count and GetAll
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = s.Count()
				_ = s.GetAll()
			}
		}(i)

		// Writer: update query or processes
		go func(workerID int) {
			defer wg.Done()
			queries := []string{"", "80", "node", "docker", "podman", "nonexistent"}
			for j := 0; j < iterations; j++ {
				q := queries[j%len(queries)]
				s.SetQuery(q)
				if j%10 == 0 {
					newProcs := append(sampleProcesses(), model.PortProcess{
						Port: 10000 + workerID*iterations + j,
						PID:  int32(20000 + j),
						Name: fmt.Sprintf("proc-%d", j),
					})
					s.SetProcesses(newProcs)
				}
			}
		}(i)
	}

	wg.Wait()
}
