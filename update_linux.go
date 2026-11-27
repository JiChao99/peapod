//go:build linux

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func exchangeLinuxDirectories(target, staging string) (bool, error) {
	if _, err := os.Lstat(target); errors.Is(err, fs.ErrNotExist) {
		if err := os.Rename(staging, target); err != nil {
			return false, err
		}
		return true, nil
	} else if err != nil {
		return false, err
	}
	if err := unix.Renameat2(unix.AT_FDCWD, target, unix.AT_FDCWD, staging, unix.RENAME_EXCHANGE); err != nil {
		if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
			return false, fmt.Errorf("Linux updates require a filesystem with atomic directory exchange: %w", err)
		}
		return false, err
	}
	return true, nil
}
