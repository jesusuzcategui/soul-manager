//go:build !windows

package killer

import "syscall"

// killProcess sends sig to pid using POSIX signals. A zero signal probes for
// process existence without delivering a signal.
func killProcess(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}
