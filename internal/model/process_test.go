package model

import (
	"encoding/json"
	"testing"
)

func TestPortProcessJSON(t *testing.T) {
	proc := PortProcess{
		Port:          8080,
		Protocol:      "TCP",
		PID:           1234,
		Name:          "node",
		Cmdline:       "node server.js",
		IsContainer:   true,
		ContainerType: "docker",
	}

	data, err := json.Marshal(proc)
	if err != nil {
		t.Fatalf("failed to marshal PortProcess: %v", err)
	}

	var unmarshaled PortProcess
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal PortProcess: %v", err)
	}

	if unmarshaled.Port != proc.Port {
		t.Errorf("expected port %d, got %d", proc.Port, unmarshaled.Port)
	}
	if unmarshaled.PID != proc.PID {
		t.Errorf("expected pid %d, got %d", proc.PID, unmarshaled.PID)
	}
	if unmarshaled.Name != proc.Name {
		t.Errorf("expected name %q, got %q", proc.Name, unmarshaled.Name)
	}
	if !unmarshaled.IsContainer {
		t.Errorf("expected is_container to be true")
	}
	if unmarshaled.ContainerType != "docker" {
		t.Errorf("expected container_type to be %q, got %q", proc.ContainerType, unmarshaled.ContainerType)
	}
}
