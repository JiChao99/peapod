//go:build windows

package main

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

func replaceWindowsUpdateRecoveryFile(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func executeWindowsInstallerCommand(ctx context.Context, installer, commandLine string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, installer)
	cmd.Dir = filepath.Dir(installer)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: commandLine}
	return cmd.CombinedOutput()
}

func terminateWindowsUpdateHelperTree(pid int) error {
	if pid <= 0 {
		return errors.New("invalid update helper process")
	}
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}
