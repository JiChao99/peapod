package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestAppVersionMatchesPackageVersion(t *testing.T) {
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	if appVersion != pkg.Version {
		t.Fatalf("appVersion = %q, mygo.json version = %q", appVersion, pkg.Version)
	}
}
