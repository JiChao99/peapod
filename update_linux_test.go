//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExchangeLinuxDirectoriesAtomicallySwaps(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "peapod.app")
	staging := filepath.Join(parent, "staging")
	for path, content := range map[string]string{target: "old", staging: "new"} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "version.txt"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	exchanged, err := exchangeLinuxDirectories(target, staging)
	if err != nil || !exchanged {
		t.Fatalf("exchanged = %v, err = %v", exchanged, err)
	}
	for path, want := range map[string]string{target: "new", staging: "old"} {
		data, err := os.ReadFile(filepath.Join(path, "version.txt"))
		if err != nil || string(data) != want {
			t.Fatalf("%s = %q, err = %v, want %q", path, data, err, want)
		}
	}
}
