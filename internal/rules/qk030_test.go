package rules

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/manifest"
)

const contract = "api/openapi.yaml"

func runRegisteredQK030(t *testing.T, ctx check.Context) check.Result {
	t.Helper()
	for _, r := range All() {
		if r.ID == "QK030" {
			if r.Check != "contracts.breaking" || r.EnforcedFrom != check.Silver {
				t.Errorf("QK030 = check %q from %v, want contracts.breaking from silver", r.Check, r.EnforcedFrom)
			}
			return r.Run(ctx)
		}
	}
	t.Fatal("QK030 is not registered")
	return check.Result{}
}

// needOasdiff skips a test that runs the real oasdiff. The CI workflow installs
// it and sets QK_REQUIRE_OASDIFF, so there a missing oasdiff fails instead of
// quietly skipping every such test.
func needOasdiff(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("oasdiff"); err != nil {
		if os.Getenv("QK_REQUIRE_OASDIFF") != "" {
			t.Fatal("QK_REQUIRE_OASDIFF is set, but oasdiff is not on PATH")
		}
		t.Skip("oasdiff is not installed")
	}
}

// withoutOasdiff leaves PATH with nothing on it.
func withoutOasdiff(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func spec(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "qk030", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// qk030Repo commits base.yaml as the contract, plus a second contract, and
// returns a context that lists every YAML file under api/ as OpenAPI.
func qk030Repo(t *testing.T) check.Context {
	t.Helper()
	ctx := qk020Repo(t, map[string]string{contract: spec(t, "base.yaml"), "api/billing.yaml": spec(t, "base.yaml"), "README.md": "x\n"})
	ctx.Manifest.Manifest.Contracts.OpenAPI = []string{"api/*.yaml"}
	return ctx
}

// changed computes the change set once the test has edited the working tree.
func changed(t *testing.T, ctx check.Context) check.Context {
	t.Helper()
	set, err := changes.Compute(ctx.Root, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx.Changes = set
	return ctx
}

func TestQK030FailsWithOneFindingPerBreakingChange(t *testing.T) {
	needOasdiff(t)
	ctx := qk030Repo(t)
	put(t, ctx.Root, contract, spec(t, "breaking.yaml"))

	res := runRegisteredQK030(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 2 {
		t.Fatalf("result = %+v, want fail with two findings", res)
	}
	var all string
	for _, f := range res.Findings {
		if f.File != contract {
			t.Errorf("finding on %q, want %s", f.File, contract)
		}
		all += f.Message + "\n"
	}
	for _, want := range []string{"GET /orders/{id}", "path removed", "GET /orders:", "required", "tenant"} {
		if !strings.Contains(all, want) {
			t.Errorf("findings do not mention %q:\n%s", want, all)
		}
	}
}

func TestQK030PassesOnANonBreakingChange(t *testing.T) {
	needOasdiff(t)
	ctx := qk030Repo(t)
	put(t, ctx.Root, contract, spec(t, "nonbreaking.yaml"))

	if res := runRegisteredQK030(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

// oasdiff cannot confirm some changes break clients, such as a removed
// request parameter. Those warn.
func TestQK030WarnsOnAPossiblyBreakingChange(t *testing.T) {
	needOasdiff(t)
	ctx := qk030Repo(t)
	put(t, ctx.Root, contract, spec(t, "possibly-breaking.yaml"))

	res := runRegisteredQK030(t, changed(t, ctx))

	if res.Status != check.Warn || len(res.Findings) != 1 {
		t.Fatalf("result = %+v, want warn with one finding", res)
	}
	if f := res.Findings[0]; f.File != contract || !strings.Contains(f.Message, "possibly breaking") || !strings.Contains(f.Message, "limit") {
		t.Errorf("finding = %+v, want it on the contract, marked possibly breaking, naming the parameter", f)
	}
}

// A renamed contract is compared with what it held under its old name.
func TestQK030ComparesARenamedContractWithItsOldPath(t *testing.T) {
	needOasdiff(t)
	ctx := qk030Repo(t)
	gitIn(t, ctx.Root, "mv", contract, "api/v1.yaml")
	put(t, ctx.Root, "api/v1.yaml", spec(t, "breaking.yaml"))
	ctx.Changes = &changes.Set{
		Root:  ctx.Root,
		Base:  strings.TrimSpace(gitIn(t, ctx.Root, "rev-parse", "HEAD")),
		Files: []changes.File{{Path: "api/v1.yaml", OldPath: contract, Status: changes.Renamed}},
	}

	res := runRegisteredQK030(t, ctx)

	if res.Status != check.Fail || len(res.Findings) != 2 || res.Findings[0].File != "api/v1.yaml" {
		t.Errorf("result = %+v, want fail with two findings on api/v1.yaml", res)
	}
}

// A contract split over files is compared with the base version of every file.
func TestQK030ComparesAContractSplitOverFiles(t *testing.T) {
	needOasdiff(t)
	if runtime.GOOS == "windows" {
		t.Skip("oasdiff cannot read a $ref target from git on Windows (1.35, 1.37)")
	}
	root := "openapi: 3.0.3\ninfo: {title: Orders, version: 1.0.0}\npaths:\n  /orders:\n    get:\n      responses:\n        \"200\":\n          description: The orders.\n" +
		"          content:\n            application/json:\n              schema:\n                $ref: \"./schemas/order.yaml\"\n"
	ctx := qk020Repo(t, map[string]string{
		contract:                 root,
		"api/schemas/order.yaml": "type: object\nrequired: [id, total]\nproperties:\n  id: {type: string}\n  total: {type: number}\n",
	})
	ctx.Manifest.Manifest.Contracts.OpenAPI = []string{contract}
	put(t, ctx.Root, contract, root+"# changed\n")
	put(t, ctx.Root, "api/schemas/order.yaml", "type: object\nrequired: [id]\nproperties:\n  id: {type: string}\n")

	res := runRegisteredQK030(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != contract || !strings.Contains(res.Findings[0].Message, "total") {
		t.Errorf("result = %+v, want fail with one finding on the contract, naming the removed property", res)
	}
}

// Nothing at the base to break, so oasdiff is not needed either.
func TestQK030PassesOnANewContractWithoutOasdiff(t *testing.T) {
	ctx := qk030Repo(t)
	put(t, ctx.Root, "api/new.yaml", spec(t, "breaking.yaml"))
	ctx = changed(t, ctx)
	withoutOasdiff(t)

	if res := runRegisteredQK030(t, ctx); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK030IgnoresFilesThatAreNotContracts(t *testing.T) {
	ctx := qk030Repo(t)
	put(t, ctx.Root, "README.md", "changed\n")
	put(t, ctx.Root, "deploy/openapi.yaml", spec(t, "breaking.yaml"))
	ctx = changed(t, ctx)
	withoutOasdiff(t)

	if res := runRegisteredQK030(t, ctx); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK030FailsWhenAContractIsDeleted(t *testing.T) {
	ctx := qk030Repo(t)
	if err := os.Remove(filepath.Join(ctx.Root, filepath.FromSlash(contract))); err != nil {
		t.Fatal(err)
	}
	ctx = changed(t, ctx)
	withoutOasdiff(t)

	res := runRegisteredQK030(t, ctx)

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != contract || !strings.Contains(res.Findings[0].Message, "deleted") {
		t.Errorf("result = %+v, want fail with one finding saying the contract was deleted", res)
	}
}

func TestQK030SkipsWithAnInstallHintWithoutOasdiff(t *testing.T) {
	ctx := qk030Repo(t)
	put(t, ctx.Root, contract, spec(t, "breaking.yaml"))
	ctx = changed(t, ctx)
	withoutOasdiff(t)

	res := runRegisteredQK030(t, ctx)

	if res.Status != check.Skipped || !strings.Contains(res.Reason, "oasdiff is not installed") || !strings.Contains(res.Reason, "go install github.com/oasdiff/oasdiff@latest") {
		t.Errorf("result = %+v, want skipped with how to install oasdiff", res)
	}
}

// A contract oasdiff cannot read is a tooling problem: skipped, with what oasdiff said.
func TestQK030SkipsAContractOasdiffCannotRead(t *testing.T) {
	needOasdiff(t)
	ctx := qk030Repo(t)
	put(t, ctx.Root, contract, "openapi: 3.0.3\npaths: [\n")

	res := runRegisteredQK030(t, changed(t, ctx))

	if res.Status != check.Skipped || !strings.Contains(res.Reason, contract) || !strings.Contains(res.Reason, "failed to load") {
		t.Errorf("result = %+v, want skipped, naming the contract and oasdiff's error", res)
	}
}

// One unreadable contract must not hide what oasdiff found in another.
func TestQK030FailsOnOtherContractsWhenOneCannotBeRead(t *testing.T) {
	needOasdiff(t)
	ctx := qk030Repo(t)
	put(t, ctx.Root, contract, "openapi: 3.0.3\npaths: [\n")
	put(t, ctx.Root, "api/billing.yaml", spec(t, "breaking.yaml"))

	res := runRegisteredQK030(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 2 || res.Findings[0].File != "api/billing.yaml" {
		t.Errorf("result = %+v, want fail with two findings on api/billing.yaml", res)
	}
}

func TestQK030PassesWithoutContracts(t *testing.T) {
	if res := runRegisteredQK030(t, check.Context{}); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK030SkipsWithoutAChangeSet(t *testing.T) {
	ctx := check.Context{
		Manifest:   manifest.Loaded{Present: true, Manifest: manifest.Manifest{Version: 1, Contracts: manifest.Contracts{OpenAPI: []string{"api/*.yaml"}}}},
		ChangesErr: errors.New("not a git repository"),
	}

	if res := runRegisteredQK030(t, ctx); res.Status != check.Skipped || res.Reason != "not a git repository" {
		t.Errorf("result = %+v, want skipped with the reason", res)
	}
}
