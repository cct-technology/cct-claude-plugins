package rules

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/deps"
	"github.com/rkvalvaa/cct-claude-plugins/internal/waivers"
)

var qk060Today = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

// fakeNPM stands in for every registry while a test runs: it answers the
// given npm documents and 404 for anything else. requests counts what was asked.
func fakeNPM(t *testing.T, pages map[string]string) (requests *int) {
	t.Helper()
	requests = new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++
		switch body, ok := pages[r.URL.Path]; {
		case !ok:
			http.NotFound(w, r)
		case body == "down":
			http.Error(w, "down", http.StatusBadGateway)
		default:
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(srv.Close)
	real := registry
	t.Cleanup(func() { registry = real })
	registry = &deps.Client{
		NPM: srv.URL, GoProxy: srv.URL, Packagist: srv.URL, Crates: srv.URL, PyPI: srv.URL,
		HTTP: srv.Client(), Now: func() time.Time { return qk060Today },
	}
	return requests
}

func npmPackage(version, published, licence string) string {
	return `{"dist-tags": {"latest": "` + version + `"}, "time": {"` + version + `": "` + published + `"}, "versions": {"` + version + `": {"license": "` + licence + `"}}}`
}

var qk060Pages = map[string]string{
	"/react": `{"dist-tags": {"latest": "19.0.0"}, "time": {"18.3.1": "2024-04-26T16:41:27Z", "19.0.0": "2026-11-11T09:00:00Z"},
		"versions": {"18.3.1": {"license": "MIT"}, "19.0.0": {"license": "MIT"}}}`,
	"/left-pad": npmPackage("1.3.0", "2018-04-09T01:10:45Z", "MIT"),
	"/fresh":    npmPackage("0.0.1", "2026-11-12T08:00:00Z", "MIT"),
	"/flaky":    "down",
}

// qk060Repo is a repository whose package.json depends on react, at silver.
func qk060Repo(t *testing.T) check.Context {
	t.Helper()
	ctx := qk020Repo(t, map[string]string{"package.json": `{"dependencies": {"react": "^18.3.1"}}`, "README.md": "x\n"})
	ctx.Today = qk060Today
	return ctx
}

func runRegisteredQK060(t *testing.T, ctx check.Context) check.Result {
	t.Helper()
	for _, r := range All() {
		if r.ID == "QK060" {
			if r.Check != "supply-chain.dependencies" || r.EnforcedFrom != check.Silver || r.Phase != 1 {
				t.Errorf("QK060 = check %q from %v in phase %d, want supply-chain.dependencies from silver in phase 1", r.Check, r.EnforcedFrom, r.Phase)
			}
			return r.Run(ctx)
		}
	}
	t.Fatal("QK060 is not registered")
	return check.Result{}
}

func TestQK060PassesWhenNoDependencyChangesAndAsksNoRegistry(t *testing.T) {
	requests := fakeNPM(t, qk060Pages)
	ctx := qk060Repo(t)
	// Another edit to the manifest, and the same dependency in another group.
	put(t, ctx.Root, "package.json", `{"name": "app", "devDependencies": {"react": "^18.3.1"}}`)
	put(t, ctx.Root, "README.md", "y\n")

	res := runRegisteredQK060(t, changed(t, ctx))

	if res.Status != check.Pass || *requests != 0 {
		t.Errorf("result = %+v after %d requests, want pass and none", res, *requests)
	}
}

// An upgrade is the common way in for a hijacked release: the name is one the
// repository has trusted for years, the version is a day old.
func TestQK060ChecksAnUpgradeInTheManifest(t *testing.T) {
	fakeNPM(t, qk060Pages)
	ctx := qk060Repo(t)
	put(t, ctx.Root, "package.json", `{"dependencies": {"react": "^19.0.0"}}`)

	res := runRegisteredQK060(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 1 ||
		!strings.Contains(res.Findings[0].Message, "`react` (npm) 19.0.0 was published on 2026-11-11, 3 days ago") {
		t.Errorf("result = %+v, want one finding that react 19.0.0 is 3 days old", res)
	}
}

const (
	lockReact18 = `{"lockfileVersion": 3, "packages": {"node_modules/react": {"version": "18.3.1"}, "node_modules/js-tokens": {"version": "4.0.0"}}}`
	lockReact19 = `{"lockfileVersion": 3, "packages": {"node_modules/react": {"version": "19.0.0"}, "node_modules/js-tokens": {"version": "4.0.0"}}}`
)

// `npm update` moves a dependency without touching package.json.
func TestQK060ChecksAnUpgradeThatOnlyTheLockfileShows(t *testing.T) {
	fakeNPM(t, qk060Pages)
	ctx := qk020Repo(t, map[string]string{"web/package.json": `{"dependencies": {"react": "^18.3.1"}}`, "web/package-lock.json": lockReact18})
	ctx.Today = qk060Today
	put(t, ctx.Root, "web/package-lock.json", lockReact19)

	res := runRegisteredQK060(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != "web/package.json" ||
		!strings.Contains(res.Findings[0].Message, "`react` (npm) 19.0.0 was published on 2026-11-11") {
		t.Errorf("result = %+v, want one finding on web/package.json that react 19.0.0 is too young", res)
	}
}

func TestQK060TakesTheVersionFromTheLockfileNotTheRange(t *testing.T) {
	requests := fakeNPM(t, qk060Pages)
	ctx := qk020Repo(t, map[string]string{"package.json": `{"dependencies": {"react": "^18.3.1"}}`, "package-lock.json": lockReact18})
	ctx.Today = qk060Today
	// A wider range that still resolves to 18.3.1, and a package only the lockfile names.
	put(t, ctx.Root, "package.json", `{"dependencies": {"react": ">=18"}}`)
	put(t, ctx.Root, "package-lock.json", strings.Replace(lockReact18, "4.0.0", "9.9.9", 1))

	if res := runRegisteredQK060(t, changed(t, ctx)); res.Status != check.Pass || *requests != 0 {
		t.Errorf("result = %+v after %d requests, want pass and none: react is still 18.3.1, and js-tokens is not in the manifest", res, *requests)
	}
}

func TestQK060PassesForAGoodNewDependency(t *testing.T) {
	fakeNPM(t, qk060Pages)
	ctx := qk060Repo(t)
	put(t, ctx.Root, "package.json", `{"dependencies": {"react": "^18.3.1", "left-pad": "^1.3.0"}}`)

	if res := runRegisteredQK060(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK060FailsWithOneFindingPerProblemOnTheManifest(t *testing.T) {
	fakeNPM(t, qk060Pages)
	ctx := qk060Repo(t)
	put(t, ctx.Root, "package.json", `{"dependencies": {"react": "^18.3.1", "left-pad": "^1.3.0", "fresh": "^0.0.1", "reakt-domm": "^1.0.0"}}`)

	res := runRegisteredQK060(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 2 {
		t.Fatalf("result = %+v, want fail with two findings", res)
	}
	var all []string
	for _, f := range res.Findings {
		if f.File != "package.json" {
			t.Errorf("finding on %q, want package.json: a waiver names the manifest", f.File)
		}
		all = append(all, f.Message)
	}
	got := strings.Join(all, "\n")
	for _, want := range []string{
		"`fresh` (npm) 0.0.1 was published on 2026-11-12, 2 days ago; a new version must be at least 14 days old",
		"`reakt-domm` (npm) does not exist in the registry",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no finding says %q in:\n%s", want, got)
		}
	}
}

func TestQK060ChecksEveryDependencyOfANewManifest(t *testing.T) {
	fakeNPM(t, qk060Pages)
	ctx := qk060Repo(t)
	put(t, ctx.Root, "tools/package.json", `{"devDependencies": {"fresh": "0.0.1"}}`)

	res := runRegisteredQK060(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != "tools/package.json" {
		t.Errorf("result = %+v, want one finding on tools/package.json", res)
	}
}

func TestQK060LeavesOutInstalledPackagesAndDeletedManifests(t *testing.T) {
	requests := fakeNPM(t, qk060Pages)
	ctx := qk020Repo(t, map[string]string{"package.json": `{"dependencies": {"react": "^18.3.1"}}`, "old/package.json": `{"dependencies": {"fresh": "0.0.1"}}`})
	ctx.Today = qk060Today
	put(t, ctx.Root, "node_modules/x/package.json", `{"dependencies": {"reakt-domm": "1.0.0"}}`)
	put(t, ctx.Root, "vendor/y/composer.json", `{"require": {"nope/nope": "1.0.0"}}`)
	gitIn(t, ctx.Root, "rm", "-q", "old/package.json")

	if res := runRegisteredQK060(t, changed(t, ctx)); res.Status != check.Pass || *requests != 0 {
		t.Errorf("result = %+v after %d requests, want pass and none", res, *requests)
	}
}

func TestQK060IsSlow(t *testing.T) {
	requests := fakeNPM(t, qk060Pages)
	ctx := qk060Repo(t)
	ctx.Full = false
	put(t, ctx.Root, "package.json", `{"dependencies": {"react": "^18.3.1", "reakt-domm": "^1.0.0"}}`)

	res := runRegisteredQK060(t, changed(t, ctx))

	if res.Status != check.Skipped || !strings.Contains(res.Reason, "`qk check --full` or in CI") || *requests != 0 {
		t.Errorf("result = %+v after %d requests, want skipped as slow and no request", res, *requests)
	}
	if !strings.Contains(res.Reason, "reakt-domm") {
		t.Errorf("reason %q does not name the dependency that was not checked", res.Reason)
	}
}

func TestQK060SkipsWhenARegistryCannotBeAsked(t *testing.T) {
	fakeNPM(t, qk060Pages)
	ctx := qk060Repo(t)
	put(t, ctx.Root, "package.json", `{"dependencies": {"react": "^18.3.1", "left-pad": "^1.3.0", "flaky": "^1.0.0"}}`)

	res := runRegisteredQK060(t, changed(t, ctx))

	if res.Status != check.Skipped || !strings.Contains(res.Reason, "`flaky` (npm)") || !strings.Contains(res.Reason, "502") {
		t.Errorf("result = %+v, want skipped naming flaky and the registry's answer", res)
	}
}

func TestQK060FindingsWinOverARegistryThatCannotBeAsked(t *testing.T) {
	fakeNPM(t, qk060Pages)
	ctx := qk060Repo(t)
	put(t, ctx.Root, "package.json", `{"dependencies": {"react": "^18.3.1", "fresh": "^0.0.1", "flaky": "^1.0.0"}}`)

	res := runRegisteredQK060(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 1 {
		t.Errorf("result = %+v, want fail with the one finding that could be made", res)
	}
}

func TestQK060SkipsWithoutAChangeSet(t *testing.T) {
	requests := fakeNPM(t, qk060Pages)
	ctx := qk060Repo(t)

	if res := runRegisteredQK060(t, ctx); res.Status != check.Skipped || res.Reason == "" || *requests != 0 {
		t.Errorf("result = %+v, want skipped with a reason", res)
	}
}

// A waiver is for one package: the finding carries its name and version.
func TestQK060AWaiverForOnePackageLeavesTheOthers(t *testing.T) {
	fakeNPM(t, qk060Pages)
	ctx := qk060Repo(t)
	put(t, ctx.Root, "package.json", `{"dependencies": {"react": "^18.3.1", "fresh": "0.0.1", "reakt-domm": "1.0.0"}}`)
	ctx = changed(t, ctx)

	res := runRegisteredQK060(t, ctx)
	var pkgs []string
	for _, f := range res.Findings {
		pkgs = append(pkgs, f.Package)
	}
	if strings.Join(pkgs, " ") != "fresh@0.0.1 reakt-domm" {
		t.Fatalf("findings are about %q, want fresh@0.0.1 and reakt-domm", pkgs)
	}

	w := waivers.Waiver{Rule: "QK060", Packages: []string{"fresh@0.0.1"}, Reason: "urgent fix", Owner: "o", Expires: "2026-11-20"}
	ctx.Waivers = waivers.Loaded{Present: true, Waivers: []waivers.Waiver{w},
		Log: []waivers.LogEntry{waivers.NewLogEntry(w, waivers.Approval{By: "a person", Time: qk060Today})}}
	var rule check.Rule
	for _, r := range All() {
		if r.ID == "QK060" {
			rule = r
		}
	}
	rep := check.Run([]check.Rule{rule}, check.Config{Maturity: check.Silver}, ctx)

	got := rep.Results[0]
	if len(got.Waived) != 1 || got.Waived[0].Message == "" || len(got.Findings) != 1 || got.Findings[0].Package != "reakt-domm" || !rep.Blocking {
		t.Errorf("result = %+v, want fresh waived and reakt-domm still blocking", got)
	}
}
