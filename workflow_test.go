package qualitykit

import (
	"encoding/json"
	"os"
	"testing"

	"go.yaml.in/yaml/v3"
)

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

	raw, err = os.ReadFile(".github/workflows/quality.yml")
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		On struct {
			WorkflowCall struct {
				Inputs map[string]struct{ Default string } `yaml:"inputs"`
			} `yaml:"workflow_call"`
		} `yaml:"on"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatal(err)
	}

	if got := wf.On.WorkflowCall.Inputs["qk-version"].Default; got != plugin.Version {
		t.Errorf("quality.yml installs qk %q by default, plugin.json pins %q: change them together", got, plugin.Version)
	}
}
