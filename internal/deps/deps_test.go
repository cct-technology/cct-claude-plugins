package deps

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// now is the day every test is judged on. 2026-06-01 is "old"; 2026-11-10 is 4 days ago.
var now = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

const (
	old   = "2026-06-01T10:00:00Z"
	young = "2026-11-10T10:00:00Z"
)

// fakeRegistry answers each path with its body, and 404 for every other path.
// It counts requests, so a test can tell a cache hit from a second request.
type fakeRegistry struct {
	pages    map[string]string
	requests int
}

func (f *fakeRegistry) client(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests++
		if r.UserAgent() == "" || strings.Contains(r.UserAgent(), "Go-http-client") {
			t.Errorf("request to %s without qk's User-Agent; crates.io refuses those", r.URL.Path)
		}
		body, ok := f.pages[r.URL.EscapedPath()]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Client{
		NPM: srv.URL, GoProxy: srv.URL, Packagist: srv.URL, Crates: srv.URL, PyPI: srv.URL,
		CacheDir: t.TempDir(), HTTP: srv.Client(), Now: func() time.Time { return now },
	}
}

func kinds(v Verdict) []string {
	out := []string{}
	for _, p := range v.Problems {
		out = append(out, p.Kind)
	}
	return out
}

func npmDoc(version, published, extra string) string {
	return `{"dist-tags": {"latest": "` + version + `"}, "time": {"created": "` + old + `", "` + version + `": "` + published + `"}, "versions": {"` + version + `": {` + extra + `}}}`
}

