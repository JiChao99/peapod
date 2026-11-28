package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflowDefinition struct {
	On struct {
		Push struct {
			Branches []string `yaml:"branches"`
		} `yaml:"push"`
		PullRequest struct {
			Paths []string `yaml:"paths"`
		} `yaml:"pull_request"`
		WorkflowDispatch yaml.Node `yaml:"workflow_dispatch"`
	} `yaml:"on"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	RunsOn   string `yaml:"runs-on"`
	Strategy struct {
		Matrix struct {
			OS      []string                  `yaml:"os"`
			Include []workflowMatrixDimension `yaml:"include"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []workflowStep `yaml:"steps"`
}

type workflowMatrixDimension struct {
	Name string `yaml:"name"`
	OS   string `yaml:"os"`
}

type workflowStep struct {
	Name string         `yaml:"name"`
	If   string         `yaml:"if"`
	Run  string         `yaml:"run"`
	Uses string         `yaml:"uses"`
	With map[string]any `yaml:"with"`
}

func TestMacOSWorkflowContract(t *testing.T) {
	ci := readWorkflowForTest(t, "ci.yml")
	if !containsString(ci.Jobs["build"].Strategy.Matrix.OS, "macos-latest") {
		t.Error("ci.yml must test on macos-latest")
	}
	for _, command := range []string{"go build ./...", "go vet ./...", "go test ./..."} {
		if !jobHasRun(ci.Jobs["build"], command) {
			t.Errorf("ci.yml macOS job missing %q", command)
		}
	}

	macos := readWorkflowForTest(t, "build-macos.yml")
	if macos.Jobs["build"].RunsOn != "macos-latest" {
		t.Error("build-macos.yml must run on macos-latest")
	}
	if !containsString(macos.On.Push.Branches, "main") || !containsString(macos.On.PullRequest.Paths, "**/*.go") {
		t.Error("build-macos.yml must validate main and Go-source pull requests")
	}
	if macos.On.WorkflowDispatch.Kind == 0 {
		t.Error("build-macos.yml must support workflow_dispatch")
	}
	for _, command := range []string{
		"go tool mygo build -platform darwin/universal",
		"bash scripts/verify-macos-package.sh build/darwin-universal",
	} {
		if !jobHasRun(macos.Jobs["build"], command) {
			t.Errorf("build-macos.yml missing %q", command)
		}
	}
	collect := findStep(macos.Jobs["build"], "Collect macOS packages")
	if !strings.Contains(collect.Run, `tar -czf "dist/peapod-${version}-macos-universal-app.tar.gz"`) || !strings.Contains(collect.Run, "cp build/darwin-universal/*.dmg dist/") {
		t.Errorf("build-macos.yml must collect an app archive and DMG in dist, got %q", collect.Run)
	}
	upload := findStep(macos.Jobs["build"], "Upload macOS packages")
	if upload.Uses != "actions/upload-artifact@v4" {
		t.Errorf("build-macos.yml upload step uses %q", upload.Uses)
	}
	if uploadPath := stringValue(upload.With["path"]); uploadPath != "dist/*" {
		t.Errorf("build-macos.yml must upload the collected dist directory, got %q", uploadPath)
	}

	release := readWorkflowForTest(t, "release.yml")
	var macosDimension bool
	for _, dimension := range release.Jobs["build"].Strategy.Matrix.Include {
		if dimension.Name == "macos-universal" && dimension.OS == "macos-latest" {
			macosDimension = true
		}
	}
	if !macosDimension {
		t.Error("release.yml must build macOS Universal on macos-latest")
	}
	for _, command := range []string{
		"go tool mygo build -platform darwin/universal",
		"bash scripts/verify-macos-package.sh build/darwin-universal",
		"cp build/darwin-universal/*.dmg dist/",
	} {
		if !jobHasRun(release.Jobs["build"], command) {
			t.Errorf("release.yml missing %q", command)
		}
	}
	if !jobHasRun(release.Jobs["release"], "sha256sum --binary -- * > SHA256SUMS") {
		t.Error("release.yml must generate SHA256SUMS")
	}
}

func readWorkflowForTest(t *testing.T, name string) workflowDefinition {
	t.Helper()
	data, err := os.ReadFile(".github/workflows/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowDefinition
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func jobHasRun(job workflowJob, want string) bool {
	for _, step := range job.Steps {
		if strings.Contains(step.Run, want) {
			return true
		}
	}
	return false
}

func findStep(job workflowJob, name string) workflowStep {
	for _, step := range job.Steps {
		if step.Name == name {
			return step
		}
	}
	return workflowStep{}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
