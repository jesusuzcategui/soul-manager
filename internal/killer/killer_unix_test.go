//go:build !windows

package killer

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestKill_ProcessAlreadyDead(t *testing.T) {
	k := NewDefaultKiller()

	// 1. Test with an unused, highly unlikely PID
	deadPID := int32(999999)
	for i := int32(990000); i < 999999; i++ {
		// Verify that PID does not exist via signal 0
		if err := syscall.Kill(int(i), 0); errors.Is(err, syscall.ESRCH) {
			deadPID = i
			break
		}
	}

	errGraceful := k.Kill(deadPID, false)
	if !errors.Is(errGraceful, ErrProcessAlreadyDead) {
		t.Errorf("expected ErrProcessAlreadyDead for non-existent PID %d, got: %v", deadPID, errGraceful)
	}
	if !errors.Is(errGraceful, ErrProcessNotFound) {
		t.Errorf("expected ErrProcessNotFound to match as well, got: %v", errGraceful)
	}

	errForced := k.Kill(deadPID, true)
	if !errors.Is(errForced, ErrProcessAlreadyDead) {
		t.Errorf("expected ErrProcessAlreadyDead for non-existent PID %d (force), got: %v", deadPID, errForced)
	}

	// 2. Test with an ephemeral child process that has already exited and been reaped
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run dummy command: %v", err)
	}
	reapedPID := int32(cmd.Process.Pid)

	errReaped := k.Kill(reapedPID, false)
	if !errors.Is(errReaped, ErrProcessAlreadyDead) {
		t.Errorf("expected ErrProcessAlreadyDead for reaped process %d, got: %v", reapedPID, errReaped)
	}
}

func TestKill_Graceful_TerminatesProcess(t *testing.T) {
	cmd := exec.Command("sleep", "100")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to spawn child process: %v", err)
	}
	pid := int32(cmd.Process.Pid)
	defer func() {
		// Clean up in case of test failure
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	k := NewDefaultKiller()

	// Verify process is initially alive
	if !k.IsAlive(pid) {
		t.Fatalf("expected child process %d to be alive initially", pid)
	}

	// Send graceful termination (SIGTERM)
	if err := k.Kill(pid, false); err != nil {
		t.Fatalf("Kill(false) failed unexpectedly: %v", err)
	}

	// Wait for the process to exit
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process terminated successfully
	case <-time.After(3 * time.Second):
		t.Fatalf("process %d failed to terminate within 3 seconds after SIGTERM", pid)
	}

	// Once terminated and reaped, subsequent kill must return ErrProcessAlreadyDead
	if err := k.Kill(pid, false); !errors.Is(err, ErrProcessAlreadyDead) {
		t.Errorf("expected ErrProcessAlreadyDead after process exit, got: %v", err)
	}
}

func TestKill_Forced_TerminatesProcess(t *testing.T) {
	cmd := exec.Command("sleep", "100")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to spawn child process: %v", err)
	}
	pid := int32(cmd.Process.Pid)
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	k := NewDefaultKiller()

	if !k.IsAlive(pid) {
		t.Fatalf("expected child process %d to be alive initially", pid)
	}

	// Send forced termination (SIGKILL)
	if err := k.Kill(pid, true); err != nil {
		t.Fatalf("Kill(true) failed unexpectedly: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process terminated successfully
	case <-time.After(3 * time.Second):
		t.Fatalf("process %d failed to terminate within 3 seconds after SIGKILL", pid)
	}

	if err := k.Kill(pid, true); !errors.Is(err, ErrProcessAlreadyDead) {
		t.Errorf("expected ErrProcessAlreadyDead after process exit, got: %v", err)
	}
}

func TestKill_PermissionDenied_RealSystem(t *testing.T) {
	// Signal 0 proves this host refuses signals to PID 2 before the test sends
	// SIGTERM. Some containers expose a different process as PID 2 or grant
	// CAP_KILL, so a successful probe must skip rather than terminate it.
	if err := syscall.Kill(2, 0); !errors.Is(err, syscall.EPERM) {
		t.Skip("PID 2 is absent or signalable on this host")
	}

	k := NewDefaultKiller()
	err := k.Kill(2, false)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied when killing root PID 2 as non-root, got: %v", err)
	}
}

func TestIsAlive_LiveProcess(t *testing.T) {
	k := NewDefaultKiller()

	cmd := exec.Command("sleep", "100")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to spawn child process: %v", err)
	}
	pid := int32(cmd.Process.Pid)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	if !k.IsAlive(pid) {
		t.Errorf("IsAlive(%d) expected true for running process", pid)
	}

	// Kill and wait
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	if k.IsAlive(pid) {
		t.Errorf("IsAlive(%d) expected false after process reaped", pid)
	}
}
