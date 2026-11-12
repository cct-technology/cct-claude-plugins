package qualitykit

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// qualityWorkflow is the part of .github/workflows/quality.yml these tests read.
type qualityWorkflow struct {
	On struct {
		WorkflowCall struct {
			Inputs map[string]struct{ Default string } `yaml:"inputs"`
		} `yaml:"workflow_call"`
	} `yaml:"on"`
	Jobs struct {
		Check struct{ Steps []step } `yaml:"check"`
	} `yaml:"jobs"`
}

type step struct {
	Name string
	If   string            `yaml:"if"`
	Env  map[string]string `yaml:"env"`
	Run  string
}

func readQualityWorkflow(t *testing.T) qualityWorkflow {
	t.Helper()
	raw, err := os.ReadFile(".github/workflows/quality.yml")
	if err != nil {
		t.Fatal(err)
	}
	var wf qualityWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatal(err)
	}
	return wf
}

// The reusable CI workflow installs the qk release the plugin pins, so a
// consumer that pins the workflow to a tag runs the same qk as its hooks.
func TestCIWorkflowInstallsThePluginVersion(t *testing.T) {
	raw, err := os.ReadFile("plugins/quality-kit/.claude-plugin/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var plugin struct{ Version string }
	if err := json.Unmarshal(raw, &plugin); err != nil {
		t.Fatal(err)
	}

	if got := readQualityWorkflow(t).On.WorkflowCall.Inputs["qk-version"].Default; got != plugin.Version {
		t.Errorf("quality.yml installs qk %q by default, plugin.json pins %q: change them together", got, plugin.Version)
	}
}

const setupStepName = "Set up what the checks need"

// setupStep returns the step that runs the `setup` input.
func setupStep(t *testing.T) step {
	t.Helper()
	for _, s := range readQualityWorkflow(t).Jobs.Check.Steps {
		if s.Name == setupStepName {
			return s
		}
	}
	t.Fatalf("quality.yml has no step named %q", setupStepName)
	return step{}
}

// The caller's commands reach the step as an environment variable. Written
// into `run:` they would become part of the workflow's own script.
func TestSetupInputRunsBeforeQkCheckAndOnlyThroughEnv(t *testing.T) {
	wf := readQualityWorkflow(t)
	if _, ok := wf.On.WorkflowCall.Inputs["setup"]; !ok {
		t.Fatal("quality.yml has no `setup` input")
	}
	at := map[string]int{}
	for i, s := range wf.Jobs.Check.Steps {
		at[s.Name] = i + 1
	}
	if at[setupStepName] == 0 || at["qk check"] == 0 || at[setupStepName] > at["qk check"] {
		t.Errorf("steps at %v, want %q before \"qk check\"", at, setupStepName)
	}

	s := setupStep(t)
	if s.If != "inputs.setup != ''" {
		t.Errorf("if = %q, want the step left out without the input", s.If)
	}
	if s.Env["SETUP"] != "${{ inputs.setup }}" {
		t.Errorf("env SETUP = %q, want the input", s.Env["SETUP"])
	}
	if strings.Contains(s.Run, "${{") {
		t.Errorf("run uses a workflow expression, so the input could become part of the script:\n%s", s.Run)
	}
}

func needLinuxShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("quality.yml runs on Linux; on Windows `bash` may be the WSL launcher")
	}
}

// runSetup runs the step as GitHub does (`bash -e`) in dir, and returns
// RUNNER_TEMP and the directories the step added to GITHUB_PATH.
func runSetup(t *testing.T, dir, setup string) (runnerTemp string, path []string, err error) {
	t.Helper()
	needLinuxShell(t)
	runnerTemp = t.TempDir()
	githubPath := filepath.Join(runnerTemp, "github-path")
	cmd := exec.Command("bash", "-e", "-c", setupStep(t).Run)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "SETUP="+setup, "RUNNER_TEMP="+runnerTemp, "GITHUB_PATH="+githubPath)
	out, err := cmd.CombinedOutput()
	t.Logf("setup step:\n%s", out)
	added, _ := os.ReadFile(githubPath)
	return runnerTemp, strings.Fields(string(added)), err
}

// A generator whose tool is not on the runner is skipped by QK020. With the
// tool installed by `setup`, the same generator runs and QK020 passes.
func TestSetupGivesAGeneratorItsTool(t *testing.T) {
	needLinuxShell(t)
	qk := filepath.Join(t.TempDir(), "qk")
	if out, err := exec.Command("go", "build", "-o", qk, "./cmd/qk").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	repo := t.TempDir()
	for name, content := range map[string]string{
		"quality.yaml": "version: 1\nmaturity: silver\ngenerated:\n  - file: out.txt\n    cmd: gen-thing\n",
		"out.txt":      "generated\n",
	} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitconfig := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(gitconfig, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	check := func() string {
		cmd := exec.Command(qk, "check", "--full", "--format", "text")
		cmd.Dir = repo
		out, _ := cmd.CombinedOutput()
		return string(out)
	}

	if out := check(); !strings.Contains(out, "SKIP  QK020") || !strings.Contains(out, "`gen-thing` is not installed") {
		t.Fatalf("before setup, want QK020 skipped for the missing tool:\n%s", out)
	}

	_, path, err := runSetup(t, repo, `mkdir -p "$RUNNER_TEMP/bin"
printf '#!/bin/sh\necho generated > out.txt\n' > "$RUNNER_TEMP/bin/gen-thing"
chmod +x "$RUNNER_TEMP/bin/gen-thing"
echo "$RUNNER_TEMP/bin" >> "$GITHUB_PATH"`)
	if err != nil {
		t.Fatalf("setup step: %v", err)
	}
	if len(path) != 1 {
		t.Fatalf("setup added %v to GITHUB_PATH, want one directory", path)
	}
	t.Setenv("PATH", path[0]+string(os.PathListSeparator)+os.Getenv("PATH")) // what GitHub does for the next step

	if out := check(); !strings.Contains(out, "PASS  QK020") {
		t.Errorf("after setup, want QK020 to pass:\n%s", out)
	}
}

// A failing command stops the setup and fails the job there, and the pull
// request comment says why qk did not run.
func TestSetupThatFailsStopsAndSaysQkDidNotRun(t *testing.T) {
	dir := t.TempDir()

	runnerTemp, _, err := runSetup(t, dir, "false\ntouch later")

	if err == nil {
		t.Error("the step passed, want it to fail")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "later")); statErr == nil {
		t.Error("the commands after the failing one ran")
	}
	summary, _ := os.ReadFile(filepath.Join(runnerTemp, "qk-summary.txt"))
	if !strings.Contains(string(summary), "qk did not run") || !strings.Contains(string(summary), "setup") {
		t.Errorf("summary = %q, want it to say qk did not run because the setup failed", summary)
	}
}
