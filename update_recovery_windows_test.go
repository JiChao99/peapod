//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsUpdateRecoveryReplacementFailurePreservesState(t *testing.T) {
	state, path, _, _ := newWindowsRecoveryFixture(t)
	if err := createWindowsUpdateRecovery(path, state); err != nil {
		t.Fatal(err)
	}
	locked, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Close()
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(locked.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped); err != nil {
		t.Fatal(err)
	}
	defer windows.UnlockFileEx(windows.Handle(locked.Fd()), 0, 1, 0, &overlapped)
	state.Version = "0.15.0"
	if err := writeWindowsUpdateRecovery(path, state); err == nil {
		t.Fatal("locked recovery state replaced")
	}
	if err := windows.UnlockFileEx(windows.Handle(locked.Fd()), 0, 1, 0, &overlapped); err != nil {
		t.Fatal(err)
	}
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := readWindowsUpdateRecovery(path)
	if err != nil || got.Version != "0.14.0" {
		t.Fatalf("recovery state = %+v, err = %v", got, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("recovery directory entries = %v, err = %v", entries, err)
	}
}

func TestRecoverWindowsUpdateRetainsStateWhenWorkDirLocked(t *testing.T) {
	state, path, executable, tempDir := newWindowsRecoveryFixture(t)
	if err := writeWindowsUpdateRecovery(path, state); err != nil {
		t.Fatal(err)
	}
	lockedPath := filepath.Join(state.WorkDir, "helper.exe")
	locked, err := os.OpenFile(lockedPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Close()
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(locked.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped); err != nil {
		t.Fatal(err)
	}
	defer windows.UnlockFileEx(windows.Handle(locked.Fd()), 0, 1, 0, &overlapped)

	result := recoverWindowsUpdate(path, executable, tempDir)
	if result.Err == nil || result.Cleaned || result.RetainedPath != state.WorkDir {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("recovery state removed while directory locked: %v", err)
	}
	if _, err := os.Stat(state.BackupDir); err != nil {
		t.Fatalf("backup removed while temporary directory locked: %v", err)
	}
	if err := windows.UnlockFileEx(windows.Handle(locked.Fd()), 0, 1, 0, &overlapped); err != nil {
		t.Fatal(err)
	}
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	if result := recoverWindowsUpdate(path, executable, tempDir); result.Err != nil || !result.Cleaned {
		t.Fatalf("retry result = %+v", result)
	}
}
