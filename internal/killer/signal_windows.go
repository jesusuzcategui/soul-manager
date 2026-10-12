//go:build windows

package killer

import (
	"errors"
	"syscall"
)

// Windows process access rights and exit-code sentinels. Declared locally so the
// implementation does not depend on platform-specific exported identifiers.
const (
	processQueryLimitedInformation = 0x1000
	processTerminate               = 0x0001
	stillActive                    = 259 // STILL_ACTIVE
)

// killProcess emulates POSIX signalling on Windows:
//   - a zero signal performs a liveness probe, mirroring signal 0 on Unix;
//   - any real signal maps to a hard TerminateProcess, because Windows has no
//     SIGTERM/SIGKILL distinction.
//
// Errors are mapped to the same sentinels Unix produces (syscall.ESRCH for a
// missing process, syscall.EPERM for a denied one) so classifyError and
// IsAlive behave consistently across platforms.
func killProcess(pid int, sig syscall.Signal) error {
	if sig == 0 {
		return probeProcess(pid)
	}

	h, err := syscall.OpenProcess(processTerminate, false, uint32(pid))
	if err != nil {
		return mapOpenProcessError(err)
	}
	defer syscall.CloseHandle(h)

	return syscall.TerminateProcess(h, 1)
}

// probeProcess reports whether pid is alive, mirroring signal-0 semantics.
func probeProcess(pid int) error {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return mapOpenProcessError(err)
	}
	defer syscall.CloseHandle(h)

	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return syscall.ESRCH
	}
	if code == stillActive {
		return nil
	}
	return syscall.ESRCH
}

func mapOpenProcessError(err error) error {
	if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		return syscall.EPERM
	}
	return syscall.ESRCH
}
