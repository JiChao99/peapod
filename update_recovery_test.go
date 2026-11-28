package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsUpdateRecoveryRoundTrip(t *testing.T) {
	state, path, _, _ := newWindowsRecoveryFixture(t)
	if err := writeWindowsUpdateRecovery(path, state); err != nil {
		t.Fatal(err)
	}
	got, err := readWindowsUpdateRecovery(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != state.Version || got.Phase != state.Phase || got.InstallDir != state.InstallDir || got.BackupDir != state.BackupDir || got.WorkDir != state.WorkDir {
		t.Fatalf("recovery state = %+v, want %+v", got, state)
	}
	state.Phase = windowsRecoveryInstalled
	state.LastError = "restart failed"
	if err := writeWindowsUpdateRecovery(path, state); err != nil {
		t.Fatal(err)
	}
	got, err = readWindowsUpdateRecovery(path)
	if err != nil || got.Phase != windowsRecoveryInstalled || got.LastError != state.LastError {
		t.Fatalf("updated recovery state = %+v, err = %v", got, err)
	}
}

func TestCreateWindowsUpdateRecoveryDoesNotOverwrite(t *testing.T) {
	state, path, _, _ := newWindowsRecoveryFixture(t)
	if err := createWindowsUpdateRecovery(path, state); err != nil {
		t.Fatal(err)
	}
	state.Version = "0.15.0"
	if err := createWindowsUpdateRecovery(path, state); err == nil {
		t.Fatal("existing recovery state overwritten")
	}
	got, err := readWindowsUpdateRecovery(path)
	if err != nil || got.Version != "0.14.0" {
		t.Fatalf("recovery state = %+v, err = %v", got, err)
	}
}

func TestWindowsUpdateRecoveryRejectsInvalidStateFiles(t *testing.T) {
	for _, data := range [][]byte{[]byte("{"), []byte(strings.Repeat("x", windowsRecoveryMaxBytes+1))} {
		path := filepath.Join(t.TempDir(), "update-recovery.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readWindowsUpdateRecovery(path); err == nil {
			t.Fatalf("invalid recovery file accepted: %d bytes", len(data))
		}
	}
}

func TestValidateWindowsUpdateRecovery(t *testing.T) {
	state, _, executable, tempDir := newWindowsRecoveryFixture(t)
	if err := validateWindowsUpdateRecovery(state, executable, tempDir); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*windowsUpdateRecovery){
		"version":       func(s *windowsUpdateRecovery) { s.Version = "latest" },
		"phase":         func(s *windowsUpdateRecovery) { s.Phase = "unknown" },
		"installation":  func(s *windowsUpdateRecovery) { s.InstallDir = filepath.Join(filepath.Dir(s.InstallDir), "other") },
		"executable":    func(s *windowsUpdateRecovery) { s.Executable = filepath.Join(s.InstallDir, "other.exe") },
		"backup parent": func(s *windowsUpdateRecovery) { s.BackupDir = filepath.Join(tempDir, filepath.Base(s.BackupDir)) },
		"backup prefix": func(s *windowsUpdateRecovery) { s.BackupDir = filepath.Join(filepath.Dir(s.BackupDir), "backup") },
		"work parent": func(s *windowsUpdateRecovery) {
			s.WorkDir = filepath.Join(filepath.Dir(tempDir), filepath.Base(s.WorkDir))
		},
		"work prefix":   func(s *windowsUpdateRecovery) { s.WorkDir = filepath.Join(tempDir, "update") },
		"ready file":    func(s *windowsUpdateRecovery) { s.ReadyFile = filepath.Join(s.WorkDir, "other") },
		"relative path": func(s *windowsUpdateRecovery) { s.WorkDir = filepath.Base(s.WorkDir) },
		"root path": func(s *windowsUpdateRecovery) {
			s.BackupDir = filepath.VolumeName(s.BackupDir) + string(filepath.Separator)
		},
		"path overlap":   func(s *windowsUpdateRecovery) { s.WorkDir = filepath.Join(s.InstallDir, "peapod-update-test") },
		"non-clean path": func(s *windowsUpdateRecovery) { s.ReadyFile += string(filepath.Separator) + "." },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			got := state
			mutate(&got)
			if err := validateWindowsUpdateRecovery(got, executable, tempDir); err == nil {
				t.Fatal("invalid recovery state accepted")
			}
		})
	}
}

