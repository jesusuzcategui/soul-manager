package killer

import (
	"errors"
	"math"
	"os"
	"syscall"
	"testing"
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

func TestIsAlive_InvalidAndMocked(t *testing.T) {
	k := NewDefaultKiller()

	// Invalid PIDs are rejected before any OS call.
	if k.IsAlive(0) {
		t.Errorf("IsAlive(0) should be false")
	}
	if k.IsAlive(1) != false {
		t.Errorf("IsAlive(1) should be false")
	}
	if k.IsAlive(-5) {
		t.Errorf("IsAlive(-5) should be false")
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
