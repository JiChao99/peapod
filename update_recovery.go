package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	windowsRecoveryInstalling = "installing"
	windowsRecoveryInstalled  = "installed"
	windowsRecoveryMaxBytes   = 64 << 10
)

var removeWindowsRecoveryDirectory = os.RemoveAll

type windowsUpdateRecovery struct {
	Version    string    `json:"version"`
	Phase      string    `json:"phase"`
	InstallDir string    `json:"installDir"`
	Executable string    `json:"executable"`
	BackupDir  string    `json:"backupDir"`
	WorkDir    string    `json:"workDir"`
	ReadyFile  string    `json:"readyFile"`
	LastError  string    `json:"lastError,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type windowsUpdateRecoveryResult struct {
	Version      string
	LastError    string
	Cleaned      bool
	Pending      bool
	RetainedPath string
	Err          error
}

func windowsUpdateRecoveryPath() string {
	return filepath.Join(filepath.Dir(settingsPath()), "update-recovery.json")
}

func recoveryStateFromConfig(config windowsUpdateHelperConfig, phase, lastError string) windowsUpdateRecovery {
	return windowsUpdateRecovery{
		Version:    config.Version,
		Phase:      phase,
		InstallDir: config.InstallDir,
		Executable: config.Executable,
		BackupDir:  config.BackupDir,
		WorkDir:    config.WorkDir,
		ReadyFile:  config.ReadyFile,
		LastError:  lastError,
		UpdatedAt:  time.Now().UTC(),
	}
}

func writeWindowsUpdateRecovery(path string, state windowsUpdateRecovery) error {
	return writeWindowsUpdateRecoveryFile(path, state, false)
}

func createWindowsUpdateRecovery(path string, state windowsUpdateRecovery) error {
	return writeWindowsUpdateRecoveryFile(path, state, true)
}

func writeWindowsUpdateRecoveryFile(path string, state windowsUpdateRecovery, createOnly bool) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "update-recovery-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if createOnly {
		return os.Link(tmp, path)
	}
	return replaceWindowsUpdateRecoveryFile(tmp, path)
}

func readWindowsUpdateRecovery(path string) (windowsUpdateRecovery, error) {
	var state windowsUpdateRecovery
	data, err := readBoundedFile(path, windowsRecoveryMaxBytes)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func validateWindowsUpdateRecovery(state windowsUpdateRecovery, executable, tempDir string) error {
	if state.Phase != windowsRecoveryInstalling && state.Phase != windowsRecoveryInstalled {
		return fmt.Errorf("invalid update recovery phase %q", state.Phase)
	}
	if !semver.IsValid("v" + state.Version) {
		return fmt.Errorf("invalid update recovery version %q", state.Version)
	}
	paths := []string{state.InstallDir, state.Executable, state.BackupDir, state.WorkDir, state.ReadyFile}
	for _, item := range paths {
		if item == "" || !filepath.IsAbs(item) || filepath.Clean(item) != item || filepath.Dir(item) == item {
			return fmt.Errorf("invalid update recovery path %q", item)
		}
	}
	realExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	realExecutable, err = filepath.Abs(realExecutable)
	if err != nil {
		return err
	}
	realInstallDir := filepath.Dir(realExecutable)
	if !samePath(realInstallDir, state.InstallDir) {
		return errors.New("update recovery does not belong to the current installation")
	}
	realRecoveryExecutable, err := filepath.EvalSymlinks(state.Executable)
	if err != nil {
		return fmt.Errorf("resolve recovery executable: %w", err)
	}
	if !samePath(realExecutable, realRecoveryExecutable) {
		return errors.New("update recovery does not belong to the current executable")
	}
	if !samePath(state.Executable, filepath.Join(state.InstallDir, "peapod.exe")) {
		return errors.New("update recovery executable does not match the installation")
	}
	backupPrefix := "." + filepath.Base(state.InstallDir) + ".backup-"
	if err := validateWindowsRecoveryDirectory(state.BackupDir, filepath.Dir(state.InstallDir), backupPrefix); err != nil {
		return fmt.Errorf("invalid update backup: %w", err)
	}
	if err := validateWindowsRecoveryDirectory(state.WorkDir, tempDir, "peapod-update-"); err != nil {
		return fmt.Errorf("invalid update working directory: %w", err)
	}
	if !samePath(state.ReadyFile, filepath.Join(state.WorkDir, "ready")) {
		return errors.New("update recovery ready file does not match its working directory")
	}
	if pathIsWithin(state.InstallDir, state.BackupDir) || pathIsWithin(state.BackupDir, state.InstallDir) ||
		pathIsWithin(state.InstallDir, state.WorkDir) || pathIsWithin(state.WorkDir, state.InstallDir) ||
		pathIsWithin(state.BackupDir, state.WorkDir) || pathIsWithin(state.WorkDir, state.BackupDir) {
		return errors.New("update recovery paths overlap")
	}
	return nil
}

func validateWindowsRecoveryDirectory(path, parent, prefix string) error {
	if !samePath(filepath.Dir(path), parent) || !strings.HasPrefix(filepath.Base(path), prefix) {
		return errors.New("directory is outside its expected parent or prefix")
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("recovery path is not a real directory")
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if !samePath(filepath.Dir(realPath), realParent) || !strings.HasPrefix(filepath.Base(realPath), prefix) {
		return errors.New("directory resolves outside its expected parent or prefix")
	}
	return nil
}

func (a *app) recoverWindowsUpdate() {
	if runtime.GOOS != "windows" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		a.setNoticeErr(a.tr("err.updateRecovery") + ": " + err.Error())
		return
	}
	result := recoverWindowsUpdate(windowsUpdateRecoveryPath(), executable, os.TempDir())
	if result.Err != nil {
		message := a.tr("err.updateRecovery") + ": " + result.Err.Error()
		if result.RetainedPath != "" {
			a.setNoticeOpenKey("warning", "notice.updateRecoveryFailed", result.RetainedPath, true, result.Version, message)
		} else {
			a.setNoticeErr(message)
		}
		return
	}
	if result.LastError != "" {
		a.setNoticeWarnKey("notice.updateRestartFailed", result.Version)
		return
	}
	if result.Pending {
		a.setNoticeOpenKey("warning", "notice.updateRecoveryPending", result.RetainedPath, true, result.Version)
		return
	}
	if result.Cleaned {
		a.setNoticeOKKey("notice.updateRecovered", result.Version)
	}
}

func recordWindowsUpdateRestartFailure(config windowsUpdateHelperConfig, restartErr error) error {
	return writeWindowsUpdateRecovery(config.RecoveryPath, recoveryStateFromConfig(config, windowsRecoveryInstalled, restartErr.Error()))
}

func windowsRecoveryBackupExists(state windowsUpdateRecovery) (bool, error) {
	info, err := os.Stat(filepath.Join(state.BackupDir, "previous"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, errors.New("Windows installation backup is not a directory")
	}
	return true, nil
}

func recoverWindowsUpdate(path, executable, tempDir string) windowsUpdateRecoveryResult {
	state, err := readWindowsUpdateRecovery(path)
	if err != nil {
		if os.IsNotExist(err) {
			return windowsUpdateRecoveryResult{}
		}
		return windowsUpdateRecoveryResult{Err: err}
	}
	result := windowsUpdateRecoveryResult{Version: state.Version, LastError: state.LastError}
	if err := validateWindowsUpdateRecovery(state, executable, tempDir); err != nil {
		result.Err = err
		return result
	}
	if state.Phase == windowsRecoveryInstalling {
		ready, readErr := os.ReadFile(state.ReadyFile)
		if readErr == nil {
			switch strings.TrimSpace(string(ready)) {
			case windowsHelperComplete:
				state.Phase = windowsRecoveryInstalled
			case windowsHelperFailed:
				backupExists, err := windowsRecoveryBackupExists(state)
				if err != nil {
					result.Err = err
					return result
				}
				if backupExists {
					result.Pending = true
					result.RetainedPath = state.BackupDir
					return result
				}
			default:
				result.Pending = true
				result.RetainedPath = state.WorkDir
				return result
			}
		} else if !os.IsNotExist(readErr) {
			result.Err = readErr
			return result
		} else {
			result.Pending = true
			result.RetainedPath = state.BackupDir
			return result
		}
	}
	for _, dir := range []string{state.WorkDir, state.BackupDir} {
		if err := removeWindowsRecoveryDirectory(dir); err != nil {
			result.Err = fmt.Errorf("clean update recovery directory %s: %w", dir, err)
			result.RetainedPath = dir
			return result
		}
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		result.Err = fmt.Errorf("remove update recovery state: %w", err)
		return result
	}
	result.Cleaned = true
	return result
}
