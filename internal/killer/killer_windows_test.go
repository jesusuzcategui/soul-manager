//go:build windows

package killer

import (
	"errors"
	"os/exec"
	"testing"
	"time"
)

// spawnSleeper starts a long-lived child process using a command that exists on
// Windows, so the Windows signalling path is exercised on the CI runner.
func spawnSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("cmd", "/c", "ping", "-n", "60", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to spawn child process: %v", err)
	}
	return cmd
}

func waitForExit(t *testing.T, cmd *exec.Cmd, pid int32, how string) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process terminated successfully.
	case <-time.After(5 * time.Second):
		t.Fatalf("process %d failed to terminate within 5 seconds after %s", pid, how)
	}
}

func TestKill_Graceful_TerminatesProcess(t *testing.T) {
	cmd := spawnSleeper(t)
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

	if err := k.Kill(pid, false); err != nil {
		t.Fatalf("Kill(false) failed unexpectedly: %v", err)
	}

	waitForExit(t, cmd, pid, "Kill(false)")

	if err := k.Kill(pid, false); !errors.Is(err, ErrProcessAlreadyDead) {
		t.Errorf("expected ErrProcessAlreadyDead after process exit, got: %v", err)
	}
}

func TestKill_Forced_TerminatesProcess(t *testing.T) {
	cmd := spawnSleeper(t)
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

	if err := k.Kill(pid, true); err != nil {
		t.Fatalf("Kill(true) failed unexpectedly: %v", err)
	}

	waitForExit(t, cmd, pid, "Kill(true)")

	if err := k.Kill(pid, true); !errors.Is(err, ErrProcessAlreadyDead) {
		t.Errorf("expected ErrProcessAlreadyDead after process exit, got: %v", err)
	}
}

func TestIsAlive_LiveProcess(t *testing.T) {
	k := NewDefaultKiller()

	cmd := spawnSleeper(t)
	pid := int32(cmd.Process.Pid)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	if !k.IsAlive(pid) {
		t.Errorf("IsAlive(%d) expected true for running process", pid)
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	if k.IsAlive(pid) {
		t.Errorf("IsAlive(%d) expected false after process reaped", pid)
	}
}