func TestWindowsUpdateRecoveryRejectsOtherExecutable(t *testing.T) {
	state, _, executable, tempDir := newWindowsRecoveryFixture(t)
	other := filepath.Join(state.InstallDir, "other.exe")
	if err := os.WriteFile(other, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	state.Executable = other
	if err := validateWindowsUpdateRecovery(state, executable, tempDir); err == nil {
		t.Fatal("recovery state for another executable accepted")
	}
}

func TestWindowsUpdateRecoveryRejectsRegularFileAsBackup(t *testing.T) {
	state, _, executable, tempDir := newWindowsRecoveryFixture(t)
	if err := os.RemoveAll(state.BackupDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.BackupDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateWindowsUpdateRecovery(state, executable, tempDir); err == nil {
		t.Fatal("regular file accepted as backup directory")
	}
}

func TestWindowsUpdateRecoveryRejectsSymlinkEscape(t *testing.T) {
	state, _, executable, tempDir := newWindowsRecoveryFixture(t)
	outside := t.TempDir()
	if err := os.RemoveAll(state.BackupDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, state.BackupDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := validateWindowsUpdateRecovery(state, executable, tempDir); err == nil {
		t.Fatal("symlinked backup accepted")
	}
}

func TestRecoverWindowsUpdateCleansIdempotently(t *testing.T) {
	state, path, executable, tempDir := newWindowsRecoveryFixture(t)
	if err := writeWindowsUpdateRecovery(path, state); err != nil {
		t.Fatal(err)
	}
	result := recoverWindowsUpdate(path, executable, tempDir)
	if result.Err != nil || !result.Cleaned {
		t.Fatalf("result = %+v", result)
	}
	for _, item := range []string{path, state.BackupDir, state.WorkDir} {
		if _, err := os.Stat(item); !os.IsNotExist(err) {
			t.Fatalf("%s still exists, err = %v", item, err)
		}
	}
	if result := recoverWindowsUpdate(path, executable, tempDir); result.Err != nil || result.Cleaned {
		t.Fatalf("second recovery result = %+v", result)
	}
}

func TestRecoverWindowsUpdateHandlesPhases(t *testing.T) {
	cases := []struct {
		name        string
		phase       string
		ready       string
		keepBackup  bool
		emptyBackup bool
		want        string
	}{
		{name: "restart failed", phase: windowsRecoveryInstalled, ready: windowsHelperComplete, want: "cleaned"},
		{name: "complete before state flush", phase: windowsRecoveryInstalling, ready: windowsHelperComplete, want: "cleaned"},
		{name: "installer failed and rolled back", phase: windowsRecoveryInstalling, ready: windowsHelperFailed, want: "cleaned"},
		{name: "installer failed before backup", phase: windowsRecoveryInstalling, ready: windowsHelperFailed, emptyBackup: true, want: "cleaned"},
		{name: "installer failed with backup", phase: windowsRecoveryInstalling, ready: windowsHelperFailed, keepBackup: true, want: "pending"},
		{name: "helper still running", phase: windowsRecoveryInstalling, ready: windowsHelperReady, want: "pending"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, path, executable, tempDir := newWindowsRecoveryFixture(t)
			state.Phase = tc.phase
			if tc.name == "restart failed" {
				state.LastError = "restart failed"
			}
			if err := os.WriteFile(state.ReadyFile, []byte(tc.ready), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.emptyBackup {
				if err := os.RemoveAll(filepath.Join(state.BackupDir, "previous")); err != nil {
					t.Fatal(err)
				}
			} else if !tc.keepBackup {
				if err := os.RemoveAll(state.BackupDir); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeWindowsUpdateRecovery(path, state); err != nil {
				t.Fatal(err)
			}
			result := recoverWindowsUpdate(path, executable, tempDir)
			if result.Err != nil {
				t.Fatal(result.Err)
			}
			if tc.want == "cleaned" && (!result.Cleaned || result.Pending) {
				t.Fatalf("result = %+v", result)
			}
			if tc.want == "pending" && (!result.Pending || result.Cleaned || result.RetainedPath == "") {
				t.Fatalf("result = %+v", result)
			}
			if tc.name == "restart failed" && result.LastError != state.LastError {
				t.Fatalf("restart error = %q", result.LastError)
			}
		})
	}
}

func TestRecoverWindowsUpdateRetainsStateOnCleanupFailure(t *testing.T) {
	state, path, executable, tempDir := newWindowsRecoveryFixture(t)
	if err := writeWindowsUpdateRecovery(path, state); err != nil {
		t.Fatal(err)
	}
	original := removeWindowsRecoveryDirectory
	removeWindowsRecoveryDirectory = func(path string) error {
		if path == state.WorkDir {
			return errors.New("directory locked")
		}
		return original(path)
	}
	defer func() { removeWindowsRecoveryDirectory = original }()
	result := recoverWindowsUpdate(path, executable, tempDir)
	if result.Err == nil || result.Cleaned || result.RetainedPath != state.WorkDir {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("recovery state removed after failure: %v", err)
	}
	if _, err := os.Stat(state.WorkDir); err != nil {
		t.Fatalf("locked work directory removed: %v", err)
	}
	if _, err := os.Stat(state.BackupDir); err != nil {
		t.Fatalf("backup removed before temporary cleanup succeeded: %v", err)
	}
	removeWindowsRecoveryDirectory = original
	if result := recoverWindowsUpdate(path, executable, tempDir); result.Err != nil || !result.Cleaned {
		t.Fatalf("retry result = %+v", result)
	}
}

func TestRecordWindowsUpdateRestartFailure(t *testing.T) {
	state, path, _, _ := newWindowsRecoveryFixture(t)
	config := windowsUpdateHelperConfig{
		Version: state.Version, InstallDir: state.InstallDir, Executable: state.Executable,
		BackupDir: state.BackupDir, WorkDir: state.WorkDir, ReadyFile: state.ReadyFile, RecoveryPath: path,
	}
	if err := createWindowsUpdateRecovery(path, recoveryStateFromConfig(config, windowsRecoveryInstalling, "")); err != nil {
		t.Fatal(err)
	}
	restartErr := errors.New("restart failed")
	if err := recordWindowsUpdateRestartFailure(config, restartErr); err != nil {
		t.Fatal(err)
	}
	got, err := readWindowsUpdateRecovery(path)
	if err != nil || got.Phase != windowsRecoveryInstalled || got.LastError != restartErr.Error() {
		t.Fatalf("recovery state = %+v, err = %v", got, err)
	}
}

func TestSelectUpdateAssetsCarriesVersion(t *testing.T) {
	info := &releaseInfo{Version: "0.14.0", Assets: []releaseAsset{
		{Name: "peapod-setup-0.14.0-amd64.exe", URL: "https://github.com/example/setup.exe", Size: 1},
		{Name: "SHA256SUMS", URL: "https://github.com/example/SHA256SUMS", Size: 1},
	}}
	selected, err := selectUpdateAssets(info, "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Version != info.Version {
		t.Fatalf("selected version = %q, want %q", selected.Version, info.Version)
	}
}

func newWindowsRecoveryFixture(t *testing.T) (windowsUpdateRecovery, string, string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	installDir := filepath.Join(root, "Peapod App")
	executable := filepath.Join(installDir, "peapod.exe")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	tempDir := filepath.Join(root, "temp")
	backupDir := filepath.Join(root, "."+filepath.Base(installDir)+".backup-test")
	workDir := filepath.Join(tempDir, "peapod-update-test")
	for _, dir := range []string{tempDir, backupDir, workDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	previousDir := filepath.Join(backupDir, "previous")
	if err := os.MkdirAll(previousDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(previousDir, "peapod.exe"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "helper.exe"), []byte("helper"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := windowsUpdateRecovery{
		Version:    "0.14.0",
		Phase:      windowsRecoveryInstalled,
		InstallDir: installDir,
		Executable: executable,
		BackupDir:  backupDir,
		WorkDir:    workDir,
		ReadyFile:  filepath.Join(workDir, "ready"),
	}
	return state, filepath.Join(root, "config", "update-recovery.json"), executable, tempDir
}
