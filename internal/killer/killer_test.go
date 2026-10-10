package killer

import (
	"errors"
	"math"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestKill_InvalidPID(t *testing.T) {
	testPIDs := []struct {
		name string
		pid  int32
	}{
		{"PID 0", 0},
		{"PID 1 (init)", 1},
		{"Negative PID -1", -1},
		{"Negative PID -99", -99},
		{"MinInt32", math.MinInt32},
	}

	for _, tc := range testPIDs {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			mockKiller := NewDefaultKiller(WithSignalFunc(func(pid int, sig syscall.Signal) error {
				called = true
				return nil
			}))

			// Test graceful kill
			err := mockKiller.Kill(tc.pid, false)
			if !errors.Is(err, ErrInvalidPID) {
				t.Errorf("expected ErrInvalidPID for pid %d (graceful), got: %v", tc.pid, err)
			}
			if called {
				t.Errorf("signalFunc was called unexpectedly for invalid pid %d", tc.pid)
			}

			// Test forced kill
			called = false
			err = mockKiller.Kill(tc.pid, true)
			if !errors.Is(err, ErrInvalidPID) {
				t.Errorf("expected ErrInvalidPID for pid %d (forced), got: %v", tc.pid, err)
			}
			if called {
				t.Errorf("signalFunc was called unexpectedly for invalid pid %d", tc.pid)
			}
		})
	}
}

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

func TestKill_PermissionDenied_Mocked(t *testing.T) {
	testCases := []struct {
		name    string
		mockErr error
	}{
		{"syscall.EPERM", syscall.EPERM},
		{"syscall.EACCES", syscall.EACCES},
		{"os.ErrPermission", os.ErrPermission},
		{"wrapped permission denied", errors.New("operation not permitted")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k := NewDefaultKiller(WithSignalFunc(func(pid int, sig syscall.Signal) error {
				return tc.mockErr
			}))

			err := k.Kill(4242, false)
			if !errors.Is(err, ErrPermissionDenied) {
				t.Errorf("expected ErrPermissionDenied for %s, got: %v", tc.name, err)
			}
		})
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

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name        string
		input       error
		expected    error
		exactOrType bool
	}{
		{"nil error", nil, nil, true},
		{"ErrInvalidPID pass-through", ErrInvalidPID, ErrInvalidPID, true},
		{"ErrProcessAlreadyDead pass-through", ErrProcessAlreadyDead, ErrProcessAlreadyDead, true},
		{"ErrPermissionDenied pass-through", ErrPermissionDenied, ErrPermissionDenied, true},
		{"syscall.ESRCH", syscall.ESRCH, ErrProcessAlreadyDead, true},
		{"os.ErrProcessDone", os.ErrProcessDone, ErrProcessAlreadyDead, true},
		{"syscall.EPERM", syscall.EPERM, ErrPermissionDenied, true},
		{"syscall.EACCES", syscall.EACCES, ErrPermissionDenied, true},
		{"os.ErrPermission", os.ErrPermission, ErrPermissionDenied, true},
		{"syscall.EINVAL", syscall.EINVAL, ErrInvalidPID, true},
		{"text no such process", errors.New("something: no such process found"), ErrProcessAlreadyDead, true},
		{"text operation not permitted", errors.New("something: operation not permitted"), ErrPermissionDenied, true},
		{"text permission denied", errors.New("access: permission denied"), ErrPermissionDenied, true},
		{"unknown error", errors.New("unrelated I/O failure"), errors.New("unrelated I/O failure"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := classifyError(tc.input)
			if tc.expected == nil {
				if res != nil {
					t.Errorf("expected nil, got: %v", res)
				}
				return
			}

			if tc.exactOrType {
				if !errors.Is(res, tc.expected) {
					t.Errorf("expected error %v, got: %v", tc.expected, res)
				}
			} else {
				if res == nil || res.Error() != tc.expected.Error() {
					t.Errorf("expected error %v, got: %v", tc.expected, res)
				}
			}
		})
	}
}

func TestIsAlive(t *testing.T) {
	k := NewDefaultKiller()

	// Invalid PIDs
	if k.IsAlive(0) {
		t.Errorf("IsAlive(0) should be false")
	}
	if k.IsAlive(1) {
		// Even if PID 1 exists, IsAlive rejects PID <= 1
		if k.IsAlive(1) != false {
			t.Errorf("IsAlive(1) should be false")
		}
	}
	if k.IsAlive(-5) {
		t.Errorf("IsAlive(-5) should be false")
	}

	// Live process
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

	// Mock testing for permission denied (process is running but owned by someone else)
	mockKiller := NewDefaultKiller(WithSignalFunc(func(pid int, sig syscall.Signal) error {
		return syscall.EPERM
	}))
	if !mockKiller.IsAlive(8888) {
		t.Errorf("IsAlive expected true when signal returns EPERM (process exists)")
	}
}

func TestPackageLevelConvenience(t *testing.T) {
	// Verify New() interface instantiation
	killerInterface := New()
	if killerInterface == nil {
		t.Fatal("New() returned nil")
	}

	// Test package-level Kill on invalid PID
	if err := Kill(0, false); !errors.Is(err, ErrInvalidPID) {
		t.Errorf("package-level Kill(0) expected ErrInvalidPID, got: %v", err)
	}

	// Test package-level IsAlive on invalid PID
	if IsAlive(-1) {
		t.Errorf("package-level IsAlive(-1) expected false")
	}
}
