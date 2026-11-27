//go:build !windows

package main

import (
	"context"
	"errors"
)

func executeWindowsInstallerCommand(context.Context, string, string) ([]byte, error) {
	return nil, errors.New("Windows installer command cannot run on this platform")
}

func terminateWindowsUpdateHelperTree(int) error {
	return errors.New("Windows update helper cannot run on this platform")
}
