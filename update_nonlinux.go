//go:build !linux

package main

func exchangeLinuxDirectories(string, string) (bool, error) {
	return false, nil
}
