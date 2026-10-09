package model

// PortProcess represents a network port bound to an active system process or container.
type PortProcess struct {
	// Port is the listening network port number (e.g. 80, 8080).
	Port int `json:"port"`

	// Protocol is the transport protocol used (e.g. "TCP", "TCP6").
	Protocol string `json:"protocol"`

	// PID is the operating system process identifier.
	PID int32 `json:"pid"`

	// Name is the executable or process name (e.g. "node", "docker-proxy").
	Name string `json:"name"`

	// Cmdline is the full command-line invocation of the process.
	Cmdline string `json:"cmdline"`

	// IsContainer indicates whether the process is associated with a container engine.
	IsContainer bool `json:"is_container"`

	// ContainerType identifies the container runtime or proxy type (e.g. "docker", "podman", "containerd").
	ContainerType string `json:"container_type"`
}
