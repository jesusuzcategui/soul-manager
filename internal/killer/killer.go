package killer

import (
	"errors"
	"os"
	"strings"
	"syscall"
)

var (
	// ErrInvalidPID is returned when a PID is <= 1 or negative.
	// This protects PID 0 (calling process group), PID 1 (init/systemd), and negative values.
	ErrInvalidPID = errors.New("invalid process ID: PID must be greater than 1")

	// ErrProcessAlreadyDead indicates that the targeted process is already terminated or does not exist.
	ErrProcessAlreadyDead = errors.New("process already dead")

	// ErrProcessNotFound is an alias for ErrProcessAlreadyDead.
	ErrProcessNotFound = ErrProcessAlreadyDead

	// ErrPermissionDenied is returned when the caller lacks required OS permissions to signal the process.
	ErrPermissionDenied = errors.New("permission denied")
)

// Killer defines the capability to terminate a process by PID.
type Killer interface {
	Kill(pid int32, force bool) error
}

// SignalFunc represents the function signature used to send an OS signal to a process.
type SignalFunc func(pid int, sig syscall.Signal) error

// Option defines a functional configuration option for DefaultKiller.
type Option func(*DefaultKiller)

// WithSignalFunc overrides the signal-sending function (primarily for mocking and unit testing).
func WithSignalFunc(fn SignalFunc) Option {
	return func(k *DefaultKiller) {
		k.signalFunc = fn
	}
}

// DefaultKiller provides the standard implementation of the Killer interface using OS signals.
type DefaultKiller struct {
	signalFunc SignalFunc
}

// Ensure DefaultKiller implements Killer interface.
var _ Killer = (*DefaultKiller)(nil)

// New returns a new Killer using default system signaling.
func New() Killer {
	return NewDefaultKiller()
}

// NewDefaultKiller returns a concrete *DefaultKiller configured with the default OS signal handler.
func NewDefaultKiller(opts ...Option) *DefaultKiller {
	k := &DefaultKiller{
		signalFunc: killProcess,
	}
	for _, opt := range opts {
		opt(k)
	}
	return k
}

// Kill terminates a process by sending a graceful (SIGTERM) or forced (SIGKILL) signal.
func (k *DefaultKiller) Kill(pid int32, force bool) error {
	if pid <= 1 {
		return ErrInvalidPID
	}

	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}

	signalFn := k.signalFunc
	if signalFn == nil {
		signalFn = killProcess
	}

	err := signalFn(int(pid), sig)
	if err == nil {
		return nil
	}

	return classifyError(err)
}

// IsAlive checks whether a process with the given PID is currently active.
func (k *DefaultKiller) IsAlive(pid int32) bool {
	if pid <= 1 {
		return false
	}

	signalFn := k.signalFunc
	if signalFn == nil {
		signalFn = killProcess
	}

	// Signal 0 checks for process existence without sending an actual signal.
	err := signalFn(int(pid), 0)
	if err == nil {
		return true
	}

	// If permission is denied, the process exists but is owned by another user (e.g. root).
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) || errors.Is(err, os.ErrPermission) {
		return true
	}

	return false
}

// Kill terminates a process using the package-level default killer.
func Kill(pid int32, force bool) error {
	return NewDefaultKiller().Kill(pid, force)
}

// IsAlive checks if a process is active using the package-level default killer.
func IsAlive(pid int32) bool {
	return NewDefaultKiller().IsAlive(pid)
}

// classifyError maps platform-specific OS errors to killer sentinel errors.
func classifyError(err error) error {
	if err == nil {
		return nil
	}

	// Pass through already classified killer sentinel errors
	if errors.Is(err, ErrInvalidPID) ||
		errors.Is(err, ErrProcessAlreadyDead) ||
		errors.Is(err, ErrPermissionDenied) {
		return err
	}

	// ESRCH / ErrProcessDone indicates the process does not exist or has finished
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
		return ErrProcessAlreadyDead
	}

	// EPERM / EACCES / ErrPermission indicates insufficient privileges
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) || errors.Is(err, os.ErrPermission) {
		return ErrPermissionDenied
	}

	// EINVAL indicates an invalid signal or PID argument
	if errors.Is(err, syscall.EINVAL) {
		return ErrInvalidPID
	}

	// Fallback string matching for wrapper errors or platform idiosyncrasies
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no such process") || strings.Contains(msg, "process already finished") {
		return ErrProcessAlreadyDead
	}
	if strings.Contains(msg, "operation not permitted") || strings.Contains(msg, "permission denied") {
		return ErrPermissionDenied
	}

	return err
}
