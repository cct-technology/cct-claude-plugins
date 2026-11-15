package rules

import (
	"errors"
	"strings"
	"testing"

	"github.com/cct-technology/cct-claude-plugins/internal/check"
	"github.com/cct-technology/cct-claude-plugins/internal/manifest"
)

const (
	oldSpec = "docs/specs/login.md"
	newSpec = "docs/specs/payments.md"
)

func runRegisteredQK040(t *testing.T, ctx check.Context) check.Result {
	t.Helper()
	for _, r := range All() {
		if r.ID == "QK040" {
			if r.Check != "security.threat-model" || r.EnforcedFrom != check.Gold {
				t.Errorf("QK040 = check %q from %v, want security.threat-model from gold", r.Check, r.EnforcedFrom)
			}
			return r.Run(ctx)
		}
	}
	t.Fatal("QK040 is not registered")
	return check.Result{}
}

// qk040Repo commits one spec with its threat model, and returns a context
// whose manifest names the two folders.
func qk040Repo(t *testing.T) check.Context {
	t.Helper()
	ctx := qk020Repo(t, map[string]string{
		oldSpec:                                "# Login\n",
		"docs/security/threat-models/login.md": "# Login threat model\n",
	})
	ctx.Manifest.Manifest.Security = manifest.Security{SpecsDir: "docs/specs", ThreatModels: "docs/security/threat-models"}
	return ctx
}

func TestQK040FailsOnANewSpecWithoutAThreatModel(t *testing.T) {
	ctx := qk040Repo(t)
	put(t, ctx.Root, newSpec, "# Payments\n")

	res := runRegisteredQK040(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 1 {
		t.Fatalf("result = %+v, want fail with one finding", res)
	}
	f := res.Findings[0]
	if f.File != newSpec || !strings.Contains(f.Message, "docs/security/threat-models/payments.md") || !strings.Contains(f.Message, "spec: "+newSpec) {
		t.Errorf("finding = %+v, want it on the spec, naming both ways to give it a threat model", f)
	}
}

func TestQK040PassesWithAThreatModelOfTheSameName(t *testing.T) {
	ctx := qk040Repo(t)
	put(t, ctx.Root, newSpec, "# Payments\n")
	put(t, ctx.Root, "docs/security/threat-models/payments.md", "# Payments threat model\n")

	if res := runRegisteredQK040(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK040PassesWithAThreatModelNamingTheSpecInItsFrontmatter(t *testing.T) {
	ctx := qk040Repo(t)
	put(t, ctx.Root, newSpec, "# Payments\n")
	put(t, ctx.Root, "docs/security/threat-models/2026/card-data.md", "---\r\nowner: rk\r\nspec: ./docs/specs/payments.md\r\n---\r\n# Card data\r\n")

	if res := runRegisteredQK040(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

// A threat model that names another spec, or names this one outside its
// frontmatter, does not belong to it.
func TestQK040FailsWhenNoThreatModelNamesTheSpec(t *testing.T) {
	ctx := qk040Repo(t)
	put(t, ctx.Root, newSpec, "# Payments\n")
	put(t, ctx.Root, "docs/security/threat-models/other.md", "---\nspec: docs/specs/login.md\n---\nspec: docs/specs/payments.md\n")
	put(t, ctx.Root, "docs/security/threat-models/broken.md", "---\nspec: [unclosed\n---\n")

	if res := runRegisteredQK040(t, changed(t, ctx)); res.Status != check.Fail || len(res.Findings) != 1 {
		t.Errorf("result = %+v, want fail with one finding", res)
	}
}

// Only new specs are checked. An old spec whose threat model is gone is QK041's.
func TestQK040IgnoresAnEditedSpec(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{oldSpec: "# Login\n"})
	ctx.Manifest.Manifest.Security = manifest.Security{SpecsDir: "docs/specs", ThreatModels: "docs/security/threat-models"}
	put(t, ctx.Root, oldSpec, "# Login\n\nNow with passkeys.\n")

	if res := runRegisteredQK040(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK040IgnoresNewFilesThatAreNotSpecs(t *testing.T) {
	ctx := qk040Repo(t)
	put(t, ctx.Root, "docs/specs/payments-flow.png", "not really a picture\n")
	put(t, ctx.Root, "docs/how-to/pay.md", "# Pay\n")

	if res := runRegisteredQK040(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

// With the threat models kept inside the specs folder, a new threat model is
// not itself a spec that needs one.
func TestQK040DoesNotTakeAThreatModelForASpec(t *testing.T) {
	ctx := qk040Repo(t)
	ctx.Manifest.Manifest.Security = manifest.Security{SpecsDir: "docs", ThreatModels: "docs/security/threat-models"}
	put(t, ctx.Root, "docs/security/threat-models/payments.md", "# Payments threat model\n")

	if res := runRegisteredQK040(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK040ThreatModelsDefaultToTheUsualFolder(t *testing.T) {
	ctx := qk040Repo(t)
	ctx.Manifest.Manifest.Security = manifest.Security{SpecsDir: "docs/specs/"}
	put(t, ctx.Root, newSpec, "# Payments\n")
	put(t, ctx.Root, "docs/specs/refunds.md", "# Refunds\n")
	put(t, ctx.Root, "docs/security/threat-models/payments.md", "# Payments threat model\n")

	res := runRegisteredQK040(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != "docs/specs/refunds.md" {
		t.Errorf("result = %+v, want fail with one finding on docs/specs/refunds.md", res)
	}
}

// A manifest says what the repository owes. One that names no specs folder owes no threat models.
func TestQK040PassesWhenTheManifestNamesNoSpecsFolder(t *testing.T) {
	ctx := qk040Repo(t)
	ctx.Manifest.Manifest.Security = manifest.Security{}
	put(t, ctx.Root, newSpec, "# Payments\n")

	if res := runRegisteredQK040(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK040ChecksDocsSpecsWithoutAManifest(t *testing.T) {
	ctx := qk040Repo(t)
	ctx.Manifest = manifest.Loaded{}
	put(t, ctx.Root, newSpec, "# Payments\n")

	res := runRegisteredQK040(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != newSpec {
		t.Errorf("result = %+v, want fail with one finding on %s", res, newSpec)
	}
}

func TestQK040PassesWithoutAManifestOrASpecsFolder(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"README.md": "x\n"})
	ctx.Manifest = manifest.Loaded{}
	put(t, ctx.Root, "specs/payments.md", "# Payments\n")

	if res := runRegisteredQK040(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK040SkipsWithoutAChangeSet(t *testing.T) {
	ctx := check.Context{
		Manifest:   manifest.Loaded{Present: true, Manifest: manifest.Manifest{Version: 1, Security: manifest.Security{SpecsDir: "docs/specs"}}},
		ChangesErr: errors.New("not a git repository"),
	}

	if res := runRegisteredQK040(t, ctx); res.Status != check.Skipped || res.Reason != "not a git repository" {
		t.Errorf("result = %+v, want skipped with the reason", res)
	}
}
