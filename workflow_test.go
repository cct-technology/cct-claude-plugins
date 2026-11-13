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

const (
	setupStepName = "Set up what the checks need"
	checkStepName = "qk check"
)

func stepNamed(t *testing.T, name string) step {
	t.Helper()
	for _, s := range readQualityWorkflow(t).Jobs.Check.Steps {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("quality.yml has no step named %q", name)
	return step{}
}

// setupStep returns the step that runs the `setup` input.
func setupStep(t *testing.T) step {
	t.Helper()
	return stepNamed(t, setupStepName)
}

// The job result, the SARIF and the pull request comment must come from one
// run. A second run repeats every generator, and sees what the first left behind.
func TestQualityWorkflowRunsQkCheckOnce(t *testing.T) {
	if n := strings.Count(stepNamed(t, checkStepName).Run, "qk check --"); n != 1 {
		t.Errorf("the %q step calls qk check %d times, want once", checkStepName, n)
	}
}

// A generator that writes a file not listed under `generated` leaves it
// changed, with a QK020 warning. The comment must show that warning, and must
// not show a QK010 finding on the leftover file that the job never failed on.
func TestQkCheckStepReportsWhatTheJobDecidedOn(t *testing.T) {
	needLinuxShell(t)
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "qk"), "./cmd/qk").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	generator := "#!/bin/sh\necho generated > out.txt\nmkdir -p src\necho 'package src' > src/extra.go\necho run >> \"$GEN_RUNS\"\n"
	if err := os.WriteFile(filepath.Join(bin, "gen-thing"), []byte(generator), 0o755); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	for name, content := range map[string]string{
		"quality.yaml": "version: 1\nmaturity: silver\n" +
			"docs:\n  impact_map:\n    - paths: [\"src/**\"]\n      docs: [\"docs/**\"]\n" +
			"generated:\n  - file: out.txt\n    cmd: gen-thing\n",
		"out.txt": "generated\n",
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
		{"update-ref", "refs/remotes/origin/main", "HEAD"}, // what the workflow's checkout gives
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runnerTemp := t.TempDir()
	genRuns := filepath.Join(runnerTemp, "gen-runs")
	output := filepath.Join(runnerTemp, "github-output")

	cmd := exec.Command("bash", "-e", "-c", stepNamed(t, checkStepName).Run)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"BASE=main", "PROBLEM=", "CI=true", "GEN_RUNS="+genRuns,
		"RUNNER_TEMP="+runnerTemp, "GITHUB_OUTPUT="+output, "GITHUB_STEP_SUMMARY="+filepath.Join(runnerTemp, "step-summary"))
	out, err := cmd.CombinedOutput()
	t.Logf("qk check step:\n%s", out)
	if err != nil {
		t.Fatalf("qk check step: %v", err)
	}

	if runs, _ := os.ReadFile(genRuns); strings.Count(string(runs), "run") != 1 {
		t.Errorf("the generator ran %d times, want once", strings.Count(string(runs), "run"))
	}
	summary, _ := os.ReadFile(filepath.Join(runnerTemp, "qk-summary.txt"))
	if !strings.Contains(string(summary), "QK020 warn") || !strings.Contains(string(summary), "src/extra.go") {
		t.Errorf("summary lacks the QK020 warning about src/extra.go:\n%s", summary)
	}
	if strings.Contains(string(summary), "QK010") {
		t.Errorf("summary has a QK010 finding the job did not decide on:\n%s", summary)
	}
	if outputs, _ := os.ReadFile(output); !strings.Contains(string(outputs), "exit-code=0") || !strings.Contains(string(outputs), "sarif=true") {
		t.Errorf("step outputs = %q, want exit-code=0 and sarif=true", outputs)
	}
	var sarif struct {
		Runs []struct{ Results []struct{ RuleID string } }
	}
	raw, _ := os.ReadFile(filepath.Join(runnerTemp, "qk.sarif"))
	if err := json.Unmarshal(raw, &sarif); err != nil || len(sarif.Runs) != 1 || len(sarif.Runs[0].Results) != 1 || sarif.Runs[0].Results[0].RuleID != "QK020" {
		t.Errorf("qk.sarif = %s (%v), want one result, for QK020", raw, err)
	}
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