func TestVerify(t *testing.T) {
	reg := &fakeRegistry{pages: map[string]string{
		"/good":         npmDoc("1.3.0", old, `"license": "MIT"`),
		"/@scope%2Fpkg": npmDoc("2.0.0", old, `"license": {"type": "ISC"}`),
		"/fresh":        npmDoc("0.0.1", young, `"license": "MIT"`),
		"/left-pad":     npmDoc("1.3.0", old, `"license": "MIT", "deprecated": "use String.prototype.padStart()"`),
		"/copyleft":     npmDoc("3.0.0", old, `"license": "GPL-3.0-only"`),
		"/unlicensed":   npmDoc("1.0.0", old, ``),
		"/everything":   npmDoc("0.1.0", young, `"deprecated": "do not use", "licenses": [{"type": "GPL-2.0"}, {"type": "LGPL-2.1"}]`),
		"/unpublished":  `{"name": "unpublished", "time": {"created": "` + old + `", "unpublished": {"time": "` + old + `"}}}`,
		"/two-versions": `{"dist-tags": {"latest": "2.0.0", "stable": "1.0.0"}, "time": {"1.0.0": "` + old + `", "2.0.0": "` + young + `"}, "versions": {"1.0.0": {"license": "MIT"}, "2.0.0": {"license": "MIT"}}}`,

		"/github.com/spf13/cobra/@latest":              `{"Version": "v1.8.0", "Time": "` + old + `"}`,
		"/github.com/spf13/cobra/@v/v1.8.0.mod":        "module github.com/spf13/cobra\n",
		"/github.com/!burnt!sushi/toml/@latest":        `{"Version": "v1.6.0", "Time": "` + young + `"}`,
		"/github.com/!burnt!sushi/toml/@v/v1.4.0.info": `{"Version": "v1.4.0", "Time": "` + old + `"}`,
		"/github.com/!burnt!sushi/toml/@v/v1.6.0.mod":  "module github.com/BurntSushi/toml\n",
		"/github.com/golang/protobuf/@latest":          `{"Version": "v1.5.4", "Time": "` + old + `"}`,
		"/github.com/golang/protobuf/@v/v1.5.4.mod":    "// Deprecated: Use the \"google.golang.org/protobuf\" module instead.\nmodule github.com/golang/protobuf\n",

		"/packages/monolog/monolog.json":         `{"package": {"versions": {"dev-main": {"time": "` + young + `", "license": ["MIT"]}, "3.7.0": {"time": "` + old + `", "license": ["MIT"]}, "3.6.0": {"time": "2026-01-01T00:00:00+00:00", "license": ["MIT"]}}}}`,
		"/packages/swiftmailer/swiftmailer.json": `{"package": {"abandoned": "symfony/mailer", "versions": {"v6.3.0": {"time": "` + old + `", "license": ["MIT"]}}}}`,
		"/packages/old/gone.json":                `{"package": {"abandoned": true, "versions": {"1.0.0": {"time": "` + old + `", "license": ["GPL-2.0-or-later", "MIT"]}}}}`,

		"/api/v1/crates/serde":  `{"crate": {"max_stable_version": "1.0.229", "max_version": "1.0.229"}, "versions": [{"num": "1.0.229", "created_at": "` + old + `", "license": "MIT OR Apache-2.0", "yanked": false}, {"num": "1.0.100", "created_at": "` + old + `", "license": "MIT/Apache-2.0", "yanked": true}]}`,
		"/api/v1/crates/gplish": `{"crate": {"max_stable_version": "0.1.0"}, "versions": [{"num": "0.1.0", "created_at": "` + young + `", "license": "MIT AND GPL-3.0-only", "yanked": false}]}`,

		"/pypi/requests/json": `{"info": {"version": "2.34.2", "license": "Apache-2.0", "license_expression": null, "classifiers": ["License :: OSI Approved :: Apache Software License"]}, "releases": {"2.34.2": [{"upload_time_iso_8601": "` + old + `", "yanked": false}], "2.31.0": [{"upload_time_iso_8601": "2023-05-22T15:12:42Z", "yanked": false}]}}`,
		"/pypi/nose/json":     `{"info": {"version": "1.3.7", "license": "", "classifiers": ["Development Status :: 7 - Inactive", "License :: OSI Approved :: GNU Library or Lesser General Public License (LGPL)"]}, "releases": {"1.3.7": [{"upload_time_iso_8601": "` + old + `", "yanked": false}]}}`,
		"/pypi/pulled/json":   `{"info": {"version": "1.0.0", "license_expression": "MIT", "classifiers": []}, "releases": {"1.0.0": [{"upload_time_iso_8601": "` + old + `", "yanked": true, "yanked_reason": "broken build"}]}}`,
		"/pypi/classy/json":   `{"info": {"version": "1.0.0", "license": "A long licence text.\nSecond line.", "classifiers": ["License :: OSI Approved :: MIT License"]}, "releases": {"1.0.0": [{"upload_time_iso_8601": "` + old + `", "yanked": false}]}}`,
		"/pypi/empty/json":    `{"info": {"version": "", "classifiers": []}, "releases": {}}`,
	}}
	c := reg.client(t)

	cases := []struct {
		name    string
		pkg     Package
		version string   // the version looked at
		kinds   []string // the problems found
		says    string   // text one of the problems or notes must hold
	}{
		{"a good package", Package{"npm", "good", "^1.3.0"}, "1.3.0", nil, ""},
		{"a scoped npm name, licence as an object", Package{"npm", "@scope/pkg", ""}, "2.0.0", nil, ""},
		{"a package that does not exist", Package{"npm", "reakt-domm", "^1.0.0"}, "", []string{Missing}, "`reakt-domm` (npm) does not exist in the registry"},
		{"an unpublished package is missing", Package{"npm", "unpublished", ""}, "", []string{Missing}, ""},
		{"too young", Package{"npm", "fresh", "0.0.1"}, "0.0.1", []string{TooYoung}, "`fresh` (npm) 0.0.1 was published on 2026-11-10, 4 days ago; a new version must be at least 14 days old"},
		{"deprecated", Package{"npm", "left-pad", "^1.3.0"}, "1.3.0", []string{Deprecated}, `is deprecated. The registry says: "use String.prototype.padStart()"`},
		{"a licence that is not allowed", Package{"npm", "copyleft", ""}, "3.0.0", []string{Licence}, "has the licence `GPL-3.0-only`, which is not on the allowed list (MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, 0BSD, Unlicense)"},
		{"no licence at all", Package{"npm", "unlicensed", ""}, "1.0.0", []string{Licence}, "declares no licence"},
		{"every problem at once", Package{"npm", "everything", ""}, "0.1.0", []string{TooYoung, Deprecated, Licence}, "`GPL-2.0 OR LGPL-2.1`"},
		{"an exact version: that version, not the newest", Package{"npm", "two-versions", "1.0.0"}, "1.0.0", nil, ""},
		{"a range: the newest release, the one an install without a lockfile may fetch", Package{"npm", "two-versions", "^1.0.0"}, "2.0.0", []string{TooYoung}, ""},
		{"an npm tag: the version it points at", Package{"npm", "two-versions", "stable"}, "1.0.0", nil, ""},
		{"no version named: the newest release", Package{"npm", "two-versions", "*"}, "2.0.0", []string{TooYoung}, ""},
		{"a version the registry does not have: the newest release", Package{"npm", "two-versions", "1.5.0"}, "2.0.0", []string{TooYoung}, ""},

		{"go: good, and the proxy has no licences", Package{"go", "github.com/spf13/cobra", "v1.8.0"}, "v1.8.0", nil, "licence not checked: this registry holds no licence data"},
		{"go: upper case in the path, an older version", Package{"go", "github.com/BurntSushi/toml", "v1.4.0"}, "v1.4.0", nil, ""},
		{"go: the newest version is young", Package{"go", "github.com/BurntSushi/toml", "v1.6.0"}, "v1.6.0", []string{TooYoung}, ""},
		{"go: deprecated in go.mod", Package{"go", "github.com/golang/protobuf", "v1.5.4"}, "v1.5.4", []string{Deprecated}, `Use the 'google.golang.org/protobuf' module instead.`},
		{"go: no such module", Package{"go", "github.com/spf13/kobra", "v1.0.0"}, "", []string{Missing}, ""},

		{"composer: good; a branch is not a release", Package{"composer", "monolog/monolog", "^3.0"}, "3.7.0", nil, ""},
		{"composer: abandoned, with a replacement", Package{"composer", "swiftmailer/swiftmailer", "6.3.0"}, "v6.3.0", []string{Deprecated}, "abandoned; use symfony/mailer"},
		{"composer: abandoned; one allowed licence of a choice is enough", Package{"composer", "old/gone", ""}, "1.0.0", []string{Deprecated}, `is deprecated. The registry says: "abandoned"`},
		{"composer: no such package", Package{"composer", "nope/nope", ""}, "", []string{Missing}, ""},

		{"cargo: good, a choice of licences", Package{"cargo", "serde", "1"}, "1.0.229", nil, ""},
		{"cargo: a yanked version", Package{"cargo", "serde", "1.0.100"}, "1.0.100", []string{Deprecated}, "this version was yanked"},
		{"cargo: every licence of an AND must be allowed", Package{"cargo", "gplish", "0.1"}, "0.1.0", []string{TooYoung, Licence}, "`MIT AND GPL-3.0-only`"},
		{"cargo: no such crate", Package{"cargo", "serdee", ""}, "", []string{Missing}, ""},

		{"pypi: good, licence as a name", Package{"pypi", "requests", ">=2.31"}, "2.34.2", nil, ""},
		{"pypi: the version named", Package{"pypi", "requests", "==2.31.0"}, "2.31.0", nil, ""},
		{"pypi: inactive, licence from the classifier", Package{"pypi", "nose", ""}, "1.3.7", []string{Deprecated, Licence}, "`GNU Library or Lesser General Public License (LGPL)`"},
		{"pypi: yanked", Package{"pypi", "pulled", ""}, "1.0.0", []string{Deprecated}, "this version was yanked broken build"},
		{"pypi: a licence text is not a name; the classifier is", Package{"pypi", "classy", ""}, "1.0.0", nil, ""},
		{"pypi: a name with nothing to install", Package{"pypi", "empty", ""}, "", []string{Missing}, ""},
		{"pypi: no such project", Package{"pypi", "reqeusts", ""}, "", []string{Missing}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := c.Verify(tc.pkg, now)
			if v.Err != nil {
				t.Fatalf("Verify: %v", v.Err)
			}
			if got := kinds(v); !slices.Equal(got, append([]string{}, tc.kinds...)) {
				t.Errorf("problems = %v, want %v\n%+v", got, tc.kinds, v.Problems)
			}
			if v.Info.Version != tc.version {
				t.Errorf("looked at version %q, want %q", v.Info.Version, tc.version)
			}
			all := strings.Join(v.Notes, "\n")
			for _, p := range v.Problems {
				all += "\n" + p.Message
			}
			if !strings.Contains(all, tc.says) {
				t.Errorf("nothing says %q in:\n%s", tc.says, all)
			}
		})
	}
}

