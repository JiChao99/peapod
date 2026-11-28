//go:build !windows

package main

import "os"

func replaceWindowsUpdateRecoveryFile(source, destination string) error {
	return os.Rename(source, destination)
}
