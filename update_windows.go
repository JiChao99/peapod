//go:build windows

package main

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

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