func TestVerifyUsesTheCacheForADay(t *testing.T) {
	reg := &fakeRegistry{pages: map[string]string{"/fresh": npmDoc("0.0.1", young, `"license": "MIT"`)}}
	c := reg.client(t)
	pkg := Package{"npm", "fresh", "0.0.1"}

	c.Verify(pkg, now)
	first := reg.requests
	if v := c.Verify(pkg, now); reg.requests != first || !slices.Equal(kinds(v), []string{TooYoung}) {
		t.Errorf("second check made %d more requests and found %v, want 0 and too-young", reg.requests-first, kinds(v))
	}

	// The cache holds the publish date, not the verdict: 20 days on, the same
	// answer is old enough. Now is still the day it was stored.
	if v := c.Verify(pkg, now.Add(20*24*time.Hour)); len(v.Problems) != 0 || reg.requests != first {
		t.Errorf("20 days later: problems %v after %d more requests, want none and 0", kinds(v), reg.requests-first)
	}

	c.Now = func() time.Time { return now.Add(25 * time.Hour) }
	if c.Verify(pkg, now); reg.requests == first {
		t.Error("after 25 hours the registry was not asked again")
	}
}

func TestAMissingPackageIsNotCached(t *testing.T) {
	reg := &fakeRegistry{pages: map[string]string{}}
	c := reg.client(t)
	pkg := Package{"npm", "soon", ""}
	c.Verify(pkg, now)
	reg.pages["/soon"] = npmDoc("1.0.0", old, `"license": "MIT"`)
	if v := c.Verify(pkg, now); len(v.Problems) != 0 {
		t.Errorf("published a minute later, still: %v", v.Problems)
	}
}

// A registry that cannot be asked is never a finding: Err is set, Problems is empty.
func TestARegistryThatCannotBeAskedIsNotAFinding(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/busy":
			http.Error(w, "slow down", http.StatusTooManyRequests)
		case "/garbled":
			_, _ = w.Write([]byte("<html>maintenance</html>"))
		default:
			http.Error(w, "oops", http.StatusInternalServerError)
		}
	}))
	defer broken.Close()
	offline := httptest.NewServer(http.NotFoundHandler())
	offline.Close() // nothing listens there any more

	for name, url := range map[string]string{"an error from the registry": broken.URL, "offline": offline.URL} {
		c := &Client{NPM: url, GoProxy: url, Packagist: url, Crates: url, PyPI: url, HTTP: &http.Client{Timeout: 5 * time.Second}, Now: time.Now}
		for _, pkg := range []Package{{"npm", "busy", ""}, {"npm", "garbled", ""}, {"go", "example.com/x", "v1.0.0"}, {"composer", "a/b", ""}, {"cargo", "x", ""}, {"pypi", "x", ""}} {
			if v := c.Verify(pkg, now); v.Err == nil || len(v.Problems) != 0 {
				t.Errorf("%s, %v: Err = %v, problems = %v; want an error and no problems", name, pkg, v.Err, v.Problems)
			}
		}
	}
}

func TestUnknownEcosystemIsAnError(t *testing.T) {
	if v := (&Client{}).Verify(Package{"gems", "rails", ""}, now); v.Err == nil || !strings.Contains(v.Err.Error(), "npm, go, composer, cargo, pypi") {
		t.Errorf("Err = %v, want one that lists the ecosystems", v.Err)
	}
}

func TestAllowed(t *testing.T) {
	for expr, want := range map[string]bool{
		"MIT":                                       true,
		"mit":                                       true,
		"Apache-2.0":                                true,
		"MIT OR GPL-3.0-only":                       true,
		"GPL-3.0-only OR MIT":                       true,
		"MIT AND GPL-3.0-only":                      false,
		"MIT AND BSD-3-Clause":                      true,
		"MIT/Apache-2.0":                            true,
		"(MIT OR GPL-3.0-only) AND GPL-2.0-only":    false,
		"(MIT OR GPL-3.0-only) AND ISC":             true,
		"MIT OR GPL-2.0-only AND GPL-3.0-only":      true,
		"Apache-2.0 WITH LLVM-exception":            true,
		"GPL-2.0-only WITH Classpath-exception-2.0": false,
		"GPL-3.0-only":                              false,
		"MPL-2.0":                                   false,
		"SEE LICENSE IN LICENSE.txt":                false,
		"UNLICENSED":                                false,
		"MIT License":                               false,
		"(MIT":                                      false,
		"":                                          false,
	} {
		if got := Allowed(expr); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", expr, got, want)
		}
	}
}
